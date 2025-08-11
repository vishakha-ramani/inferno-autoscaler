#!/usr/bin/env bash

set -eou pipefail

KIND=${KIND:-kind}
KUBECTL=${KUBECTL:-kubectl}
KIND_NAME=${KIND_NAME:-"kind-inferno-gpu-cluster"}
KIND_CONTEXT=kind-${KIND_NAME}
NAMESPACE=${NAMESPACE:-"inferno-autoscaler-system"}
MONITORING_NAMESPACE=${MONITORING_NAMESPACE:-"inferno-autoscaler-monitoring"}
KIND_NODE_NAME=${KIND_NODE_NAME:-"kind-inferno-gpu-cluster-control-plane"}
WEBHOOK_TIMEOUT=${WEBHOOK_TIMEOUT:-2m}
PROMETHEUS_BASE_URL=${PROMETHEUS_BASE_URL:-"http://kube-prometheus-stack-prometheus.inferno-autoscaler-monitoring.svc.cluster.local:9090"}

_kubectl() {
        ${KUBECTL} --context ${KIND_CONTEXT} $@
}

_kind() {
	${KIND} $@
}

# Check if the Kind cluster exists, if not, create it
if ! _kind get kubeconfig --name "${KIND_NAME}" &>/dev/null; then
  echo "Kind cluster '${KIND_NAME}' does not exist. Creating..."
  hack/create-kind-gpu-cluster.sh "$@"
else
  echo "Kind cluster '${KIND_NAME}' is already running."
fi

# Load the Docker image into the Kind cluster
echo "Loading Docker image '${IMG}' into Kind cluster '${KIND_NAME}'..."
docker pull "${IMG}"
_kind load docker-image ${IMG} --name ${KIND_NAME}

echo "Creating namespace ${NAMESPACE}"
_kubectl create ns ${NAMESPACE} 2>/dev/null || true

echo "Installing inferno CRD"
make install
sleep 10

${KUBECTL} config set-context ${KIND_CONTEXT}

# Install the configmap service class
_kubectl apply -f deploy/configmap-serviceclass.yaml

# Install the configmap for the accelerator unit cost
_kubectl apply -f deploy/configmap-accelerator-unitcost.yaml

# deploy emulated vllme server (includes Prometheus)
hack/deploy-emulated-vllme-server.sh

echo "Deploying Inferno controller-manager"
make deploy-emulated
echo "Inferno controller-manager Installed"

# Configure Prometheus URL for the Kind cluster deployment
echo "Configuring Prometheus URL for Kind cluster deployment..."
echo "Using Prometheus URL: ${PROMETHEUS_BASE_URL}"
_kubectl patch configmap inferno-autoscaler-variantautoscaling-config -n ${NAMESPACE} --patch "{\"data\":{\"PROMETHEUS_BASE_URL\":\"${PROMETHEUS_BASE_URL}\"}}" --type=merge

echo "Restarting controller to pick up new configuration..."
_kubectl rollout restart deployment inferno-autoscaler-controller-manager -n ${NAMESPACE}
echo "Inferno Deployment complete"