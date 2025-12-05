/*
Copyright 2025 The llm-d Authors

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

package utils

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wvav1alpha1 "github.com/llm-d-incubation/workload-variant-autoscaler/api/v1alpha1"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
)

// VariantFilter is a function that determines if a VA should be included.
type VariantFilter func(deploy *appsv1.Deployment) bool

// ActiveVariantAutoscalingByModel retrieves all VariantAutoscaling resources that are ready for optimization
// and have at least one target replica.
// Returns the shallow-copied VAs (not safe for mutation) grouped by ModelID.
func ActiveVariantAutoscalingByModel(ctx context.Context, client client.Client) (map[string][]wvav1alpha1.VariantAutoscaling, error) {
	vas, err := ActiveVariantAutoscaling(ctx, client)
	if err != nil {
		return nil, err
	}
	return GroupVariantAutoscalingByModel(vas), nil
}

// InactiveVariantAutoscalingByModel retrieves all VariantAutoscaling resources that are ready for optimization
// and have no target replicas.
// Returns the shallow-copied VAs (not safe for mutation) grouped by ModelID.
func InactiveVariantAutoscalingByModel(ctx context.Context, client client.Client) (map[string][]wvav1alpha1.VariantAutoscaling, error) {
	vas, err := InactiveVariantAutoscaling(ctx, client)
	if err != nil {
		return nil, err
	}
	return GroupVariantAutoscalingByModel(vas), nil
}

// GroupVariantAutoscalingByModel groups VariantAutoscalings by model ID
func GroupVariantAutoscalingByModel(
	vas []wvav1alpha1.VariantAutoscaling,
) map[string][]wvav1alpha1.VariantAutoscaling {
	groups := make(map[string][]wvav1alpha1.VariantAutoscaling)
	for _, va := range vas {
		modelID := va.Spec.ModelID
		groups[modelID] = append(groups[modelID], va)
	}
	return groups
}

// ActiveVariantAutoscalings retrieves all VariantAutoscaling resources that are ready for optimization
// and have at least one target replica.
// Returns a slice of deep-copied VariantAutoscaling objects.
func ActiveVariantAutoscaling(ctx context.Context, client client.Client) ([]wvav1alpha1.VariantAutoscaling, error) {
	return filterVariantsByDeployment(ctx, client, isActive, "active")
}

// InactiveVariantAutoscaling retrieves all VariantAutoscaling resources that are ready for optimization
// and have no target replicas.
// Returns a slice of deep-copied VariantAutoscaling objects.
func InactiveVariantAutoscaling(ctx context.Context, client client.Client) ([]wvav1alpha1.VariantAutoscaling, error) {
	return filterVariantsByDeployment(ctx, client, isInactive, "inactive")
}

// filterVariantsByDeployment is a generic function to filter VAs based on deployment state.
func filterVariantsByDeployment(ctx context.Context, client client.Client, filter VariantFilter, filterName string) ([]wvav1alpha1.VariantAutoscaling, error) {
	readyVAs, err := readyVariantAutoscalings(ctx, client)
	if err != nil {
		return nil, err
	}

	filteredVAs := make([]wvav1alpha1.VariantAutoscaling, 0, len(readyVAs))

	for _, va := range readyVAs {

		// Check if the context is done
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// TODO: Generalize to other scale target kinds in future
		var deploy appsv1.Deployment
		if err := GetDeploymentWithBackoff(ctx, client, va.Name, va.Namespace, &deploy); err != nil {
			logger.Log.Errorw("Failed to get deployment", "namespace", va.Namespace, "name", va.Name, "error", err)
			continue
		}

		// Skip deleted deployments
		if !deploy.DeletionTimestamp.IsZero() {
			logger.Log.Debugw("Skipping deleted deployment", "namespace", va.Namespace, "name", va.Name)
			continue
		}

		// Apply the filter function
		if filter(&deploy) {
			filteredVAs = append(filteredVAs, va)
		}
	}
	logger.Log.Debugw("Found filtered VariantAutoscaling resources",
		"filterType", filterName,
		"count", len(filteredVAs))

	return filteredVAs, nil
}

// readyVariantAutoscalings retrieves all VariantAutoscaling resources that are ready for optimization
// (condition TargetResolved is true).
func readyVariantAutoscalings(ctx context.Context, client client.Client) ([]wvav1alpha1.VariantAutoscaling, error) {
	// List all VariantAutoscaling resources
	var variantAutoscalingList wvav1alpha1.VariantAutoscalingList
	if err := client.List(ctx, &variantAutoscalingList); err != nil {
		logger.Log.Errorw("unable to list variantAutoscaling resources", "error", err)
		return nil, err
	}

	// Filter VAs that are ready for optimization
	readyVAs := make([]wvav1alpha1.VariantAutoscaling, 0, len(variantAutoscalingList.Items))
	for _, va := range variantAutoscalingList.Items {
		// Skip deleted VAs
		if !va.DeletionTimestamp.IsZero() {
			continue
		}

		if wvav1alpha1.IsConditionTrue(&va, wvav1alpha1.TypeTargetResolved) { // TODO: add a Ready condition
			readyVAs = append(readyVAs, va) // Shallow copy
		}
	}

	logger.Log.Debugw("Found VariantAutoscaling resources ready for optimization", "count", len(readyVAs))
	return readyVAs, nil
}

// isActive explicitly requires that replicas > 0
func isActive(deploy *appsv1.Deployment) bool {
	return GetDesiredReplicas(deploy) > 0
}

// isInactive explicitly requires that replicas == 0
func isInactive(deploy *appsv1.Deployment) bool {
	return GetDesiredReplicas(deploy) == 0
}

// Helper function makes behavior explicit
func GetDesiredReplicas(deploy *appsv1.Deployment) int32 {
	if deploy == nil || deploy.Spec.Replicas == nil {
		return 1 // Kubernetes default
	}
	return *deploy.Spec.Replicas
}
