#!/usr/bin/env bash

set -euo pipefail

INFRA_REPO_DIR="${HOME}/.cache/llm-d-infra"
PROJ_ROOT_DIR="$(pwd)"

INTEGRATION_DIR="${PROJ_ROOT_DIR}/hack/vllme/deploy/integration_llm-d"
LLMD_NAMESPACE="llm-d-sim"
INFRA_RELEASE_NAME="infra-sim"
GATEWAY="kgateway"

INFERNO_DEFAULT_IMAGE="quay.io/infernoautoscaler/inferno-controller:latest"

function undeploy_inferno() {
    echo ">>> Undeploying Inferno Autoscaler..."
    make undeploy-inferno-on-kind
    if [[ $? -ne 0 ]]; then
        echo "ERROR: Inferno Autoscaler undeployment failed."
        exit 1
    fi
}

undeploy_inferno

cd "$INFRA_REPO_DIR/quickstart"

echo ">>> Running the llm-d-infra installer script to uninstall llm-d..."
./llmd-infra-installer.sh --namespace "$LLMD_NAMESPACE" -r "$INFRA_RELEASE_NAME" --gateway "$GATEWAY" -u
if [[ $? -ne 0 ]]; then
    echo "ERROR: llm-d-infra installer script failed to uninstall llm-d."
    exit 1
fi

cd "$PROJ_ROOT_DIR"

kubectl delete namespace "$GATEWAY"-system --ignore-not-found
echo ">>> Cleaning up llm-d-infra repo..."
rm -rf "$INFRA_REPO_DIR"

echo "llm-d-infra uninstallation complete."
