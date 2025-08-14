package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createAcceleratorUnitCostConfigMap creates the accelerator unitcost ConfigMap
func CreateAcceleratorUnitCostConfigMap(controllerNamespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "accelerator-unit-costs",
			Namespace: controllerNamespace,
		},
		Data: map[string]string{
			"A100": `{
"device": "NVIDIA-A100-PCIE-80GB",
"cost": "40.00"
}`,
			"MI300X": `{
"device": "AMD-MI300X-192GB",
"cost": "65.00"
}`,
			"G2": `{
"device": "Intel-Gaudi-2-96GB",
"cost": "23.00"
}`,
		},
	}
}

// createServiceClassConfigMap creates the serviceclass ConfigMap
func CreateServiceClassConfigMap(controllerNamespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-classes-config",
			Namespace: controllerNamespace,
		},
		Data: map[string]string{
			"premium.yaml": `name: Premium
priority: 1
data:
  - model: default/default
    slo-tpot: 24
    slo-ttft: 500
  - model: meta/llama0-70b
    slo-tpot: 80
    slo-ttft: 500`,
			"freemium.yaml": `name: Freemium
priority: 10
data:
  - model: ibm/granite-13b
    slo-tpot: 200
    slo-ttft: 2000
  - model: meta/llama0-7b
    slo-tpot: 150
    slo-ttft: 1500`,
		},
	}
}

func CreateVariantAutoscalingConfigMap(controllerNamespace string) *corev1.ConfigMap {

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inferno-autoscaler-variantautoscaling-config",
			Namespace: controllerNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "inferno-autoscaler",
			},
		},
		Data: map[string]string{
			"PROMETHEUS_BASE_URL": "http://kube-prometheus-stack-prometheus.inferno-autoscaler-monitoring.svc.cluster.local:9090",
			"GLOBAL_OPT_INTERVAL": "60s",
			"GLOBAL_OPT_TRIGGER":  "false",
		},
	}
}
