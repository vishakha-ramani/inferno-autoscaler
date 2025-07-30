package metrics

import (
	"context"

	llmdOptv1alpha1 "github.com/llm-d-incubation/inferno-autoscaler/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	replicaScalingTotal *prometheus.CounterVec
	desiredReplicas     *prometheus.GaugeVec
	currentReplicas     *prometheus.GaugeVec
	optimizationErrors  *prometheus.CounterVec
)

// InitMetrics registers all custom metrics with the provided registry
func InitMetrics(registry prometheus.Registerer) {
	replicaScalingTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "inferno_replica_scaling_total",
			Help: "Total number of replica scaling operations",
		},
		[]string{"variant_name", "namespace", "direction", "reason"},
	)
	desiredReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "inferno_desired_replicas",
			Help: "Desired number of replicas for each variant",
		},
		[]string{"variant_name", "namespace", "accelerator_type"},
	)
	currentReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "inferno_current_replicas",
			Help: "Current number of replicas for each variant",
		},
		[]string{"variant_name", "namespace", "accelerator_type"},
	)
	optimizationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "inferno_optimization_errors_total",
			Help: "Total number of optimization errors",
		},
		[]string{"variant_name", "namespace", "error_type"},
	)

	registry.MustRegister(replicaScalingTotal)
	registry.MustRegister(desiredReplicas)
	registry.MustRegister(currentReplicas)
	registry.MustRegister(optimizationErrors)
}

// InitMetricsAndEmitter registers metrics with Prometheus and creates a metrics emitter
// This is a convenience function that handles both registration and emitter creation
func InitMetricsAndEmitter(registry prometheus.Registerer) *MetricsEmitter {
	InitMetrics(registry)
	return NewMetricsEmitter()
}

// MetricsEmitter handles emission of custom metrics
type MetricsEmitter struct{}

// NewMetricsEmitter creates a new metrics emitter
func NewMetricsEmitter() *MetricsEmitter {
	return &MetricsEmitter{}
}

// EmitReplicaScalingMetrics emits metrics related to replica scaling
func (m *MetricsEmitter) EmitReplicaScalingMetrics(ctx context.Context, va *llmdOptv1alpha1.VariantAutoscaling, direction, reason string) {
	if va == nil {
		return
	}

	labels := prometheus.Labels{
		"variant_name": va.Name,
		"namespace":    va.Namespace,
		"direction":    direction,
		"reason":       reason,
	}

	replicaScalingTotal.With(labels).Inc()
}

// EmitReplicaMetrics emits current and desired replica metrics
func (m *MetricsEmitter) EmitReplicaMetrics(ctx context.Context, va *llmdOptv1alpha1.VariantAutoscaling, current, desired int32, acceleratorType string) {
	if va == nil {
		return
	}

	baseLabels := prometheus.Labels{
		"variant_name":     va.Name,
		"namespace":        va.Namespace,
		"accelerator_type": acceleratorType,
	}

	currentReplicas.With(baseLabels).Set(float64(current))
	desiredReplicas.With(baseLabels).Set(float64(desired))
}

// EmitErrorMetrics emits error-related metrics
func (m *MetricsEmitter) EmitErrorMetrics(ctx context.Context, va *llmdOptv1alpha1.VariantAutoscaling, errorType string) {
	if va == nil {
		return
	}

	labels := prometheus.Labels{
		"variant_name": va.Name,
		"namespace":    va.Namespace,
		"error_type":   errorType,
	}

	optimizationErrors.With(labels).Inc()
}
