#!/usr/bin/env bash

set -eou pipefail

KIND=${KIND:-kind}
KUBECTL=${KUBECTL:-kubectl}
KIND_NAME=${KIND_NAME:-"kind-inferno-gpu-cluster"}
KIND_CONTEXT=kind-${KIND_NAME}
MONITORING_NAMESPACE=${MONITORING_NAMESPACE:-"inferno-autoscaler-monitoring"}
KIND_NODE_NAME=${KIND_NODE_NAME:-"kind-inferno-gpu-cluster-control-plane"}
WEBHOOK_TIMEOUT=${WEBHOOK_TIMEOUT:-2m}

_kubectl() {
        ${KUBECTL} --context ${KIND_CONTEXT} $@
}

_kind() {
	${KIND} $@
}

# Local development will need emulated vllm server, prometheus installed in KinD cluster
_kubectl create ns ${MONITORING_NAMESPACE}

# Install Prometheus using Helm
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack -n ${MONITORING_NAMESPACE}

# Wait for prometheus installation to complete
_kubectl apply -f hack/vllme/deploy/prometheus-operator/prometheus-deploy-all-in-one.yaml
_kubectl wait --for=condition=ready pods --all -n ${MONITORING_NAMESPACE} --timeout=${WEBHOOK_TIMEOUT}

# Create vllm emulated deployment
_kubectl apply -f hack/vllme/deploy/vllme-setup/vllme-deployment-with-service-and-servicemon.yaml

# Create variant autoscaling object for controller
_kubectl apply -f hack/vllme/deploy/vllme-setup/vllme-variantautoscaling.yaml