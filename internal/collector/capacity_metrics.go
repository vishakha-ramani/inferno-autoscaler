package controller

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/constants"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/interfaces"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

var (
	// kubernetesLabelPattern validates Kubernetes label values (RFC 1123 subdomain)
	// Matches alphanumeric, hyphens, dots, underscores (must start/end with alphanumeric)
	kubernetesLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-_.]*[a-zA-Z0-9])?$`)
)

// CapacityMetricsCollector collects vLLM capacity metrics from Prometheus
type CapacityMetricsCollector struct {
	promAPI promv1.API
}

// NewCapacityMetricsCollector creates a new capacity metrics collector
func NewCapacityMetricsCollector(promAPI promv1.API) *CapacityMetricsCollector {
	return &CapacityMetricsCollector{
		promAPI: promAPI,
	}
}

// validatePrometheusLabel validates that a label value is safe for use in Prometheus queries.
// This prevents query injection attacks by ensuring the value matches Kubernetes label patterns.
// Returns error if validation fails.
func validatePrometheusLabel(value, name string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}
	// Kubernetes label validation (RFC 1123 subdomain)
	// Must be 63 characters or less and match the pattern
	if len(value) > 63 {
		return fmt.Errorf("%s too long (max 63 characters): %s", name, value)
	}
	if !kubernetesLabelPattern.MatchString(value) {
		return fmt.Errorf("invalid %s: must match Kubernetes label pattern (alphanumeric, '-', '_', '.' allowed, must start and end with alphanumeric): %s", name, value)
	}
	return nil
}

// contextWithRespectedDeadline creates a timeout context that respects the parent context deadline.
// If the parent has a deadline shorter than the desired timeout, uses the parent's remaining time minus a buffer.
// Returns the context and cancel function.
func contextWithRespectedDeadline(parent context.Context, desiredTimeout time.Duration) (context.Context, context.CancelFunc) {
	deadline, hasDeadline := parent.Deadline()
	if !hasDeadline {
		// No parent deadline, use desired timeout
		return context.WithTimeout(parent, desiredTimeout)
	}

	// Calculate remaining time from parent deadline
	remaining := time.Until(deadline)
	if remaining <= 0 {
		// Parent already expired, use minimal timeout
		return context.WithTimeout(parent, time.Millisecond)
	}

	// If remaining time is less than desired, use remaining minus buffer
	const deadlineBuffer = 100 * time.Millisecond
	if remaining < desiredTimeout {
		timeout := remaining - deadlineBuffer
		if timeout < time.Millisecond {
			timeout = time.Millisecond
		}
		return context.WithTimeout(parent, timeout)
	}

	// Parent deadline is generous, use desired timeout
	return context.WithTimeout(parent, desiredTimeout)
}

// CollectReplicaMetrics collects KV cache and queue metrics for all replicas of a model.
// It queries Prometheus for:
// - constants.VLLMKvCacheUsagePerc (KV cache utilization 0.0-1.0)
// - constants.VLLMNumRequestsWaiting (queue length)
//
// Uses max_over_time[1m] to capture peak values in the last minute for safety-first
// capacity guardrails. This prevents missing saturation events that could occur between
// instant queries and provides more conservative capacity analysis.
//
// The function groups metrics by model ID across all variants.
func (cmc *CapacityMetricsCollector) CollectReplicaMetrics(
	ctx context.Context,
	modelID string,
	namespace string,
	variantCosts map[string]float64,
) ([]interfaces.ReplicaMetrics, error) {

	// Validate input to prevent injection and ensure valid queries
	if err := validatePrometheusLabel(namespace, "namespace"); err != nil {
		return nil, err
	}
	if err := validatePrometheusLabel(modelID, "modelID"); err != nil {
		return nil, err
	}

	// Query KV cache and queue metrics in parallel for better performance
	// Use result struct to avoid race conditions on error variables
	type queryResult struct {
		kvMetrics    map[string]float64
		queueMetrics map[string]int
		kvErr        error
		queueErr     error
	}
	result := &queryResult{}
	var resultMutex sync.Mutex
	var wg sync.WaitGroup

	wg.Add(2)

	// Query KV cache metrics in parallel
	go func() {
		defer wg.Done()
		kv, err := cmc.queryKvCacheMetrics(ctx, modelID, namespace)
		resultMutex.Lock()
		result.kvMetrics = kv
		result.kvErr = err
		resultMutex.Unlock()
	}()

	// Query queue metrics in parallel
	go func() {
		defer wg.Done()
		queue, err := cmc.queryQueueMetrics(ctx, modelID, namespace)
		resultMutex.Lock()
		result.queueMetrics = queue
		result.queueErr = err
		resultMutex.Unlock()
	}()

	wg.Wait()

	// Check for errors after both queries complete
	if result.kvErr != nil {
		return nil, fmt.Errorf("failed to query KV cache metrics: %w", result.kvErr)
	}
	if result.queueErr != nil {
		return nil, fmt.Errorf("failed to query queue metrics: %w", result.queueErr)
	}

	// Use results from struct
	kvMetricsMap := result.kvMetrics
	queueMetricsMap := result.queueMetrics

	// Merge metrics by pod
	replicaMetrics := cmc.mergeMetrics(kvMetricsMap, queueMetricsMap, modelID, namespace, variantCosts)

	logger.Log.Debug("Collected replica metrics",
		"modelID", modelID,
		"namespace", namespace,
		"replicaCount", len(replicaMetrics))

	return replicaMetrics, nil
}

// queryKvCacheMetrics queries constants.VLLMKvCacheUsagePerc metric with max_over_time[1m]
// to capture peak KV cache usage in the last minute for conservative capacity analysis.
func (cmc *CapacityMetricsCollector) queryKvCacheMetrics(
	ctx context.Context,
	modelID string,
	namespace string,
) (map[string]float64, error) {

	// Query for peak KV cache usage over last minute across all pods of this model (all variants)
	// Using max_over_time ensures we don't miss saturation events between queries
	// TODO: Verify vLLM metrics include model_id label, adjust filter if needed
	query := fmt.Sprintf(`max_over_time(%s{namespace="%s",model_id="%s"}[1m])`,
		constants.VLLMKvCacheUsagePerc, namespace, modelID)

	// Add timeout to prevent hanging on Prometheus issues (respects parent deadline)
	queryCtx, cancel := contextWithRespectedDeadline(ctx, 5*time.Second)
	defer cancel()

	result, warnings, err := cmc.promAPI.Query(queryCtx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	if len(warnings) > 0 {
		logger.Log.Warn("Prometheus query returned warnings",
			"query", query,
			"warnings", warnings)
	}

	metricsMap := make(map[string]float64)

	if result.Type() == model.ValVector {
		vector := result.(model.Vector)
		for _, sample := range vector {
			podName := string(sample.Metric["pod"])
			if podName == "" {
				// Try alternative label names
				podName = string(sample.Metric["pod_name"])
			}

			if podName != "" {
				metricsMap[podName] = float64(sample.Value)
			}
		}
	}

	logger.Log.Debug("KV cache metrics collected (max over 1m)",
		"modelID", modelID,
		"namespace", namespace,
		"podCount", len(metricsMap))

	return metricsMap, nil
}

// queryQueueMetrics queries constants.VLLMNumRequestsWaiting metric with max_over_time[1m]
// to capture peak queue length in the last minute for conservative capacity analysis.
func (cmc *CapacityMetricsCollector) queryQueueMetrics(
	ctx context.Context,
	modelID string,
	namespace string,
) (map[string]int, error) {

	// Query for peak queue length over last minute
	// Using max_over_time ensures we catch burst traffic that could saturate the system
	// TODO: Verify vLLM metrics include model_id label, adjust filter if needed
	query := fmt.Sprintf(`max_over_time(%s{namespace="%s",model_id="%s"}[1m])`,
		constants.VLLMNumRequestsWaiting, namespace, modelID)

	// Add timeout to prevent hanging on Prometheus issues (respects parent deadline)
	queryCtx, cancel := contextWithRespectedDeadline(ctx, 5*time.Second)
	defer cancel()

	result, warnings, err := cmc.promAPI.Query(queryCtx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	if len(warnings) > 0 {
		logger.Log.Warn("Prometheus query returned warnings",
			"query", query,
			"warnings", warnings)
	}

	metricsMap := make(map[string]int)

	if result.Type() == model.ValVector {
		vector := result.(model.Vector)
		for _, sample := range vector {
			podName := string(sample.Metric["pod"])
			if podName == "" {
				podName = string(sample.Metric["pod_name"])
			}

			if podName != "" {
				metricsMap[podName] = int(sample.Value)
			}
		}
	}

	logger.Log.Debug("Queue metrics collected (max over 1m)",
		"modelID", modelID,
		"namespace", namespace,
		"podCount", len(metricsMap))

	return metricsMap, nil
}

// mergeMetrics combines KV cache and queue metrics into ReplicaMetrics structs
func (cmc *CapacityMetricsCollector) mergeMetrics(
	kvMetrics map[string]float64,
	queueMetrics map[string]int,
	modelID string,
	namespace string,
	variantCosts map[string]float64,
) []interfaces.ReplicaMetrics {

	// Use union of pod names from both metric sets
	podSet := make(map[string]bool)
	for pod := range kvMetrics {
		podSet[pod] = true
	}
	for pod := range queueMetrics {
		podSet[pod] = true
	}

	replicaMetrics := make([]interfaces.ReplicaMetrics, 0, len(podSet))

	for podName := range podSet {
		// Check for missing metrics and warn (prevents silent data loss)
		kvUsage, hasKv := kvMetrics[podName]
		queueLen, hasQueue := queueMetrics[podName]

		if !hasKv {
			logger.Log.Warn("Pod missing KV cache metrics, using 0 (may cause incorrect capacity analysis)",
				"pod", podName,
				"model", modelID,
				"namespace", namespace)
			kvUsage = 0
		}
		if !hasQueue {
			logger.Log.Warn("Pod missing queue metrics, using 0 (may cause incorrect capacity analysis)",
				"pod", podName,
				"model", modelID,
				"namespace", namespace)
			queueLen = 0
		}

		// TODO: Extract variant name and accelerator from pod labels
		// For now, use placeholder - will be enhanced in controller integration
		variantName := "unknown"
		acceleratorName := "unknown"

		// Look up cost by variant name, default to 10.0 if not found
		cost := 10.0
		if variantCosts != nil {
			if c, ok := variantCosts[variantName]; ok {
				cost = c
			}
		}

		metric := interfaces.ReplicaMetrics{
			PodName:         podName,
			ModelID:         modelID,
			Namespace:       namespace,
			VariantName:     variantName,
			AcceleratorName: acceleratorName,
			KvCacheUsage:    kvUsage,
			QueueLength:     queueLen,
			Cost:            cost,
		}

		replicaMetrics = append(replicaMetrics, metric)
	}

	return replicaMetrics
}

// CollectReplicaMetricsFromPods collects metrics with pod metadata enrichment.
// This version takes pod information to properly populate variant and accelerator names.
func (cmc *CapacityMetricsCollector) CollectReplicaMetricsFromPods(
	ctx context.Context,
	pods []PodInfo,
	variantCosts map[string]float64,
) ([]interfaces.ReplicaMetrics, error) {

	if len(pods) == 0 {
		return []interfaces.ReplicaMetrics{}, nil
	}

	// Use first pod's namespace for query (all should be same namespace)
	namespace := pods[0].Namespace
	modelID := pods[0].ModelID

	// Query metrics in parallel for better performance
	// Use result struct to avoid race conditions on error variables
	type queryResult struct {
		kvMetrics    map[string]float64
		queueMetrics map[string]int
		kvErr        error
		queueErr     error
	}
	result := &queryResult{}
	var resultMutex sync.Mutex
	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		kv, err := cmc.queryKvCacheMetrics(ctx, modelID, namespace)
		resultMutex.Lock()
		result.kvMetrics = kv
		result.kvErr = err
		resultMutex.Unlock()
	}()

	go func() {
		defer wg.Done()
		queue, err := cmc.queryQueueMetrics(ctx, modelID, namespace)
		resultMutex.Lock()
		result.queueMetrics = queue
		result.queueErr = err
		resultMutex.Unlock()
	}()

	wg.Wait()

	if result.kvErr != nil {
		return nil, fmt.Errorf("failed to query KV cache metrics: %w", result.kvErr)
	}
	if result.queueErr != nil {
		return nil, fmt.Errorf("failed to query queue metrics: %w", result.queueErr)
	}

	// Use results from struct
	kvMetricsMap := result.kvMetrics
	queueMetricsMap := result.queueMetrics

	// Merge with pod metadata
	replicaMetrics := make([]interfaces.ReplicaMetrics, 0, len(pods))

	for _, pod := range pods {
		// Look up cost by variant name, default to 10.0 if not found
		cost := 10.0
		if variantCosts != nil {
			if c, ok := variantCosts[pod.VariantName]; ok {
				cost = c
			}
		}

		metric := interfaces.ReplicaMetrics{
			PodName:         pod.Name,
			ModelID:         pod.ModelID,
			Namespace:       pod.Namespace,
			VariantName:     pod.VariantName,
			AcceleratorName: pod.AcceleratorName,
			KvCacheUsage:    kvMetricsMap[pod.Name],    // 0 if not found
			QueueLength:     queueMetricsMap[pod.Name], // 0 if not found
			Cost:            cost,
		}

		replicaMetrics = append(replicaMetrics, metric)
	}

	logger.Log.Debug("Collected enriched replica metrics (max over 1m)",
		"modelID", modelID,
		"namespace", namespace,
		"replicaCount", len(replicaMetrics))

	return replicaMetrics, nil
}

// PodInfo holds metadata about a pod for metric enrichment
type PodInfo struct {
	Name            string
	Namespace       string
	ModelID         string
	VariantName     string
	AcceleratorName string
}
