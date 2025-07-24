#!/bin/bash

set -euo pipefail

DEFAULT_CLUSTER_NAME="a100-cluster"
DEFAULT_NODES=5
DEFAULT_GPUS_PER_NODE=2
DEFAULT_GPU_TYPE=nvidia
DEFAULT_GPU_MODEL=NVIDIA-A100-PCIE-40GB
DEFAULT_GPU_MEMORY=40960

cluster_name="a100-cluster"
control_plane_node="${cluster_name}-control-plane"

echo "[1/5] Creating Kind cluster: ${cluster_name}..."

nodes=$DEFAULT_NODES
gpu_type=$DEFAULT_GPU_TYPE
gpus_per_node=$DEFAULT_GPUS_PER_NODE
gpu_model=$DEFAULT_GPU_MODEL
gpu_memory=$DEFAULT_GPU_MEMORY

#
# Create the kind cluster with the specified number of nodes
#
cat <<EOF > kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
EOF

for ((i=1; i<nodes; i++)); do
  echo "- role: worker" >> kind-config.yaml
done

kind create cluster --name "${cluster_name}" --config kind-config.yaml

echo "[2/5] Waiting for node ${control_plane_node} to be ready..."
while [[ $(kubectl get nodes "${control_plane_node}" --no-headers 2>/dev/null | awk '{print $2}') != "Ready" ]]; do
  sleep 1
done

#
# Patch a node with fake gpus
#
patch_node_gpu() {
    local node_name="$1"
    local gpu_type="$2"
    local gpu_count="$3"
    local gpu_product="$4"
    local gpu_memory="$5"

    echo "[3/5] Patching node '${node_name}' with ${gpu_type} GPU annotation..."

    cat <<EOF | kubectl patch node "${node_name}" --type merge --patch "$(cat)"
metadata:
  labels:
    ${gpu_type}.com/gpu.count: "${gpu_count}"
    ${gpu_type}.com/gpu.product: "${gpu_product}"
    ${gpu_type}.com/gpu.memory: "${gpu_memory}"
EOF
}

for node_name in $(kubectl get nodes --no-headers -o custom-columns=":metadata.name")
do
    patch_node_gpu ${node_name} ${gpu_type} ${gpus_per_node} ${gpu_model} ${gpu_memory}
done

echo "[4/5] Starting kubectl proxy..."
kubectl proxy > /dev/null 2>&1 &
proxy_pid=$!
sleep 15  # Give proxy a moment to start

echo "Starting background proxy connection (pid=${proxy_pid})..."
curl 127.0.0.1:8001 > /dev/null 2>&1
if [[ ! $? -eq 0 ]]; then
    echo "Calling 'kubectl proxy' did not create a successful connection to the kubelet needed to patch the nodes. Exiting."
    exit 1
else
    echo "Connected to the kubelet for patching the nodes"
fi

# Patch nodes with gpu resource capacity
for node_name in $(kubectl get nodes --no-headers -o custom-columns=":metadata.name")
do
    echo "- Patching node (add): ${node_name}"
    resource_name="${gpu_type}.com~1gpu"
    resource_count="${gpus_per_node}"

    curl --header "Content-Type: application/json-patch+json" \
            --request PATCH \
            --data '[{"op":"add","path":"/status/capacity/'${resource_name}'","value":"'${resource_count}'"}]' \
            http://localhost:8001/api/v1/nodes/${node_name}/status
done

echo "[5/5] Cleaning up..."
kill -9 ${proxy_pid}

echo "🎉 Done: Nodes have GPU annotations, capacities, and allocatables set."
