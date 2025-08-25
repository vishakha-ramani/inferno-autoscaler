#!/usr/bin/env bash

set -euo pipefail

INFRA_REPO_DIR="${HOME}/.cache/llm-d-infra"
PROJ_ROOT_DIR="$(pwd)"
ARCH=$(uname -m)

INTEGRATION_DIR="${PROJ_ROOT_DIR}/hack/vllme/deploy/integration_llm-d"
DEFAULT_VALUES_FILE="${INTEGRATION_DIR}/arm64-gaie-sim-values.yaml"
LLMD_NAMESPACE="llm-d-sim"
INFRA_RELEASE_NAME="infra-sim"
EPP_RELEASE_NAME="gaie-sim"

# Default values
INFERNO_IMAGE="quay.io/infernoautoscaler/inferno-controller:0.0.1-multi-arch"
CLUSTER_NODES="3"
CLUSTER_GPUS="4"
CLUSTER_TYPE="mix"

print_help() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Options:
  -i, --inferno-image IMAGE   Container image to use for the Inferno-autoscaler (default: quay.io/infernoautoscaler/inferno-controller:0.0.1-multi-arch)
  -n, --nodes NUM              Number of nodes for KIND cluster (default: 4)
  -g, --gpus NUM               Number of GPUs per node (default: 5)  
  -t, --type TYPE              GPU type: nvidia, amd, intel, or mix (default: nvidia)
  -h, --help                   Show this help and exit

Examples:
  # Deploy with default values
  $(basename "$0")
  
  # Deploy with custom inferno image and cluster configuration
  $(basename "$0") -i my-registry/inferno:latest -n 3 -g 6 -t mix
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -i|--inferno-image)     INFERNO_IMAGE="$2"; shift 2 ;;
      -n|--nodes)             CLUSTER_NODES="$2"; shift 2 ;;
      -g|--gpus)              CLUSTER_GPUS="$2"; shift 2 ;;
      -t|--type)              CLUSTER_TYPE="$2"; shift 2 ;;
      -h|--help)              print_help; exit 0 ;;
      *)                      echo "Unknown option: $1"; print_help; exit 1 ;;
    esac
  done
}

get-llm-d-latest() {
  if [ -d "$INFRA_REPO_DIR" ]; then
    echo ">>> Removing any existing llm-d infrastructure repo at $INFRA_REPO_DIR"
    rm -rf "$INFRA_REPO_DIR"
  fi

  local owner="llm-d-incubation" 
  local project="llm-d-infra"
  local release="v1.1.1"

  echo ">>> Cloning the latest release of $project from $owner: $release"
  echo ">>> Cloning into $INFRA_REPO_DIR"
  git clone -b $release -- https://github.com/$owner/$project.git $INFRA_REPO_DIR
}

function apply_fix_for_vllme_comp() {
  local INFERENCE_MODEL_FILE="${INTEGRATION_DIR}/vllme-inferencemodel.yaml"

  echo ">>> Applying fixes to integrate vLLM emulator servers..."
  echo ">>> Applying InferenceModel CR ..."
  kubectl apply -f "$INFERENCE_MODEL_FILE"

  echo ">>> Patching InferencePool to target vLLM emulator port ..."
  kubectl patch inferencepool "$EPP_RELEASE_NAME" -n "$LLMD_NAMESPACE" --type='merge' -p '{"spec":{"targetPortNumber":80}}'

  echo ">>> Deleting other SIM deployments if they exist..."
  kubectl delete deployments.apps ms-sim-llm-d-modelservice-decode ms-sim-llm-d-modelservice-prefill --ignore-not-found -n "$LLMD_NAMESPACE"
}

function deploy_inferno() {
    echo ">>> Deploying Inferno Autoscaler using image: $INFERNO_IMAGE"
    make deploy-inferno-emulated-on-kind IMG="${INFERNO_IMAGE}" KIND_ARGS="-n ${CLUSTER_NODES} -g ${CLUSTER_GPUS} -t ${CLUSTER_TYPE}"
    echo "Inferno Autoscaler deployed successfully."
}

function deploy-llm-d-infra() {
  local GATEWAY="kgateway"

  echo ">>> Running the dependency script"
  bash install-deps.sh

  echo ">>> Running the llm-d installer script"
  export HF_TOKEN="dummy-token"
  ./llmd-infra-installer.sh --namespace "$LLMD_NAMESPACE" -r "$INFRA_RELEASE_NAME" --gateway "$GATEWAY" --disable-metrics-collection

  echo ">>> Use the helmfile to apply the modelservice and GIE charts on top of it."
  cd "$INFRA_REPO_DIR/quickstart/examples/sim"
  helmfile --selector managedBy=helmfile apply -f helmfile.yaml --skip-diff-on-install

  sleep 30
  echo ">>> Gateway and EPP Installed."
}

echo ">>> Getting latest llm-d infrastructure release..."
get-llm-d-latest

main() {
parse_args "$@"

echo ">>> Using configuration:"
echo "    Inferno Image: $INFERNO_IMAGE"
echo "    Cluster Nodes: $CLUSTER_NODES"
echo "    Cluster GPUs: $CLUSTER_GPUS"
echo "    Cluster Type: $CLUSTER_TYPE"

if [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
  echo "ARM64 platform detected, using custom arm64 values.yaml"
  cp "$DEFAULT_VALUES_FILE" $INFRA_REPO_DIR/quickstart/examples/sim/gaie-sim/values.yaml

else
  echo "Non-ARM64 platform, using default manifest."

fi

cd "$PROJ_ROOT_DIR"
deploy_inferno

cd "$INFRA_REPO_DIR/quickstart"
deploy-llm-d-infra

apply_fix_for_vllme_comp

echo "llm-d infrastructure installation complete."

echo ">>> To target the deployed vLLM-emulator servers, deploy the VariantAutoscaling object:"
echo "kubectl apply -f hack/vllme/deploy/vllme-setup/vllme-variantautoscaling.yaml"

echo ">>> To curl the Gateway, port-forward it first using:"
echo "kubectl port-forward -n $LLMD_NAMESPACE svc/$INFRA_RELEASE_NAME-inference-gateway 8000:80"

echo ">>> Then launch the load generator:"
echo "cd $PROJ_ROOT_DIR/hack/vllme/vllm_emulator"
echo "pip install -r requirements.txt"
echo "python loadgen.py --model default/default  --rate '[[1200, 40]]' --url http://localhost:8000/v1 --content 50"
}

main "$@"