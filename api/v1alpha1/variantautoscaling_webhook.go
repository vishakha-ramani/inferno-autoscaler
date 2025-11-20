package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// log is for logging in this package.
var variantautoscalinglog = logf.Log.WithName("variantautoscaling-resource")

// SetupWebhookWithManager registers the webhook with the manager.
func (r *VariantAutoscaling) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-llm-d-ai-v1alpha1-variantautoscaling,mutating=false,failurePolicy=fail,sideEffects=None,groups=llm-d.ai,resources=variantautoscalings,verbs=create;update,versions=v1alpha1,name=vvariantautoscaling.kb.io,admissionReviewVersions=v1

// ValidateCreate implements webhook validation for create operations
func (r *VariantAutoscaling) ValidateCreate() error {
	variantautoscalinglog.Info("validate create", "name", r.Name)

	return r.validateVariantAutoscaling()
}

// ValidateUpdate implements webhook validation for update operations
func (r *VariantAutoscaling) ValidateUpdate(old runtime.Object) error {
	variantautoscalinglog.Info("validate update", "name", r.Name)

	return r.validateVariantAutoscaling()
}

// ValidateDelete implements webhook validation for delete operations
func (r *VariantAutoscaling) ValidateDelete() error {
	variantautoscalinglog.Info("validate delete", "name", r.Name)

	// No validation needed for delete
	return nil
}

// validateVariantAutoscaling performs validation checks on the VariantAutoscaling resource
func (r *VariantAutoscaling) validateVariantAutoscaling() error {
	var allErrs []error

	// Validate ModelID is not empty
	if r.Spec.ModelID == "" {
		allErrs = append(allErrs, fmt.Errorf("spec.modelID is required and cannot be empty"))
	}

	// Validate ModelProfile has at least one accelerator
	if len(r.Spec.ModelProfile.Accelerators) == 0 {
		allErrs = append(allErrs, fmt.Errorf("spec.modelProfile.accelerators must contain at least one accelerator"))
	}

	// Validate each accelerator profile
	for i, acc := range r.Spec.ModelProfile.Accelerators {
		if acc.Acc == "" {
			allErrs = append(allErrs, fmt.Errorf("spec.modelProfile.accelerators[%d].acc cannot be empty", i))
		}
		if acc.AccCount < 1 {
			allErrs = append(allErrs, fmt.Errorf("spec.modelProfile.accelerators[%d].accCount must be at least 1", i))
		}
		if acc.MaxBatchSize < 1 {
			allErrs = append(allErrs, fmt.Errorf("spec.modelProfile.accelerators[%d].maxBatchSize must be at least 1", i))
		}

		// Validate performance parameters if provided
		if acc.PerfParms.DecodeParms != nil {
			if _, hasAlpha := acc.PerfParms.DecodeParms["alpha"]; !hasAlpha {
				allErrs = append(allErrs, fmt.Errorf("spec.modelProfile.accelerators[%d].perfParms.decodeParms must contain 'alpha' key", i))
			}
			if _, hasBeta := acc.PerfParms.DecodeParms["beta"]; !hasBeta {
				allErrs = append(allErrs, fmt.Errorf("spec.modelProfile.accelerators[%d].perfParms.decodeParms must contain 'beta' key", i))
			}
		}

		if acc.PerfParms.PrefillParms != nil {
			if _, hasGamma := acc.PerfParms.PrefillParms["gamma"]; !hasGamma {
				allErrs = append(allErrs, fmt.Errorf("spec.modelProfile.accelerators[%d].perfParms.prefillParms must contain 'gamma' key", i))
			}
			if _, hasDelta := acc.PerfParms.PrefillParms["delta"]; !hasDelta {
				allErrs = append(allErrs, fmt.Errorf("spec.modelProfile.accelerators[%d].perfParms.prefillParms must contain 'delta' key", i))
			}
		}
	}

	// Validate SLOClassRef if provided
	if r.Spec.SLOClassRef.Name != "" && r.Spec.SLOClassRef.Key == "" {
		allErrs = append(allErrs, fmt.Errorf("spec.sloClassRef.key is required when spec.sloClassRef.name is specified"))
	}
	if r.Spec.SLOClassRef.Key != "" && r.Spec.SLOClassRef.Name == "" {
		allErrs = append(allErrs, fmt.Errorf("spec.sloClassRef.name is required when spec.sloClassRef.key is specified"))
	}

	// Combine all errors
	if len(allErrs) > 0 {
		errMsg := "validation failed:"
		for _, err := range allErrs {
			errMsg += fmt.Sprintf("\n  - %s", err.Error())
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}
