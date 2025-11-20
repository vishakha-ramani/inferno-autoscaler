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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	llmdVariantAutoscalingV1alpha1 "github.com/llm-d-incubation/workload-variant-autoscaler/api/v1alpha1"
	actuator "github.com/llm-d-incubation/workload-variant-autoscaler/internal/actuator"
	collector "github.com/llm-d-incubation/workload-variant-autoscaler/internal/collector"
	interfaces "github.com/llm-d-incubation/workload-variant-autoscaler/internal/interfaces"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/metrics"
	analyzer "github.com/llm-d-incubation/workload-variant-autoscaler/internal/modelanalyzer"
	variantAutoscalingOptimizer "github.com/llm-d-incubation/workload-variant-autoscaler/internal/optimizer"
	tuner "github.com/llm-d-incubation/workload-variant-autoscaler/internal/tuner"
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

	experimentalProactiveModel := os.Getenv("WVA_EXPERIMENTAL_PROACTIVE_MODEL")

	if strings.EqualFold(experimentalProactiveModel, "true") {
		logger.Log.Info("experimental proactive model is enabled!")
	}

	// TODO: decide on whether to keep accelerator properties (device name, cost) in same configMap, provided by administrator
	acceleratorCm, err := r.readAcceleratorConfig(ctx, "accelerator-unit-costs", configMapNamespace)
	if err != nil {
		logger.Log.Error(err, "unable to read accelerator configMap, skipping optimizing")
		return ctrl.Result{}, err
	}

	serviceClassCm, err := r.readServiceClassConfig(ctx, "service-classes-config", configMapNamespace)
	if err != nil {
		logger.Log.Error(err, "unable to read serviceclass configMap, skipping optimizing")
		return ctrl.Result{}, err
	}

	var variantAutoscalingList llmdVariantAutoscalingV1alpha1.VariantAutoscalingList
	if err := r.List(ctx, &variantAutoscalingList); err != nil {
		logger.Log.Error(err, "unable to list variantAutoscaling resources")
		return ctrl.Result{}, err
	}

	activeVAs := filterActiveVariantAutoscalings(variantAutoscalingList.Items)

	if len(activeVAs) == 0 {
		logger.Log.Info("No active VariantAutoscalings found, skipping optimization")
		return ctrl.Result{}, nil
	}

	switch experimentalProactiveModel {
	case "true":
		logger.Log.Info("Experimental proactive model is enabled!")
		if ctrlResult, err := r.runExperimentalProactiveModel(ctx, activeVAs, acceleratorCm, serviceClassCm, requeueDuration); err != nil {
			logger.Log.Error(err, "Experimental optimization failed")
			return ctrlResult, err
		}

	default:
		// Add saturation based reactive scaling
		logger.Log.Debug("Running in saturation based scaling")
	}

	return ctrl.Result{RequeueAfter: requeueDuration}, nil
}

func (r *VariantAutoscalingReconciler) runExperimentalProactiveModel(
	ctx context.Context,
	activeVAs []llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
	acceleratorCm map[string]map[string]string, serviceClassCm map[string]string,
	requeueDuration time.Duration,
) (ctrl.Result, error) {

	// WVA operates in unlimited mode - no cluster inventory collection needed
	systemData := utils.CreateSystemData(acceleratorCm, serviceClassCm)

	updateList, vaMap, allAnalyzerResponses, err := r.prepareVariantAutoscalings(ctx, activeVAs, acceleratorCm, serviceClassCm, systemData)
	if err != nil {
		logger.Log.Error(err, "failed to prepare variant autoscalings")
		return ctrl.Result{}, err
	}

	// Check if model tuner is enabled globally
	tunerEnabled, err := r.isModelTunerEnabled(ctx)
	if err != nil {
		logger.Log.Error(err, "Failed to read model tuner configuration, defaulting to disabled")
		tunerEnabled = false
	}

	if tunerEnabled {
		logger.Log.Debug("Experimental model tuner is enabled globally (EXPERIMENTAL_MODEL_TUNER_ENABLED=true) tuning model performance parameters for active VAs")

		// Check if auto-guess initial state is enabled globally
		autoGuessInitStateEnabled, err := r.isAutoGuessInitialStateEnabled(ctx)
		if err != nil {
			logger.Log.Debugf("Failed to read auto-guess configuration, defaulting to false: %v", err)
			autoGuessInitStateEnabled = false
		}
		// Tune queueing model parameters for all servers using the system data and all active VAs
		if err := tuner.TuneModelPerfParams(updateList.Items, systemData, autoGuessInitStateEnabled); err != nil {
			logger.Log.Warn(err, "failed to tune system data")
		}
	} else {
		logger.Log.Info("Model tuner is disabled globally (EXPERIMENTAL_MODEL_TUNER_ENABLED=false), using spec parameters for all VAs")
		// Populate TunerPerfData with spec parameters for all VAs
		for i := range updateList.Items {
			va := &updateList.Items[i]
			if err := tuner.SetFallbackTunedParamsInVAStatus(va); err != nil {
				logger.Log.Warnf("Failed to set fallback tuned parameters for variant %s/%s: %v", va.Name, va.Namespace, err)
			}
		}
	}

	// analyze
	system := inferno.NewSystem()
	optimizerSpec := system.SetFromSpec(&systemData.Spec)
	optimizer := infernoSolver.NewOptimizerFromSpec(optimizerSpec)
	manager := infernoManager.NewManager(system, optimizer)

	modelAnalyzer := analyzer.NewModelAnalyzer(system)
	for _, s := range system.Servers() {
		modelAnalyzeResponse := modelAnalyzer.AnalyzeModel(ctx, *vaMap[s.Name()])
		if len(modelAnalyzeResponse.Allocations) == 0 {
			logger.Log.Info("No potential allocations found for server - ", "serverName: ", s.Name())
			continue
		}
		allAnalyzerResponses[s.Name()] = modelAnalyzeResponse
	}
	logger.Log.Debug("System data prepared for optimization: - ", utils.MarshalStructToJsonString(systemData.Spec.Capacity))
	logger.Log.Debug("System data prepared for optimization: - ", utils.MarshalStructToJsonString(systemData.Spec.Accelerators))
	logger.Log.Debug("System data prepared for optimization: - ", utils.MarshalStructToJsonString(systemData.Spec.ServiceClasses))
	logger.Log.Debug("System data prepared for optimization: - ", utils.MarshalStructToJsonString(systemData.Spec.Models))
	logger.Log.Debug("System data prepared for optimization: - ", utils.MarshalStructToJsonString(systemData.Spec.Optimizer))
	logger.Log.Debug("System data prepared for optimization: - ", utils.MarshalStructToJsonString(systemData.Spec.Servers))

	engine := variantAutoscalingOptimizer.NewVariantAutoscalingsEngine(manager, system)

	optimizedAllocation, err := engine.Optimize(ctx, *updateList, allAnalyzerResponses)
	if err != nil {
		logger.Log.Error(err, "unable to perform model optimization, skipping this iteration")

		// Update OptimizationReady condition to False for all VAs in the update list
		for i := range updateList.Items {
			va := &updateList.Items[i]
			llmdVariantAutoscalingV1alpha1.SetCondition(va,
				llmdVariantAutoscalingV1alpha1.TypeOptimizationReady,
				metav1.ConditionFalse,
				llmdVariantAutoscalingV1alpha1.ReasonOptimizationFailed,
				fmt.Sprintf("Optimization failed: %v", err))

			if statusErr := r.Status().Update(ctx, va); statusErr != nil {
				logger.Log.Error(statusErr, "failed to update status condition after optimization failure",
					"variantAutoscaling", va.Name)
			}
		}

		return ctrl.Result{RequeueAfter: requeueDuration}, nil
	}

	logger.Log.Debug("Optimization completed successfully, emitting optimization metrics")
	logger.Log.Debug("Optimized allocation map - ", "numKeys: ", len(optimizedAllocation), ", updateList_count: ", len(updateList.Items))
	for key, value := range optimizedAllocation {
		logger.Log.Debug("Optimized allocation entry - ", "key: ", key, ", value: ", value)
	}

	if err := r.applyOptimizedAllocations(ctx, updateList, optimizedAllocation); err != nil {
		// If we fail to apply optimized allocations, we log the error
		// In next reconcile, the controller will retry.
		logger.Log.Error(err, "failed to apply optimized allocations")
		return ctrl.Result{RequeueAfter: requeueDuration}, nil
	}

	return ctrl.Result{}, nil
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

// applyOptimizedAllocations applies the optimized allocation to all VariantAutoscaling resources.
func (r *VariantAutoscalingReconciler) applyOptimizedAllocations(
	ctx context.Context,
	updateList *llmdVariantAutoscalingV1alpha1.VariantAutoscalingList,
	optimizedAllocation map[string]llmdVariantAutoscalingV1alpha1.OptimizedAlloc,
) error {
	logger.Log.Debug("Optimization metrics emitted, starting to process variants - ", "variant_count: ", len(updateList.Items))

	for i := range updateList.Items {
		va := &updateList.Items[i]
		_, ok := optimizedAllocation[va.Name]
		logger.Log.Debug("Processing variant - ", "index: ", i, ", variantAutoscaling-name: ", va.Name, ", namespace: ", va.Namespace, ", has_optimized_alloc: ", ok)
		if !ok {
			logger.Log.Debug("No optimized allocation found for variant - ", "variantAutoscaling-name: ", va.Name)
			continue
		}
		// Fetch the latest version from API server
		var updateVa llmdVariantAutoscalingV1alpha1.VariantAutoscaling
		if err := utils.GetVariantAutoscalingWithBackoff(ctx, r.Client, va.Name, va.Namespace, &updateVa); err != nil {
			logger.Log.Error(err, "failed to get latest VariantAutoscaling from API server: ", "variantAutoscaling-name: ", va.Name)
			continue
		}

		// Note: ownerReference is now set earlier in prepareVariantAutoscalings
		// This ensures it's set even if metrics aren't available yet

		updateVa.Status.CurrentAlloc = va.Status.CurrentAlloc
		updateVa.Status.DesiredOptimizedAlloc = optimizedAllocation[va.Name]
		updateVa.Status.Actuation.Applied = false // No longer directly applying changes

		// Copy existing conditions from updateList (includes MetricsAvailable condition set during preparation)
		// This ensures we don't lose the MetricsAvailable condition when fetching fresh copy from API
		// Always copy, even if empty, to preserve conditions set during prepareVariantAutoscalings
		updateVa.Status.Conditions = va.Status.Conditions

		// Copy TunerPerfData from updateList (which was updated by tuner)
		// The tuner handles setting initial params when ActivateModelTuner is false or tuned params when ActivateModelTuner is true
		updateVa.Status.TunerPerfData = va.Status.TunerPerfData

		// Set OptimizationReady condition to True on successful optimization
		llmdVariantAutoscalingV1alpha1.SetCondition(&updateVa,
			llmdVariantAutoscalingV1alpha1.TypeOptimizationReady,
			metav1.ConditionTrue,
			llmdVariantAutoscalingV1alpha1.ReasonOptimizationSucceeded,
			fmt.Sprintf("Optimization completed: %d replicas on %s",
				updateVa.Status.DesiredOptimizedAlloc.NumReplicas,
				updateVa.Status.DesiredOptimizedAlloc.Accelerator))

		act := actuator.NewActuator(r.Client)

		// Emit optimization signals for external autoscalers
		if err := act.EmitMetrics(ctx, &updateVa); err != nil {
			logger.Log.Error(err, "failed to emit optimization signals for external autoscalers", "variant", updateVa.Name)
		} else {
			logger.Log.Info(fmt.Sprintf("Successfully emitted optimization signals for external autoscalers - variant: %s", updateVa.Name))
			updateVa.Status.Actuation.Applied = true // Signals emitted successfully
		}

		if err := utils.UpdateStatusWithBackoff(ctx, r.Client, &updateVa, utils.StandardBackoff, "VariantAutoscaling"); err != nil {
			logger.Log.Error(err, "failed to patch status for variantAutoscaling after retries", "variantAutoscaling-name", updateVa.Name)
			continue
		}
	}

	logger.Log.Debug("Completed variant processing loop")

	// Log summary of reconciliation
	if len(updateList.Items) > 0 {
		logger.Log.Info("Reconciliation completed - ",
			"variants_processed: ", len(updateList.Items),
			", optimization_successful: ", true)
	}

	return nil
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&llmdVariantAutoscalingV1alpha1.VariantAutoscaling{}).
		// Watch the specific ConfigMap to trigger reconcile for all VAs
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				if obj.GetName() != configMapName || obj.GetNamespace() != configMapNamespace {
					return nil
				}

				// List all VariantAutoscaling resources and enqueue each one
				var vaList llmdVariantAutoscalingV1alpha1.VariantAutoscalingList
				if err := mgr.GetClient().List(ctx, &vaList); err != nil {
					logger.Log.Error(err, "Failed to list VariantAutoscaling resources for ConfigMap watch")
					return nil
				}

				var requests []reconcile.Request
				for _, va := range vaList.Items {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      va.Name,
							Namespace: va.Namespace,
						},
					})
				}
				logger.Log.Debugf("ConfigMap watch enqueueing requests: count=%d", len(requests))
				return requests
			}),
			// Only reconcile when the target ConfigMap's Data actually changes
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetName() == configMapName && e.Object.GetNamespace() == configMapNamespace
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					// Only reconcile if it's our ConfigMap and the Data changed
					if e.ObjectNew.GetName() != configMapName || e.ObjectNew.GetNamespace() != configMapNamespace {
						return false
					}
					oldCM, okOld := e.ObjectOld.(*corev1.ConfigMap)
					newCM, okNew := e.ObjectNew.(*corev1.ConfigMap)
					if !okOld || !okNew {
						return false
					}
					// Compare Data maps - reconcile only if Data changed
					if len(oldCM.Data) != len(newCM.Data) {
						return true
					}
					for k, v := range newCM.Data {
						if oldCM.Data[k] != v {
							return true
						}
					}
					return false
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return false
				},
			}),
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

// isModelTunerEnabled checks if the experimental model tuner feature is enabled via ConfigMap
func (r *VariantAutoscalingReconciler) isModelTunerEnabled(ctx context.Context) (bool, error) {
	cm := corev1.ConfigMap{}
	err := utils.GetConfigMapWithBackoff(ctx, r.Client, configMapName, configMapNamespace, &cm)
	if err != nil {
		return false, fmt.Errorf("failed to get optimization configmap: %w", err)
	}

	enabled := cm.Data["EXPERIMENTAL_MODEL_TUNER_ENABLED"]
	return strings.EqualFold(enabled, "true"), nil
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

// isAutoGuessInitialStateEnabled checks if auto-guess initial state is enabled via ConfigMap
func (r *VariantAutoscalingReconciler) isAutoGuessInitialStateEnabled(ctx context.Context) (bool, error) {
	cm := corev1.ConfigMap{}
	err := utils.GetConfigMapWithBackoff(ctx, r.Client, configMapName, configMapNamespace, &cm)
	if err != nil {
		return false, fmt.Errorf("failed to get optimization configmap: %w", err)
	}

	enabled := cm.Data["EXPERIMENTAL_AUTO_GUESS_INITIAL_STATE"]
	return strings.EqualFold(enabled, "true"), nil
}
