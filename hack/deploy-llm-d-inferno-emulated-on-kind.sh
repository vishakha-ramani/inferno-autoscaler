#!/usr/bin/env bash

set -euo pipefail

INFRA_REPO_DIR="${HOME}/.cache/llm-d-infra"
PROJ_ROOT_DIR="$(pwd)"
ARCH=$(uname -m)

INTEGRATION_DIR="${PROJ_ROOT_DIR}/hack/vllme/deploy/integration_llm-d"
DEFAULT_VALUES_FILE="${INTEGRATION_DIR}/arm64-gaie-sim-values.yaml"
INFERENCE_MODEL_FILE="${INTEGRATION_DIR}/vllme-inferencemodel.yaml"
LLMD_NAMESPACE="llm-d-sim"
INFRA_RELEASE_NAME="infra-sim"
EPP_RELEASE_NAME="gaie-sim"
GATEWAY="kgateway"

INFERNO_DEFAULT_IMAGE="quay.io/infernoautoscaler/inferno-controller:latest"

function apply_fix_for_vllme_comp() {
  echo ">>> Applying fixes to integrate vllm-e servers..."
  echo ">>> Applying InferenceModel CR ..."
  kubectl apply -f "$INFERENCE_MODEL_FILE"
  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to apply InferenceModel CR."
    exit 1
  fi

  echo ">>> Patching InferencePool to target vllm-e port ..."
  kubectl patch inferencepool gaie-sim -n llm-d-sim --type='merge' -p '{"spec":{"targetPortNumber":80}}'
  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to patch InferencePool."
    exit 1
  fi

  echo ">>> Deleting other SIM deployments if they exist..."
  kubectl delete deployments.apps ms-sim-llm-d-modelservice-decode ms-sim-llm-d-modelservice-prefill --ignore-not-found -n "$LLMD_NAMESPACE"
}

function deploy_inferno() {
    echo ">>> Deploying Inferno Autoscaler..."
    make deploy-inferno-emulated-on-kind "$@"
    if [[ $? -ne 0 ]]; then
        echo "ERROR: Inferno Autoscaler deployment failed."
        exit 1
    fi
    echo ">>> Creating variant autoscaling object for controller..."
    kubectl apply -f hack/vllme/deploy/vllme-setup/vllme-variantautoscaling.yaml

    echo "Inferno Autoscaler deployed successfully."
}

if [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
  echo "ARM64 platform detected, using custom arm64 values.yaml"
  cp "$DEFAULT_VALUES_FILE" $INFRA_REPO_DIR/quickstart/examples/sim/gaie-sim/values.yaml

else
  echo "Non-ARM64 platform, using default manifest."

fi

cd "$PROJ_ROOT_DIR"
deploy_inferno

echo ">>> Cloning llm-d-infra repo and running the installer..."
if [[ ! -d "$INFRA_REPO_DIR" ]]; then
    echo ">>> Cloning llm-d-infra repo"
    git clone https://github.com/llm-d-incubation/llm-d-infra.git "$INFRA_REPO_DIR"
else
    echo ">>> Using existing repo clone, updating..."
    git -C "$INFRA_REPO_DIR" pull --ff-only
fi

cd "$INFRA_REPO_DIR/quickstart"
echo ">>> Running the dependency script"
bash install-deps.sh
if [[ $? -ne 0 ]]; then
    echo "ERROR: Dependency installation failed."
    exit 1
fi

echo ">>> Running the llm-d-infra installer script"
export HF_TOKEN="dummy-token"
./llmd-infra-installer.sh --namespace "$LLMD_NAMESPACE" -r "$INFRA_RELEASE_NAME" --gateway "$GATEWAY" --disable-metrics-collection
if [[ $? -ne 0 ]]; then
    echo "ERROR: llm-d-infra installer script failed."
    exit 1
fi

echo ">>> Use the helmfile to apply the modelservice and GIE charts on top of it."
cd "$INFRA_REPO_DIR/quickstart/examples/sim"
helmfile --selector managedBy=helmfile apply -f helmfile.yaml --skip-diff-on-install
if [[ $? -ne 0 ]]; then
    echo "ERROR: Helmfile apply failed."
    exit 1
fi

echo ">>> Waiting for the llm-d-infra sim EPP and Gateway to be ready..."
sleep 10
kubectl rollout status -n "$LLMD_NAMESPACE" deployment/"$INFRA_RELEASE_NAME"-inference-gateway --timeout=60s
if [[ $? -ne 0 ]]; then
    echo "ERROR: Gateway did not become ready in time."
    exit 1
fi

kubectl rollout status -n "$LLMD_NAMESPACE" deployment/"$EPP_RELEASE_NAME"-epp --timeout=60s
if [[ $? -ne 0 ]]; then
    echo "ERROR: EPP did not become ready in time."
    exit 1
fi
echo ">>> Gateway and EPP are ready."

apply_fix_for_vllme_comp

echo "llm-d-infra installation complete."

echo ">>> To curl the Gateway, port-forward it first using:"
echo "kubectl port-forward -n $LLMD_NAMESPACE svc/$INFRA_RELEASE_NAME-inference-gateway 8000:80"
echo ">>> Then launch the load generator:"
echo "cd $PROJ_ROOT_DIR/hack/vllme/vllm_emulator"
echo "python loadgen.py"
echo ">>> As 'server base URL', use: http://localhost:8000/v1 [option 3]"
echo ">>> As 'model name', insert: vllm"
