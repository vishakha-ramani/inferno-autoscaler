package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetCondition sets the specified condition on the VariantAutoscaling status
func SetCondition(va *VariantAutoscaling, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: va.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&va.Status.Conditions, condition)
}

// GetCondition returns the condition with the specified type
func GetCondition(va *VariantAutoscaling, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(va.Status.Conditions, conditionType)
}

// IsConditionTrue returns true if the condition with the specified type has status True
func IsConditionTrue(va *VariantAutoscaling, conditionType string) bool {
	return meta.IsStatusConditionTrue(va.Status.Conditions, conditionType)
}

// IsConditionFalse returns true if the condition with the specified type has status False
func IsConditionFalse(va *VariantAutoscaling, conditionType string) bool {
	return meta.IsStatusConditionFalse(va.Status.Conditions, conditionType)
}
