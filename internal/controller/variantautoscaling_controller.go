/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	llmdVariantAutoscalingV1alpha1 "github.com/llm-d-incubation/workload-variant-autoscaler/api/v1alpha1"
	actuator "github.com/llm-d-incubation/workload-variant-autoscaler/internal/actuator"
	capacity "github.com/llm-d-incubation/workload-variant-autoscaler/internal/capacity"
	collector "github.com/llm-d-incubation/workload-variant-autoscaler/internal/collector"
	interfaces "github.com/llm-d-incubation/workload-variant-autoscaler/internal/interfaces"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/metrics"
	analyzer "github.com/llm-d-incubation/workload-variant-autoscaler/internal/modelanalyzer"
	variantAutoscalingOptimizer "github.com/llm-d-incubation/workload-variant-autoscaler/internal/optimizer"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/utils"
	infernoConfig "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	inferno "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/core"
	infernoManager "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/manager"
	infernoSolver "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/solver"
	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// VariantAutoscalingReconciler reconciles a variantAutoscaling object
type VariantAutoscalingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Recorder emits Kubernetes events for observability. We keep it to follow Kubernetes
	// controller best practices and provide visibility into critical issues (e.g., ServiceMonitor
	// deletion) that may not be immediately apparent from logs alone. Events are accessible via
	// `kubectl get events` and can be monitored by cluster operators and external tooling.
	Recorder record.EventRecorder

	PromAPI promv1.API

	// Capacity scaling config cache (thread-safe, updated on ConfigMap changes)
	capacityConfigCache      map[string]interfaces.CapacityScalingConfig
	capacityConfigCacheMutex sync.RWMutex
	capacityConfigLoaded     bool // Track if initial load succeeded
}

// +kubebuilder:rbac:groups=llmd.ai,resources=variantautoscalings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=llmd.ai,resources=variantautoscalings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=llmd.ai,resources=variantautoscalings/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=get;list;update;patch;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;update;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

const (
	configMapName      = "workload-variant-autoscaler-variantautoscaling-config"
	configMapNamespace = "workload-variant-autoscaler-system"
	// ServiceMonitor constants for watching controller's own metrics ServiceMonitor
	serviceMonitorName = "workload-variant-autoscaler-controller-manager-metrics-monitor"
	// Environment variable to enable experimental hybrid-based optimization
	// When "on", runs both capacity analyzer and model-based optimizer with arbitration
	// When "model-only" runs model-based optimizer only
	// When "off" or unset, runs capacity analyzer only (default, reactive mode)
	EnvExperimentalHybridOptimization = "EXPERIMENTAL_HYBRID_OPTIMIZATION"
)

var (
	// ServiceMonitor GVK for watching controller's own metrics ServiceMonitor
	serviceMonitorGVK = schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "ServiceMonitor",
	}
)

func initMetricsEmitter() {
	logger.Log.Info("Creating metrics emitter instance")
	// Force initialization of metrics by creating a metrics emitter
	_ = metrics.NewMetricsEmitter()
	logger.Log.Info("Metrics emitter created successfully")
}

func (r *VariantAutoscalingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	//TODO: move interval to manager.yaml

	interval, err := r.readOptimizationConfig(ctx)
	if err != nil {
		logger.Log.Error(err, "Unable to read optimization config")
		return ctrl.Result{}, err
	}

	// default requeue duration
	requeueDuration := 60 * time.Second

	if interval != "" {
		if requeueDuration, err = time.ParseDuration(interval); err != nil {
			return ctrl.Result{}, err
		}
	}

	if strings.EqualFold(os.Getenv("WVA_SCALE_TO_ZERO"), "true") {
		logger.Log.Info("Scaling to zero is enabled!")
	}

	// Check experimental hybrid optimization flag
	optimizationMode := os.Getenv(EnvExperimentalHybridOptimization)
	enableModelOptimizer := optimizationMode == "on" || optimizationMode == "model-only"
	enableCapacityAnalyzer := optimizationMode == "" || optimizationMode == "off"

	if enableModelOptimizer && enableCapacityAnalyzer {
		logger.Log.Info("Operating in HYBRID mode: capacity analyzer + model-based optimizer with arbitration")
	} else if enableModelOptimizer && !enableCapacityAnalyzer {
		logger.Log.Info("Operating in MODEL-ONLY mode: model-based optimization only")
	} else if !enableModelOptimizer && enableCapacityAnalyzer {
		logger.Log.Info("Operating in CAPACITY-ONLY mode: reactive capacity-based scaling only")
	} else {
		// Invalid environment variable, default to capacity-only
		logger.Log.Info("No optimization mode enabled, defaulting to CAPACITY-ONLY mode")
		enableCapacityAnalyzer = true
	}

	// Get list of all VAs
	var variantAutoscalingList llmdVariantAutoscalingV1alpha1.VariantAutoscalingList
	if err := r.List(ctx, &variantAutoscalingList); err != nil {
		logger.Log.Error(err, "unable to list variantAutoscaling resources")
		return ctrl.Result{}, err
	}

	activeVAs := filterActiveVariantAutoscalings(variantAutoscalingList.Items)

	if len(activeVAs) == 0 {
		logger.Log.Info("No active VariantAutoscalings found, skipping optimization")
		return ctrl.Result{RequeueAfter: requeueDuration}, nil
	}

	// Get capacity scaling configuration (atomic check-and-get prevents race condition)
	capacityConfigMap, configLoaded := r.getCapacityConfigSafe()
	if !configLoaded {
		logger.Log.Warn("Capacity scaling config not loaded yet, using defaults")
	}

	// Group VAs by model for per-model capacity analysis
	modelGroups := r.groupVAsByModel(activeVAs)
	logger.Log.Info("Grouped VAs by model", "modelCount", len(modelGroups), "totalVAs", len(activeVAs))

	// Process each model independently
	allDecisions := make([]interfaces.VariantDecision, 0)
	// Accumulate errors to report all failures, not just the first
	var accumulatedErrors []error
	// Create map with safe pointers (copy slice elements to avoid pointer issues)
	vaMap := make(map[string]*llmdVariantAutoscalingV1alpha1.VariantAutoscaling, len(activeVAs))
	for i := range activeVAs {
		va := activeVAs[i] // Copy to local variable to ensure stable pointer
		vaMap[va.Name] = &va
	}

	for modelID, modelVAs := range modelGroups {
		logger.Log.Info("Processing model", "modelID", modelID, "variantCount", len(modelVAs))

		// PHASE 1: compute capacity analysis and/or model-based optimization

		// STEP 1: Run capacity analysis (if enabled)
		var capacityTargets map[string]int
		var capacityAnalysis *interfaces.ModelCapacityAnalysis
		var variantStates []interfaces.VariantReplicaState

		if enableCapacityAnalyzer {
			// Get capacity config for this model (with fallback to default)
			capacityConfig := interfaces.DefaultCapacityScalingConfig()
			if len(modelVAs) > 0 {
				modelConfig := r.getCapacityScalingConfigForVariant(capacityConfigMap, modelID, modelVAs[0].Namespace)
				capacityConfig.Merge(modelConfig)
			}

			capacityTargets, capacityAnalysis, variantStates, err = r.runCapacityAnalysis(ctx, modelID, modelVAs, capacityConfig)
			if err != nil {
				logger.Log.Error(err, "Capacity analysis failed for model, continuing with model-based if enabled", "modelID", modelID)
				// Continue with model-based approach if enabled, as per requirement #1
				if !enableModelOptimizer {
					// In capacity-only mode, if capacity fails, skip this model
					accumulatedErrors = append(accumulatedErrors, fmt.Errorf("capacity analysis failed for model %s: %w", modelID, err))
					continue
				}
				// In hybrid mode, continue to run model-based (capacity failed but we can still run optimizer)
				accumulatedErrors = append(accumulatedErrors, fmt.Errorf("capacity analysis failed for model %s (continuing with model-based): %w", modelID, err))
			}
		}

		var finalDecisions []interfaces.VariantDecision

		modelBasedTargets := make(map[string]int)
		if enableModelOptimizer {
			// Read configs needed for model-based optimizer
			acceleratorCm, err := r.readAcceleratorConfig(ctx, "accelerator-unit-costs", configMapNamespace)
			if err != nil {
				logger.Log.Error(err, "unable to read accelerator configMap, skipping model-based optimization for this model")
				accumulatedErrors = append(accumulatedErrors, fmt.Errorf("failed to read accelerator config for model %s: %w", modelID, err))
				// Fall back to capacity-only for this model
				if capacityAnalysis != nil {
					finalDecisions = convertCapacityTargetsToDecisions(capacityTargets, capacityAnalysis, variantStates)
				} else {
					// Capacity also failed - activate safety net
					logger.Log.Warn("Config read failed and capacity unavailable, activating safety net", "modelID", modelID)
					r.emitSafetyNetMetrics(ctx, modelVAs, vaMap)
				}
				allDecisions = append(allDecisions, finalDecisions...)
				continue
			}

			serviceClassCm, err := r.readServiceClassConfig(ctx, "service-classes-config", configMapNamespace)
			if err != nil {
				logger.Log.Error(err, "unable to read serviceclass configMap, skipping model-based optimization for this model")
				accumulatedErrors = append(accumulatedErrors, fmt.Errorf("failed to read service class config for model %s: %w", modelID, err))
				// Fall back to capacity-only for this model
				if capacityAnalysis != nil {
					finalDecisions = convertCapacityTargetsToDecisions(capacityTargets, capacityAnalysis, variantStates)
				} else {
					// Capacity also failed - activate safety net
					logger.Log.Warn("Config read failed and capacity unavailable, activating safety net", "modelID", modelID)
					r.emitSafetyNetMetrics(ctx, modelVAs, vaMap)
				}
				allDecisions = append(allDecisions, finalDecisions...)
				continue
			}

			// Create system data and run optimizer
			systemData := utils.CreateSystemData(acceleratorCm, serviceClassCm)
			updateList, prepareVaMap, allAnalyzerResponses, err := r.prepareVariantAutoscalings(ctx, modelVAs, acceleratorCm, serviceClassCm, systemData)
			if err != nil {
				logger.Log.Error(err, "failed to prepare variant autoscalings, falling back to capacity-only")
				accumulatedErrors = append(accumulatedErrors, fmt.Errorf("failed to prepare variant autoscalings for model %s: %w", modelID, err))
				if capacityAnalysis != nil {
					finalDecisions = convertCapacityTargetsToDecisions(capacityTargets, capacityAnalysis, variantStates)
				} else {
					// Capacity also failed - activate safety net
					logger.Log.Warn("Variant preparation failed and capacity unavailable, activating safety net", "modelID", modelID)
					r.emitSafetyNetMetrics(ctx, modelVAs, vaMap)
				}
				allDecisions = append(allDecisions, finalDecisions...)
				continue
			}

			// Run model analyzer
			system := inferno.NewSystem()
			optimizerSpec := system.SetFromSpec(&systemData.Spec)
			optimizer := infernoSolver.NewOptimizerFromSpec(optimizerSpec)
			manager := infernoManager.NewManager(system, optimizer)

			modelAnalyzer := analyzer.NewModelAnalyzer(system)
			for _, s := range system.Servers() {
				modelAnalyzeResponse := modelAnalyzer.AnalyzeModel(ctx, *prepareVaMap[s.Name()])
				if len(modelAnalyzeResponse.Allocations) == 0 {
					logger.Log.Info("No potential allocations found for server", "serverName", s.Name())
					continue
				}
				allAnalyzerResponses[s.Name()] = modelAnalyzeResponse
			}

			// Run optimizer
			engine := variantAutoscalingOptimizer.NewVariantAutoscalingsEngine(manager, system)
			optimizedAllocation, err := engine.Optimize(ctx, *updateList, allAnalyzerResponses)
			if err != nil {
				logger.Log.Error(err, "Model-based optimization failed, falling back to capacity-only")
				accumulatedErrors = append(accumulatedErrors, fmt.Errorf("model-based optimization failed for model %s: %w", modelID, err))
				if capacityAnalysis != nil {
					finalDecisions = convertCapacityTargetsToDecisions(capacityTargets, capacityAnalysis, variantStates)
				} else {
					// Both capacity and model-based failed - activate safety net
					logger.Log.Warn("Both capacity and model-based failed, activating safety net", "modelID", modelID)
					r.emitSafetyNetMetrics(ctx, modelVAs, vaMap)
				}
				allDecisions = append(allDecisions, finalDecisions...)
				continue
			}

			// Extract model-based targets for this model's variants

			for _, va := range modelVAs {
				if alloc, ok := optimizedAllocation[va.Name]; ok {
					modelBasedTargets[va.Name] = alloc.NumReplicas
				}
			}

			logger.Log.Info("Model-based optimization completed",
				"modelID", modelID,
				"modelBasedTargets", modelBasedTargets)

		}

		// PHASE 2: Accumulate final decisions

		if enableCapacityAnalyzer && !enableModelOptimizer {
			// CAPACITY-ONLY MODE

			if capacityAnalysis != nil {
				finalDecisions = convertCapacityTargetsToDecisions(capacityTargets, capacityAnalysis, variantStates)
				logger.Log.Info("Capacity-only decisions made",
					"modelID", modelID,
					"decisionCount", len(finalDecisions))
			} else {
				logger.Log.Error(nil, "Capacity analysis failed and model-based disabled, activating safety net", "modelID", modelID)
				accumulatedErrors = append(accumulatedErrors, fmt.Errorf("capacity analysis failed for model %s in capacity-only mode", modelID))
				// SAFETY NET: Emit fallback metrics to prevent HPA from using stale data
				r.emitSafetyNetMetrics(ctx, modelVAs, vaMap)
				continue
			}
		} else if enableCapacityAnalyzer && enableModelOptimizer && capacityAnalysis != nil && len(capacityTargets) > 0 {
			// HYBRID MODE: Arbitrate between capacity and model-based targets - only if capacity analysis succeeded
			if capacityAnalysis != nil && len(capacityTargets) > 0 {
				capacityAnalyzer := capacity.NewAnalyzer()
				finalDecisions = capacityAnalyzer.ArbitrateWithModelBased(
					capacityAnalysis,
					capacityTargets,
					modelBasedTargets,
					variantStates,
				)
				logger.Log.Info("Arbitration completed",
					"modelID", modelID,
					"decisionCount", len(finalDecisions))
			} else {

			}
		} else if enableModelOptimizer {
			// MODEL-ONLY MODE: Capacity failed but model-based succeeded, or capacity analysis unavailable - use model-based only
			logger.Log.Warn("Capacity analysis unavailable, using model-based targets only", "modelID", modelID)
			for _, va := range modelVAs {
				if targetReplicas, ok := modelBasedTargets[va.Name]; ok {
					state := interfaces.VariantReplicaState{VariantName: va.Name, CurrentReplicas: va.Status.CurrentAlloc.NumReplicas}
					var action interfaces.CapacityAction
					if targetReplicas > state.CurrentReplicas {
						action = interfaces.ActionScaleUp
					} else if targetReplicas < state.CurrentReplicas {
						action = interfaces.ActionScaleDown
					} else {
						action = interfaces.ActionNoChange
					}

					finalDecisions = append(finalDecisions, interfaces.VariantDecision{
						VariantName:        va.Name,
						Namespace:          va.Namespace,
						ModelID:            modelID,
						CurrentReplicas:    state.CurrentReplicas,
						TargetReplicas:     targetReplicas,
						Action:             action,
						ModelBasedDecision: true,
						CapacityBased:      false,
						Reason:             "model-based only (capacity unavailable)",
					})
				}
			}
		} else {
			// not possible. Skip
		}

		allDecisions = append(allDecisions, finalDecisions...)
	}

	// Check for accumulated errors during model processing
	var finalError error
	if len(accumulatedErrors) > 0 {
		// Format all errors into a single message
		errorMessages := make([]string, len(accumulatedErrors))
		for i, err := range accumulatedErrors {
			errorMessages[i] = err.Error()
		}
		finalError = fmt.Errorf("%d model(s) failed processing: %s", len(accumulatedErrors), strings.Join(errorMessages, "; "))
		logger.Log.Error(finalError, "Some models failed during reconciliation", "failureCount", len(accumulatedErrors))
	}

	// PHASE 3: Apply all decisions
	if len(allDecisions) > 0 {
		logger.Log.Info("Applying scaling decisions", "totalDecisions", len(allDecisions))
		if err := r.applyCapacityDecisions(ctx, allDecisions, vaMap); err != nil {
			logger.Log.Error(err, "failed to apply capacity decisions")
			return ctrl.Result{RequeueAfter: requeueDuration}, nil
		}
	} else {
		logger.Log.Info("No scaling decisions to apply")
	}

	if finalError == nil {
		logger.Log.Info("Reconciliation completed successfully",
			"mode", func() string {

				if enableModelOptimizer && enableCapacityAnalyzer {
					return "hybrid"
				} else if enableModelOptimizer {
					return "model-only"
				}
				return "capacity-only"
			}(),
			"modelsProcessed", len(modelGroups),
			"decisionsApplied", len(allDecisions))
	} else {
		logger.Log.Warn("Reconciliation completed with errors",
			"mode", func() string {
				if enableModelOptimizer && enableCapacityAnalyzer {
					return "hybrid"
				} else if enableModelOptimizer {
					return "model-only"
				}
				return "capacity-only"
			}(),
			"modelsProcessed", len(modelGroups),
			"modelsFailed", len(accumulatedErrors),
			"decisionsApplied", len(allDecisions))
	}

	return ctrl.Result{RequeueAfter: requeueDuration}, finalError
}

// filterActiveVariantAutoscalings returns only those VAs not marked for deletion.
func filterActiveVariantAutoscalings(items []llmdVariantAutoscalingV1alpha1.VariantAutoscaling) []llmdVariantAutoscalingV1alpha1.VariantAutoscaling {
	active := make([]llmdVariantAutoscalingV1alpha1.VariantAutoscaling, 0, len(items))
	for _, va := range items {
		if va.DeletionTimestamp.IsZero() {
			active = append(active, va)
		} else {
			logger.Log.Info("skipping deleted variantAutoscaling - ", "variantAutoscaling-name: ", va.Name)
		}
	}
	return active
}

// groupVAsByModel groups VariantAutoscalings by ModelID for per-model capacity analysis.
// CRD validation ensures ModelID is not empty and all required fields are valid.
func (r *VariantAutoscalingReconciler) groupVAsByModel(
	vas []llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
) map[string][]llmdVariantAutoscalingV1alpha1.VariantAutoscaling {
	groups := make(map[string][]llmdVariantAutoscalingV1alpha1.VariantAutoscaling)
	for _, va := range vas {
		modelID := va.Spec.ModelID
		groups[modelID] = append(groups[modelID], va)
	}
	return groups
}

// buildVariantStates extracts current and desired replica counts from VAs for capacity analysis.
func (r *VariantAutoscalingReconciler) buildVariantStates(
	ctx context.Context,
	vas []llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
) ([]interfaces.VariantReplicaState, error) {
	states := make([]interfaces.VariantReplicaState, 0, len(vas))

	for _, va := range vas {
		// Get current replicas from deployment
		var deploy appsv1.Deployment
		if err := utils.GetDeploymentWithBackoff(ctx, r.Client, va.Name, va.Namespace, &deploy); err != nil {
			logger.Log.Warn("Failed to get deployment for VA, using status", "name", va.Name, "error", err)
			// Fallback to status if deployment fetch fails
			states = append(states, interfaces.VariantReplicaState{
				VariantName:     va.Name,
				CurrentReplicas: va.Status.CurrentAlloc.NumReplicas,
				DesiredReplicas: va.Status.DesiredOptimizedAlloc.NumReplicas,
			})
			continue
		}

		currentReplicas := int(deploy.Status.Replicas)
		if currentReplicas == 0 && deploy.Spec.Replicas != nil {
			currentReplicas = int(*deploy.Spec.Replicas)
		}

		states = append(states, interfaces.VariantReplicaState{
			VariantName:     va.Name,
			CurrentReplicas: currentReplicas,
			DesiredReplicas: va.Status.DesiredOptimizedAlloc.NumReplicas,
		})
	}

	return states, nil
}

// convertCapacityTargetsToDecisions converts capacity-only targets to VariantDecisions.
// Used when model-based optimizer is disabled (capacity-only mode).
func convertCapacityTargetsToDecisions(
	capacityTargets map[string]int,
	capacityAnalysis *interfaces.ModelCapacityAnalysis,
	variantStates []interfaces.VariantReplicaState,
) []interfaces.VariantDecision {
	decisions := make([]interfaces.VariantDecision, 0, len(capacityTargets))

	// Build variant analysis map for quick lookup
	vaMap := make(map[string]*interfaces.VariantCapacityAnalysis)
	for i := range capacityAnalysis.VariantAnalyses {
		va := &capacityAnalysis.VariantAnalyses[i]
		vaMap[va.VariantName] = va
	}

	// Build state map for quick lookup
	stateMap := make(map[string]interfaces.VariantReplicaState)
	for _, state := range variantStates {
		stateMap[state.VariantName] = state
	}

	for variantName, targetReplicas := range capacityTargets {
		state := stateMap[variantName]
		va := vaMap[variantName]

		var action interfaces.CapacityAction
		if targetReplicas > state.CurrentReplicas {
			action = interfaces.ActionScaleUp
		} else if targetReplicas < state.CurrentReplicas {
			action = interfaces.ActionScaleDown
		} else {
			action = interfaces.ActionNoChange
		}

		decision := interfaces.VariantDecision{
			VariantName:        variantName,
			Namespace:          capacityAnalysis.Namespace,
			ModelID:            capacityAnalysis.ModelID,
			CurrentReplicas:    state.CurrentReplicas,
			TargetReplicas:     targetReplicas,
			DesiredReplicas:    state.DesiredReplicas,
			Action:             action,
			CapacityBased:      true,
			CapacityOnly:       true,
			ModelBasedDecision: false,
			SafetyOverride:     false,
			Reason:             "capacity-only mode: " + string(action),
		}

		if va != nil {
			decision.AcceleratorName = va.AcceleratorName
			decision.Cost = va.Cost
		}

		decisions = append(decisions, decision)
	}

	return decisions
}

// runCapacityAnalysis performs capacity analysis for a model and returns capacity targets.
func (r *VariantAutoscalingReconciler) runCapacityAnalysis(
	ctx context.Context,
	modelID string,
	modelVAs []llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
	capacityConfig interfaces.CapacityScalingConfig,
) (map[string]int, *interfaces.ModelCapacityAnalysis, []interfaces.VariantReplicaState, error) {
	if len(modelVAs) == 0 {
		return nil, nil, nil, fmt.Errorf("no VAs provided for model %s", modelID)
	}

	namespace := modelVAs[0].Namespace // All VAs of same model are in same namespace

	// Build variant costs map from VA specs
	variantCosts := make(map[string]float64)
	for _, va := range modelVAs {
		cost := 10.0 // default
		if va.Spec.VariantCost != "" {
			if parsedCost, err := strconv.ParseFloat(va.Spec.VariantCost, 64); err == nil {
				cost = parsedCost
			}
		}
		variantCosts[va.Name] = cost
	}

	// Collect capacity metrics from Prometheus
	metricsCollector := collector.NewCapacityMetricsCollector(r.PromAPI)
	replicaMetrics, err := metricsCollector.CollectReplicaMetrics(ctx, modelID, namespace, variantCosts)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to collect capacity metrics for model %s: %w", modelID, err)
	}

	logger.Log.Debug("Collected capacity metrics",
		"modelID", modelID,
		"namespace", namespace,
		"metricsCount", len(replicaMetrics))

	// Analyze capacity across all variants
	capacityAnalyzer := capacity.NewAnalyzer()
	capacityAnalysis, err := capacityAnalyzer.AnalyzeModelCapacity(ctx, modelID, namespace, replicaMetrics, capacityConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to analyze capacity for model %s: %w", modelID, err)
	}

	logger.Log.Info("Capacity analysis completed",
		"modelID", modelID,
		"totalReplicas", capacityAnalysis.TotalReplicas,
		"nonSaturated", capacityAnalysis.NonSaturatedCount,
		"shouldScaleUp", capacityAnalysis.ShouldScaleUp,
		"scaleDownSafe", capacityAnalysis.ScaleDownSafe)

	// Build variant states (current and desired replicas)
	variantStates, err := r.buildVariantStates(ctx, modelVAs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to build variant states for model %s: %w", modelID, err)
	}

	// Calculate capacity-based targets
	capacityTargets := capacityAnalyzer.CalculateCapacityTargets(capacityAnalysis, variantStates)

	logger.Log.Debug("Capacity targets calculated",
		"modelID", modelID,
		"targets", capacityTargets)

	return capacityTargets, capacityAnalysis, variantStates, nil
}

// applyCapacityDecisions updates VA status and emits metrics based on capacity decisions.
func (r *VariantAutoscalingReconciler) applyCapacityDecisions(
	ctx context.Context,
	decisions []interfaces.VariantDecision,
	vaMap map[string]*llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
) error {
	for _, decision := range decisions {
		va, ok := vaMap[decision.VariantName]
		if !ok {
			logger.Log.Warn("VA not found for decision, skipping", "variant", decision.VariantName)
			continue
		}

		// Fetch latest version from API server to avoid conflicts
		var updateVa llmdVariantAutoscalingV1alpha1.VariantAutoscaling
		if err := utils.GetVariantAutoscalingWithBackoff(ctx, r.Client, va.Name, va.Namespace, &updateVa); err != nil {
			logger.Log.Error(err, "failed to get latest VA from API server", "name", va.Name)
			continue
		}

		// Preserve existing current allocation
		// (will be updated by metrics collector in next iteration)
		if updateVa.Status.CurrentAlloc.Accelerator == "" {
			updateVa.Status.CurrentAlloc = va.Status.CurrentAlloc
		}

		// Update DesiredOptimizedAlloc with capacity decision
		updateVa.Status.DesiredOptimizedAlloc = llmdVariantAutoscalingV1alpha1.OptimizedAlloc{
			NumReplicas: decision.TargetReplicas,
			Accelerator: decision.AcceleratorName,
			LastRunTime: metav1.Now(),
		}
		updateVa.Status.Actuation.Applied = false

		// Set condition based on decision characteristics
		if decision.SafetyOverride {
			llmdVariantAutoscalingV1alpha1.SetCondition(&updateVa,
				llmdVariantAutoscalingV1alpha1.TypeOptimizationReady,
				metav1.ConditionTrue,
				"CapacitySafetyOverride",
				fmt.Sprintf("Capacity safety override: %s", decision.Reason))
		} else if decision.CapacityOnly {
			llmdVariantAutoscalingV1alpha1.SetCondition(&updateVa,
				llmdVariantAutoscalingV1alpha1.TypeOptimizationReady,
				metav1.ConditionTrue,
				"CapacityOnlyMode",
				fmt.Sprintf("Capacity-only decision: %s (target: %d replicas)", decision.Reason, decision.TargetReplicas))
		} else {
			llmdVariantAutoscalingV1alpha1.SetCondition(&updateVa,
				llmdVariantAutoscalingV1alpha1.TypeOptimizationReady,
				metav1.ConditionTrue,
				llmdVariantAutoscalingV1alpha1.ReasonOptimizationSucceeded,
				fmt.Sprintf("Hybrid mode: %s (target: %d replicas)", decision.Reason, decision.TargetReplicas))
		}

		// Emit metrics for external autoscalers (HPA, etc.)
		act := actuator.NewActuator(r.Client)
		if err := act.EmitMetrics(ctx, &updateVa); err != nil {
			logger.Log.Error(err, "failed to emit metrics for external autoscalers", "variant", updateVa.Name)
		} else {
			logger.Log.Info("Successfully emitted metrics for external autoscalers",
				"variant", updateVa.Name,
				"targetReplicas", decision.TargetReplicas,
				"accelerator", decision.AcceleratorName,
				"mode", func() string {
					if decision.CapacityOnly {
						return "capacity-only"
					}
					return "hybrid"
				}())
			updateVa.Status.Actuation.Applied = true
		}

		// Update VA status with backoff
		if err := utils.UpdateStatusWithBackoff(ctx, r.Client, &updateVa, utils.StandardBackoff, "VariantAutoscaling"); err != nil {
			logger.Log.Error(err, "failed to update VA status after retries", "name", updateVa.Name)
			continue
		}

		logger.Log.Info("Applied capacity decision",
			"variant", decision.VariantName,
			"action", decision.Action,
			"current", decision.CurrentReplicas,
			"target", decision.TargetReplicas,
			"reason", decision.Reason)
	}

	return nil
}

// emitSafetyNetMetrics emits fallback metrics when capacity analysis fails.
// Strategy: Use previous desired replicas if available, otherwise use current replicas.
// This prevents HPA from using completely stale metrics and provides a safe no-op signal.
func (r *VariantAutoscalingReconciler) emitSafetyNetMetrics(
	ctx context.Context,
	modelVAs []llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
	vaMap map[string]*llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
) {
	act := actuator.NewActuator(r.Client)

	for _, va := range modelVAs {
		// Get latest version from API server
		var updateVa llmdVariantAutoscalingV1alpha1.VariantAutoscaling
		if err := utils.GetVariantAutoscalingWithBackoff(ctx, r.Client, va.Name, va.Namespace, &updateVa); err != nil {
			logger.Log.Error(err, "Safety net: failed to get latest VA from API server", "name", va.Name)
			continue
		}

		// Determine fallback desired replicas
		var desiredReplicas int32
		var fallbackSource string

		// Strategy 1: Use previous desired replicas if available
		if updateVa.Status.DesiredOptimizedAlloc.NumReplicas > 0 {
			desiredReplicas = int32(updateVa.Status.DesiredOptimizedAlloc.NumReplicas)
			fallbackSource = "previous-desired"
		} else {
			// Strategy 2: Use current replicas from deployment (safe no-op)
			currentReplicas, err := act.GetCurrentDeploymentReplicas(ctx, &updateVa)
			if err != nil {
				logger.Log.Warn("Safety net: failed to get current replicas, using VA status",
					"variant", updateVa.Name, "error", err)
				currentReplicas = int32(updateVa.Status.CurrentAlloc.NumReplicas)
			}
			desiredReplicas = currentReplicas
			fallbackSource = "current-replicas"
		}

		// Get current replicas for metric emission
		currentReplicas, err := act.GetCurrentDeploymentReplicas(ctx, &updateVa)
		if err != nil {
			logger.Log.Warn("Safety net: failed to get current replicas for metrics",
				"variant", updateVa.Name, "error", err)
			currentReplicas = int32(updateVa.Status.CurrentAlloc.NumReplicas)
		}

		// Determine accelerator (use existing or fall back to status)
		accelerator := updateVa.Status.DesiredOptimizedAlloc.Accelerator
		if accelerator == "" {
			accelerator = updateVa.Status.CurrentAlloc.Accelerator
		}
		if accelerator == "" {
			accelerator = "unknown"
		}

		// Emit safety net metrics
		if err := act.MetricsEmitter.EmitReplicaMetrics(
			ctx,
			&updateVa,
			currentReplicas,
			desiredReplicas,
			accelerator,
		); err != nil {
			logger.Log.Error(err, "Safety net: failed to emit metrics", "variant", updateVa.Name)
			continue
		}

		logger.Log.Info("Safety net activated: emitted fallback metrics",
			"variant", updateVa.Name,
			"currentReplicas", currentReplicas,
			"desiredReplicas", desiredReplicas,
			"accelerator", accelerator,
			"fallbackSource", fallbackSource)
	}
}

// prepareVariantAutoscalings collects and prepares all data for optimization.
func (r *VariantAutoscalingReconciler) prepareVariantAutoscalings(
	ctx context.Context,
	activeVAs []llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
	acceleratorCm map[string]map[string]string,
	serviceClassCm map[string]string,
	systemData *infernoConfig.SystemData,
) (*llmdVariantAutoscalingV1alpha1.VariantAutoscalingList, map[string]*llmdVariantAutoscalingV1alpha1.VariantAutoscaling, map[string]*interfaces.ModelAnalyzeResponse, error) {
	var updateList llmdVariantAutoscalingV1alpha1.VariantAutoscalingList
	allAnalyzerResponses := make(map[string]*interfaces.ModelAnalyzeResponse)
	vaMap := make(map[string]*llmdVariantAutoscalingV1alpha1.VariantAutoscaling)

	for _, va := range activeVAs {
		modelName := va.Spec.ModelID
		if modelName == "" {
			logger.Log.Info("variantAutoscaling missing modelName label, skipping optimization - ", "variantAutoscaling-name: ", va.Name)
			continue
		}

		entry, className, err := utils.FindModelSLO(serviceClassCm, modelName)
		if err != nil {
			logger.Log.Error(err, "failed to locate SLO for model - ", "variantAutoscaling-name: ", va.Name, "modelName: ", modelName)
			continue
		}
		logger.Log.Info("Found SLO for model - ", "model: ", modelName, ", class: ", className, ", slo-tpot: ", entry.SLOTPOT, ", slo-ttft: ", entry.SLOTTFT)

		for _, modelAcceleratorProfile := range va.Spec.ModelProfile.Accelerators {
			if utils.AddModelAcceleratorProfileToSystemData(systemData, modelName, &modelAcceleratorProfile) != nil {
				logger.Log.Error("variantAutoscaling bad model accelerator profile data, skipping optimization - ", "variantAutoscaling-name: ", va.Name)
				continue
			}
		}

		accName := va.Labels["inference.optimization/acceleratorName"]
		acceleratorCostVal, ok := acceleratorCm[accName]["cost"]
		if !ok {
			logger.Log.Error("variantAutoscaling missing accelerator cost in configMap, skipping optimization - ", "variantAutoscaling-name: ", va.Name)
			continue
		}
		acceleratorCostValFloat, err := strconv.ParseFloat(acceleratorCostVal, 32)
		if err != nil {
			logger.Log.Error("variantAutoscaling unable to parse accelerator cost in configMap, skipping optimization - ", "variantAutoscaling-name: ", va.Name)
			continue
		}

		var deploy appsv1.Deployment
		err = utils.GetDeploymentWithBackoff(ctx, r.Client, va.Name, va.Namespace, &deploy)
		if err != nil {
			logger.Log.Error(err, "failed to get Deployment after retries - ", "variantAutoscaling-name: ", va.Name)
			continue
		}

		var updateVA llmdVariantAutoscalingV1alpha1.VariantAutoscaling
		err = utils.GetVariantAutoscalingWithBackoff(ctx, r.Client, deploy.Name, deploy.Namespace, &updateVA)
		if err != nil {
			logger.Log.Error(err, "unable to get variantAutoscaling for deployment - ", "deployment-name: ", deploy.Name, ", namespace: ", deploy.Namespace)
			continue
		}

		// Set ownerReference early, before metrics validation, to ensure it's always set
		// This ensures the VA will be garbage collected when the Deployment is deleted
		if !metav1.IsControlledBy(&updateVA, &deploy) {
			original := updateVA.DeepCopy()
			err := controllerutil.SetControllerReference(&deploy, &updateVA, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
			if err != nil {
				logger.Log.Error(err, "failed to set ownerReference - ", "variantAutoscaling-name: ", updateVA.Name)
				continue
			}

			// Patch metadata change (ownerReferences)
			patch := client.MergeFrom(original)
			if err := r.Patch(ctx, &updateVA, patch); err != nil {
				logger.Log.Error(err, "failed to patch ownerReference - ", "variantAutoscaling-name: ", updateVA.Name)
				continue
			}
			logger.Log.Info("Set ownerReference on VariantAutoscaling - ", "variantAutoscaling-name: ", updateVA.Name, ", owner: ", deploy.Name)
		}

		// Validate metrics availability before collecting metrics
		metricsValidation := collector.ValidateMetricsAvailability(ctx, r.PromAPI, modelName, deploy.Namespace)

		// Update MetricsAvailable condition based on validation result
		if metricsValidation.Available {
			llmdVariantAutoscalingV1alpha1.SetCondition(&updateVA,
				llmdVariantAutoscalingV1alpha1.TypeMetricsAvailable,
				metav1.ConditionTrue,
				metricsValidation.Reason,
				metricsValidation.Message)
		} else {
			// Metrics unavailable - just log and skip (don't update status yet to avoid CRD validation errors)
			// Conditions will be set properly once metrics become available or after first successful collection
			logger.Log.Warnw("Metrics unavailable, skipping optimization for variant",
				"variant", updateVA.Name,
				"namespace", updateVA.Namespace,
				"model", modelName,
				"reason", metricsValidation.Reason,
				"troubleshooting", metricsValidation.Message)
			continue
		}

		currentAllocation, err := collector.AddMetricsToOptStatus(ctx, &updateVA, deploy, acceleratorCostValFloat, r.PromAPI)
		if err != nil {
			logger.Log.Error(err, "unable to fetch metrics, skipping this variantAutoscaling loop")
			// Don't update status here - will be updated in next reconcile when metrics are available
			continue
		}
		updateVA.Status.CurrentAlloc = currentAllocation

		if err := utils.AddServerInfoToSystemData(systemData, &updateVA, className); err != nil {
			logger.Log.Info("variantAutoscaling bad deployment server data, skipping optimization - ", "variantAutoscaling-name: ", updateVA.Name)
			continue
		}

		vaFullName := utils.FullName(va.Name, va.Namespace)
		updateList.Items = append(updateList.Items, updateVA)
		vaMap[vaFullName] = &va
	}
	return &updateList, vaMap, allAnalyzerResponses, nil
}

// isCapacityScalingConfigMap checks if object is the capacity-scaling-config ConfigMap.
func (r *VariantAutoscalingReconciler) isCapacityScalingConfigMap(obj client.Object) bool {
	return obj.GetName() == "capacity-scaling-config" &&
		obj.GetNamespace() == configMapNamespace
}

// handleCapacityConfigMapEvent handles capacity-scaling-config ConfigMap events.
// Reloads cache and triggers reconciliation of all VariantAutoscaling resources.
func (r *VariantAutoscalingReconciler) handleCapacityConfigMapEvent(ctx context.Context, obj client.Object) []reconcile.Request {
	if !r.isCapacityScalingConfigMap(obj) {
		return nil
	}

	// Reload cache when ConfigMap changes
	logger.Log.Info("Capacity scaling ConfigMap changed, reloading cache")
	if err := r.updateCapacityConfigCache(ctx); err != nil {
		logger.Log.Error(err, "Failed to reload capacity scaling config cache")
		// Continue to trigger reconciliation even if reload fails (will use existing cache or defaults)
	}

	// Trigger reconciliation for all VariantAutoscaling resources
	vaList := &llmdVariantAutoscalingV1alpha1.VariantAutoscalingList{}
	if err := r.List(ctx, vaList); err != nil {
		logger.Log.Error(err, "Failed to list VariantAutoscaling resources")
		return nil
	}

	requests := make([]reconcile.Request, len(vaList.Items))
	for i, va := range vaList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      va.Name,
				Namespace: va.Namespace,
			},
		}
	}

	logger.Log.Info("Triggering reconciliation for all VariantAutoscaling resources due to ConfigMap change",
		"count", len(requests))

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *VariantAutoscalingReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Initialize metrics
	initMetricsEmitter()

	// Configure Prometheus client using flexible configuration with TLS support
	promConfig, err := r.getPrometheusConfig(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get Prometheus configuration: %w", err)
	}

	// ensure we have a valid configuration
	if promConfig == nil {
		return fmt.Errorf("no Prometheus configuration found - this should not happen")
	}

	// Always validate TLS configuration since HTTPS is required
	if err := utils.ValidateTLSConfig(promConfig); err != nil {
		logger.Log.Error(err, "TLS configuration validation failed - HTTPS is required")
		return fmt.Errorf("TLS configuration validation failed: %w", err)
	}

	logger.Log.Info("Initializing Prometheus client -> ", "address: ", promConfig.BaseURL, " tls_enabled: true")

	// Create Prometheus client with TLS support
	promClientConfig, err := utils.CreatePrometheusClientConfig(promConfig)
	if err != nil {
		return fmt.Errorf("failed to create prometheus client config: %w", err)
	}

	promClient, err := api.NewClient(*promClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create prometheus client: %w", err)
	}

	r.PromAPI = promv1.NewAPI(promClient)

	// Validate that the API is working by testing a simple query with retry logic
	if err := utils.ValidatePrometheusAPI(context.Background(), r.PromAPI); err != nil {
		logger.Log.Error(err, "CRITICAL: Failed to connect to Prometheus - Inferno requires Prometheus connectivity for autoscaling decisions")
		return fmt.Errorf("critical: failed to validate Prometheus API connection - autoscaling functionality requires Prometheus: %w", err)
	}
	logger.Log.Info("Prometheus client and API wrapper initialized and validated successfully")

	//logger.Log.Info("Prometheus client initialized (validation skipped)")

	return ctrl.NewControllerManagedBy(mgr).
		For(&llmdVariantAutoscalingV1alpha1.VariantAutoscaling{}).
		// Watch the specific ConfigMap to trigger global reconcile
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				if obj.GetName() == configMapName && obj.GetNamespace() == configMapNamespace {
					return []reconcile.Request{{}}
				}
				return nil
			}),
			// Predicate to filter only the target configmap
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return obj.GetName() == configMapName && obj.GetNamespace() == configMapNamespace
			})),
		).
		// Watch ServiceMonitor for controller's own metrics
		// This enables detection when ServiceMonitor is deleted, which would prevent
		// Prometheus from scraping controller metrics (including optimized replicas).
		Watches(
			func() client.Object {
				serviceMonitorSource := &unstructured.Unstructured{}
				serviceMonitorSource.SetGroupVersionKind(serviceMonitorGVK)
				return serviceMonitorSource
			}(),
			handler.EnqueueRequestsFromMapFunc(r.handleServiceMonitorEvent),
			// Predicate to filter only the target ServiceMonitor
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return obj.GetName() == serviceMonitorName && obj.GetNamespace() == configMapNamespace
			})),
		).
		// Watch capacity-scaling-config ConfigMap to reload cache on changes
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.handleCapacityConfigMapEvent),
			// Predicate to filter only the capacity-scaling-config ConfigMap
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return r.isCapacityScalingConfigMap(obj)
			})),
		).
		Named("variantAutoscaling").
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				gvk := e.ObjectNew.GetObjectKind().GroupVersionKind()
				// Allow Update events for ConfigMap (needed to trigger reconcile on config changes)
				if gvk.Kind == "ConfigMap" && gvk.Group == "" {
					return true
				}
				// Allow Update events for ServiceMonitor when deletionTimestamp is set
				// (finalizers cause deletion to emit Update events with deletionTimestamp)
				if gvk.Group == serviceMonitorGVK.Group && gvk.Kind == serviceMonitorGVK.Kind {
					// Check if deletionTimestamp was just set (deletion started)
					if deletionTimestamp := e.ObjectNew.GetDeletionTimestamp(); deletionTimestamp != nil && !deletionTimestamp.IsZero() {
						// Check if this is a newly set deletion timestamp
						oldDeletionTimestamp := e.ObjectOld.GetDeletionTimestamp()
						if oldDeletionTimestamp == nil || oldDeletionTimestamp.IsZero() {
							return true // Deletion just started
						}
					}
				}
				// Block Update events for VariantAutoscaling resource.
				// The controller reconciles all VariantAutoscaling resources periodically (every 60s by default),
				// so individual resource update events would only cause unnecessary reconciles without benefit.
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				gvk := e.Object.GetObjectKind().GroupVersionKind()
				// Allow Delete events for ServiceMonitor (for immediate deletion detection)
				if gvk.Group == serviceMonitorGVK.Group && gvk.Kind == serviceMonitorGVK.Kind {
					return true
				}
				// Block Delete events for VariantAutoscaling resource.
				// The controller reconciles all VariantAutoscaling resources periodically and filters out
				// deleted resources in filterActiveVariantAutoscalings, so individual delete events
				// would only cause unnecessary reconciles without benefit.
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		}).
		Complete(r)
}

func (r *VariantAutoscalingReconciler) readServiceClassConfig(ctx context.Context, cmName, cmNamespace string) (map[string]string, error) {
	cm := corev1.ConfigMap{}
	err := utils.GetConfigMapWithBackoff(ctx, r.Client, cmName, cmNamespace, &cm)
	if err != nil {
		return nil, err
	}
	return cm.Data, nil
}

func (r *VariantAutoscalingReconciler) readAcceleratorConfig(ctx context.Context, cmName, cmNamespace string) (map[string]map[string]string, error) {
	cm := corev1.ConfigMap{}
	err := utils.GetConfigMapWithBackoff(ctx, r.Client, cmName, cmNamespace, &cm)
	if err != nil {
		return nil, fmt.Errorf("failed to read ConfigMap %s/%s: %w", cmNamespace, cmName, err)
	}
	out := make(map[string]map[string]string)
	for acc, accInfoStr := range cm.Data {
		accInfoMap := make(map[string]string)
		if err := json.Unmarshal([]byte(accInfoStr), &accInfoMap); err != nil {
			return nil, fmt.Errorf("failed to read entry %s in ConfigMap %s/%s: %w", acc, cmNamespace, cmName, err)
		}
		out[acc] = accInfoMap
	}
	return out, nil
}

// getCapacityConfigFromCache retrieves cached config (thread-safe read).
// Returns a copy to prevent external modification.
func (r *VariantAutoscalingReconciler) getCapacityConfigFromCache() map[string]interfaces.CapacityScalingConfig {
	r.capacityConfigCacheMutex.RLock()
	defer r.capacityConfigCacheMutex.RUnlock()

	// Return copy to prevent external modification
	configCopy := make(map[string]interfaces.CapacityScalingConfig, len(r.capacityConfigCache))
	for k, v := range r.capacityConfigCache {
		configCopy[k] = v
	}
	return configCopy
}

// getCapacityConfigSafe atomically retrieves cached config and loaded status (thread-safe).
// Returns a copy of the config map and whether the initial load succeeded.
// This prevents race conditions between checking loaded status and getting the config.
func (r *VariantAutoscalingReconciler) getCapacityConfigSafe() (map[string]interfaces.CapacityScalingConfig, bool) {
	r.capacityConfigCacheMutex.RLock()
	defer r.capacityConfigCacheMutex.RUnlock()

	// Return copy to prevent external modification
	configCopy := make(map[string]interfaces.CapacityScalingConfig, len(r.capacityConfigCache))
	for k, v := range r.capacityConfigCache {
		configCopy[k] = v
	}
	return configCopy, r.capacityConfigLoaded
}

// updateCapacityConfigCache updates the cache (thread-safe write).
// Logs cache update and returns error if read fails.
func (r *VariantAutoscalingReconciler) updateCapacityConfigCache(ctx context.Context) error {
	configs, err := r.readCapacityScalingConfig(ctx, "capacity-scaling-config", configMapNamespace)
	if err != nil {
		return err
	}

	r.capacityConfigCacheMutex.Lock()
	defer r.capacityConfigCacheMutex.Unlock()

	r.capacityConfigCache = configs
	r.capacityConfigLoaded = true

	logger.Log.Info("Capacity scaling config cache updated",
		"entries", len(configs),
		"has_default", configs["default"] != (interfaces.CapacityScalingConfig{}))

	return nil
}

// isCapacityConfigLoaded returns whether the initial config load succeeded (thread-safe).
func (r *VariantAutoscalingReconciler) isCapacityConfigLoaded() bool {
	r.capacityConfigCacheMutex.RLock()
	defer r.capacityConfigCacheMutex.RUnlock()
	return r.capacityConfigLoaded
}

// InitializeCapacityConfigCache performs initial load of capacity scaling config cache.
// Called from main.go during controller startup. Non-fatal if load fails (uses defaults).
func (r *VariantAutoscalingReconciler) InitializeCapacityConfigCache(ctx context.Context) error {
	return r.updateCapacityConfigCache(ctx)
}

// readCapacityScalingConfig reads capacity scaling configuration from ConfigMap.
// Returns default config with warning if ConfigMap is not found.
// Returns a map with key "default" and optional per-model override entries.
// This method is called by updateCapacityConfigCache and should not be called directly.
func (r *VariantAutoscalingReconciler) readCapacityScalingConfig(ctx context.Context, cmName, cmNamespace string) (map[string]interfaces.CapacityScalingConfig, error) {
	cm := corev1.ConfigMap{}
	err := utils.GetConfigMapWithBackoff(ctx, r.Client, cmName, cmNamespace, &cm)

	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Log.Warn("Capacity scaling ConfigMap not found, using hardcoded defaults",
				"configmap", cmName,
				"namespace", cmNamespace)
			// Return default config only
			return map[string]interfaces.CapacityScalingConfig{
				"default": interfaces.DefaultCapacityScalingConfig(),
			}, nil
		}
		return nil, fmt.Errorf("failed to read ConfigMap %s/%s: %w", cmNamespace, cmName, err)
	}

	configs := make(map[string]interfaces.CapacityScalingConfig)

	// Parse all entries
	for key, yamlStr := range cm.Data {
		var config interfaces.CapacityScalingConfig
		if err := yaml.Unmarshal([]byte(yamlStr), &config); err != nil {
			logger.Log.Warn("Failed to parse capacity scaling config entry, skipping",
				"key", key,
				"error", err)
			continue
		}

		// Validate configuration
		if err := config.Validate(); err != nil {
			logger.Log.Warn("Invalid capacity scaling config entry, skipping",
				"key", key,
				"error", err)
			continue
		}

		configs[key] = config
	}

	// Ensure default exists
	if _, ok := configs["default"]; !ok {
		logger.Log.Warn("No 'default' entry in capacity scaling ConfigMap, using hardcoded defaults")
		configs["default"] = interfaces.DefaultCapacityScalingConfig()
	}

	return configs, nil
}

// getCapacityScalingConfigForVariant retrieves config for specific model/namespace with fallback to default.
// It searches for an override entry matching both model_id and namespace fields.
func (r *VariantAutoscalingReconciler) getCapacityScalingConfigForVariant(
	configs map[string]interfaces.CapacityScalingConfig,
	modelID, namespace string,
) interfaces.CapacityScalingConfig {
	// Start with default
	config := configs["default"]

	// Search for matching override
	for key, override := range configs {
		if key == "default" {
			continue
		}

		// Check if this override matches our model_id and namespace
		if override.ModelID == modelID && override.Namespace == namespace {
			config.Merge(override)
			logger.Log.Debug("Applied capacity scaling override",
				"key", key,
				"modelID", modelID,
				"namespace", namespace,
				"config", config)
			break
		}
	}

	return config
}

func (r *VariantAutoscalingReconciler) getPrometheusConfig(ctx context.Context) (*interfaces.PrometheusConfig, error) {
	// Try environment variables first
	config, err := r.getPrometheusConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to get config from environment: %w", err)
	}
	if config != nil {
		return config, nil
	}

	// Try ConfigMap second
	config, err = r.getPrometheusConfigFromConfigMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config from ConfigMap: %w", err)
	}
	if config != nil {
		return config, nil
	}

	// No configuration found
	logger.Log.Warn("No Prometheus configuration found. Please set PROMETHEUS_BASE_URL environment variable or configure via ConfigMap")
	return nil, fmt.Errorf("no Prometheus configuration found. Please set PROMETHEUS_BASE_URL environment variable or configure via ConfigMap")
}

func (r *VariantAutoscalingReconciler) getPrometheusConfigFromEnv() (*interfaces.PrometheusConfig, error) {
	promAddr := os.Getenv("PROMETHEUS_BASE_URL")
	if promAddr == "" {
		return nil, nil // No config found, but not an error
	}

	logger.Log.Info("Using Prometheus configuration from environment variables", "address", promAddr)
	return utils.ParsePrometheusConfigFromEnv(), nil
}

func (r *VariantAutoscalingReconciler) getPrometheusConfigFromConfigMap(ctx context.Context) (*interfaces.PrometheusConfig, error) {
	cm := corev1.ConfigMap{}
	err := utils.GetConfigMapWithBackoff(ctx, r.Client, configMapName, configMapNamespace, &cm)
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap for Prometheus config: %w", err)
	}

	promAddr, exists := cm.Data["PROMETHEUS_BASE_URL"]
	if !exists || promAddr == "" {
		return nil, nil // No config found, but not an error
	}

	logger.Log.Info("Using Prometheus configuration from ConfigMap", "address", promAddr)

	// Create config from ConfigMap data
	config := &interfaces.PrometheusConfig{
		BaseURL: promAddr,
	}

	// Parse TLS configuration from ConfigMap (TLS is always enabled for HTTPS-only support)
	config.InsecureSkipVerify = utils.GetConfigValue(cm.Data, "PROMETHEUS_TLS_INSECURE_SKIP_VERIFY", "") == "true"
	config.CACertPath = utils.GetConfigValue(cm.Data, "PROMETHEUS_CA_CERT_PATH", "")
	config.ClientCertPath = utils.GetConfigValue(cm.Data, "PROMETHEUS_CLIENT_CERT_PATH", "")
	config.ClientKeyPath = utils.GetConfigValue(cm.Data, "PROMETHEUS_CLIENT_KEY_PATH", "")
	config.ServerName = utils.GetConfigValue(cm.Data, "PROMETHEUS_SERVER_NAME", "")

	// Add bearer token if provided
	if bearerToken, exists := cm.Data["PROMETHEUS_BEARER_TOKEN"]; exists && bearerToken != "" {
		config.BearerToken = bearerToken
	}

	return config, nil
}

func (r *VariantAutoscalingReconciler) readOptimizationConfig(ctx context.Context) (interval string, err error) {
	cm := corev1.ConfigMap{}
	err = utils.GetConfigMapWithBackoff(ctx, r.Client, configMapName, configMapNamespace, &cm)

	if err != nil {
		return "", fmt.Errorf("failed to get optimization configmap after retries: %w", err)
	}

	interval = cm.Data["GLOBAL_OPT_INTERVAL"]
	return interval, nil
}

// handleServiceMonitorEvent handles events for the controller's own ServiceMonitor.
// When ServiceMonitor is deleted, it logs an error and emits a Kubernetes event.
// This ensures that administrators are aware when the ServiceMonitor that enables
// Prometheus scraping of controller metrics (including optimized replicas) is missing.
//
// Note: This handler does not enqueue reconcile requests. ServiceMonitor deletion doesn't
// affect the optimization logic (which reads from Prometheus), but it prevents future
// metrics from being scraped. The handler exists solely for observability - logging and
// emitting Kubernetes events to alert operators of the issue.
func (r *VariantAutoscalingReconciler) handleServiceMonitorEvent(ctx context.Context, obj client.Object) []reconcile.Request {
	serviceMonitor, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil
	}

	name := serviceMonitor.GetName()
	namespace := serviceMonitor.GetNamespace()

	// Check if ServiceMonitor is being deleted
	if !serviceMonitor.GetDeletionTimestamp().IsZero() {
		logger.Log.Errorw("ServiceMonitor being deleted - Prometheus will not scrape controller metrics",
			"servicemonitor", name,
			"namespace", namespace,
			"impact", "Actuator will not be able to access optimized replicas metrics",
			"action", "ServiceMonitor must be recreated for metrics scraping to resume")

		// Emit Kubernetes event for observability
		if r.Recorder != nil {
			r.Recorder.Eventf(
				serviceMonitor,
				corev1.EventTypeWarning,
				"ServiceMonitorDeleted",
				"ServiceMonitor %s/%s is being deleted. Prometheus will not scrape controller metrics. Actuator will not be able to access optimized replicas metrics. Please recreate the ServiceMonitor.",
				namespace,
				name,
			)
		}

		// Don't trigger reconciliation - ServiceMonitor deletion doesn't affect optimization logic
		return nil
	}

	// For create/update events, no action needed
	// Don't trigger reconciliation - ServiceMonitor changes don't affect optimization logic
	return nil
}
