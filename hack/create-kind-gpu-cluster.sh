#!/bin/bash

set -euo pipefail

cluster_name="kind-inferno-gpu-cluster"
control_plane_node="${cluster_name}-control-plane"
worker1_node="${cluster_name}-worker"
worker2_node="${cluster_name}-worker2"

echo "[1/5] Creating Kind cluster: ${cluster_name}..."

cat <<EOF > kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
EOF

kind create cluster --name "${cluster_name}" --config kind-config.yaml

echo "[2/5] Waiting for node ${control_plane_node} to be ready..."
while [[ $(kubectl get nodes "${control_plane_node}" --no-headers 2>/dev/null | awk '{print $2}') != "Ready" ]]; do
  sleep 1
done

echo "[3/5] Patching GPU labels..."

kubectl patch node "${control_plane_node}" --type merge --patch '
metadata:
  labels:
    nvidia.com/gpu.product: NVIDIA-A100-PCIE-40GB
    nvidia.com/gpu.memory: "40960"
'

kubectl patch node "${worker1_node}" --type merge --patch '
metadata:
  labels:
    amd.com/gpu.product: AMD-RX-7800-XT
    amd.com/gpu.memory: "16384"
'

kubectl patch node "${worker2_node}" --type merge --patch '
metadata:
  labels:
    intel.com/gpu.product: Intel-Arc-A770
    intel.com/gpu.memory: "16384"
'

echo "[4/5] Patching GPU capacity and allocatable..."

for node_name in $(kubectl get nodes --no-headers -o custom-columns=":metadata.name"); do
  echo "- Patching node: ${node_name}"

  if [[ "${node_name}" == "${worker1_node}" ]]; then
      resource_name="amd.com~1gpu"
      resource_count="6"
  elif [[ "${node_name}" == "${worker2_node}" ]]; then
      resource_name="intel.com~1gpu"
      resource_count="4"
  else
      resource_name="nvidia.com~1gpu"
      resource_count="8"
  fi

  kubectl patch node "${node_name}" --type='json' --subresource='status' -p="
  [
    {
      \"op\": \"add\",
      \"path\": \"/status/capacity/${resource_name}\",
      \"value\": \"${resource_count}\"
    },
    {
      \"op\": \"add\",
      \"path\": \"/status/allocatable/${resource_name}\",
      \"value\": \"${resource_count}\"
    }
  ]"
done

echo "[5/5] Done: Nodes have GPU annotations, capacities, and allocatables set."

echo
echo "Summary: GPU resource capacities and allocatables for cluster '${cluster_name}':"
echo "-------------------------------------------------------------------------------------------------------------------------------"
printf "%-40s %-20s %-10s %-10s %-30s %-10s\n" "Node" "Resource" "Capacity" "Allocatable" "GPU Product" "Memory (MB)"
echo "-------------------------------------------------------------------------------------------------------------------------------"

for node in $(kubectl get nodes --no-headers -o custom-columns=":metadata.name"); do
  node_json=$(kubectl get node "$node" -o json)

  for resource in "nvidia.com/gpu" "amd.com/gpu" "intel.com/gpu"; do
    cap=$(echo "$node_json" | jq -r ".status.capacity[\"$resource\"] // empty")
    alloc=$(echo "$node_json" | jq -r ".status.allocatable[\"$resource\"] // empty")

    if [[ -n "$cap" || -n "$alloc" ]]; then
      product=$(echo "$node_json" | jq -r ".metadata.labels[\"${resource}.product\"] // \"-\"")
      memory=$(echo "$node_json" | jq -r ".metadata.labels[\"${resource}.memory\"] // \"-\"")
      printf "%-40s %-20s %-10s %-10s %-30s %-10s\n" "$node" "$resource" "$cap" "$alloc" "$product" "$memory"
    fi
  done
done
echo "-------------------------------------------------------------------------------------------------------------------------------"
