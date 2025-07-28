#!/bin/bash

set -euo pipefail

# Default values
DEFAULT_CLUSTER_NAME="a100-cluster"
DEFAULT_NODES=5
DEFAULT_GPUS_PER_NODE=2
DEFAULT_GPU_TYPE="nvidia"
DEFAULT_GPU_MODEL="NVIDIA-A100-PCIE-80GB"
DEFAULT_GPU_MEMORY=81920

# Initialize variables with defaults
cluster_name="$DEFAULT_CLUSTER_NAME"
nodes="$DEFAULT_NODES"
gpus_per_node="$DEFAULT_GPUS_PER_NODE"
gpu_type="$DEFAULT_GPU_TYPE"
gpu_model="$DEFAULT_GPU_MODEL"
gpu_memory="$DEFAULT_GPU_MEMORY"

# cleanup proxy process and remove temp files on exit
cleanup() {
    [[ -n "${proxy_pid:-}" ]] && kill "$proxy_pid" 2>/dev/null || true
    [[ -f "kind-config.yaml" ]] && rm -f "kind-config.yaml"
}
trap cleanup EXIT

# Display usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Options:
    -c CLUSTER_NAME    Cluster name (default: $DEFAULT_CLUSTER_NAME)
    -n NODES          Number of nodes (default: $DEFAULT_NODES)
    -g GPUS           GPUs per node (default: $DEFAULT_GPUS_PER_NODE)
    -t TYPE           GPU type: nvidia, amd, intel, mix (default: $DEFAULT_GPU_TYPE)
    -d MODEL          GPU model (default: $DEFAULT_GPU_MODEL)
    -m MEMORY         GPU memory in MB (default: $DEFAULT_GPU_MEMORY)
    -h                Show this help message

EOF
}

# Function to validate GPU type
validate_gpu_type() {
    local type="$1"
    case "$type" in
        nvidia|amd|intel|mix)
            return 0
            ;;
        *)
            echo "Error: Invalid GPU type '$type'. Valid values: nvidia, amd, intel, mix"
            exit 1
            ;;
    esac
}

# Parse command line arguments
while getopts "c:n:g:t:d:m:h" opt; do
    case $opt in
        c)
            cluster_name="$OPTARG"
            ;;
        n)
            nodes="$OPTARG"
            if ! [[ "$nodes" =~ ^[0-9]+$ ]] || [ "$nodes" -lt 1 ]; then
                echo "Error: Number of nodes must be a positive integer"
                exit 1
            fi
            ;;
        g)
            gpus_per_node="$OPTARG"
            if ! [[ "$gpus_per_node" =~ ^[0-9]+$ ]] || [ "$gpus_per_node" -lt 0 ]; then
                echo "Error: GPUs per node must be a non-negative integer"
                exit 1
            fi
            ;;
        t)
            gpu_type="$OPTARG"
            validate_gpu_type "$gpu_type"
            ;;
        d)
            gpu_model="$OPTARG"
            ;;
        m)
            gpu_memory="$OPTARG"
            if ! [[ "$gpu_memory" =~ ^[0-9]+$ ]] || [ "$gpu_memory" -lt 1 ]; then
                echo "Error: GPU memory must be a positive integer"
                exit 1
            fi
            ;;
        h)
            usage
            exit 0
            ;;
        \?)
            echo "Invalid option: -$OPTARG" >&2
            usage
            exit 1
            ;;
        :)
            echo "Option -$OPTARG requires an argument." >&2
            usage
            exit 1
            ;;
    esac
done

# Function to create Kind cluster
create_kind_cluster() {
    local cluster_name="$1"
    local node_count="$2"

    echo "[1/5] Creating Kind cluster: ${cluster_name} with ${node_count} nodes..."

    # Create kind configuration
    cat <<EOF > kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
EOF

    # Add worker nodes
    for ((i=1; i<node_count; i++)); do
        echo "- role: worker" >> kind-config.yaml
    done

    # Create the cluster verify the cluster was created ok
    if ! kind create cluster --name "${cluster_name}" --config kind-config.yaml; then
        echo "Error: Failed to create Kind cluster"
    exit 1
fi
}

# Main execution starts here
control_plane_node="${cluster_name}-control-plane"

# Create the Kind cluster
create_kind_cluster "$cluster_name" "$nodes"

echo "[2/5] Waiting for node ${control_plane_node} to be ready..."
while [[ $(kubectl get nodes "${control_plane_node}" --no-headers 2>/dev/null | awk '{print $2}') != "Ready" ]]; do
    sleep 1
done

# Function to patch a node with fake GPUs
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

# Patch all nodes with GPU labels
nodes_list=$(kubectl get nodes --no-headers -o custom-columns=":metadata.name")
node_array=($nodes_list)
for i in "${!node_array[@]}"; do
    node_name="${node_array[$i]}"
    if [ "$gpu_type" != "mix" ]; then
        current_type="$gpu_type"
        current_model="$gpu_model"
        current_memory="$gpu_memory"
    else
        mix_index=$((i % 3))
        case $mix_index in
            0)
                current_type="nvidia"
                current_model="NVIDIA-A100-PCIE-80GB"
                current_memory=81920
                ;;
            1)
                current_type="amd"
                current_model="AMD-MI300X-192G"
                current_memory=196608
                ;;
            2)
                current_type="intel"
                current_model="Intel-Gaudi-2-96GB"
                current_memory=98304
                ;;
        esac
    fi
    patch_node_gpu "$node_name" "$current_type" "$gpus_per_node" "$current_model" "$current_memory"
done

echo "[4/5] Starting kubectl proxy..."
kubectl proxy > /dev/null 2>&1 &
proxy_pid=$!
retries=0
while [[ $retries -lt 30 ]]; do
    if curl -s 127.0.0.1:8001/api/v1 > /dev/null 2>&1; then
        echo "Connected to the kubelet for patching the nodes"
        break
    fi
    sleep 1
    ((retries++))
done

if [[ $retries -eq 30 ]]; then
    echo "Calling 'kubectl proxy' did not create a successful connection to the kubelet needed to patch the nodes. Exiting."
    exit 1
fi

# Patch nodes with GPU resource capacity
nodes_list=$(kubectl get nodes --no-headers -o custom-columns=":metadata.name")
node_array=($nodes_list)
for i in "${!node_array[@]}"; do
    node_name="${node_array[$i]}"
    if [ "$gpu_type" != "mix" ]; then
        current_type="$gpu_type"
    else
        mix_index=$((i % 3))
        case $mix_index in
            0)
                current_type="nvidia"
                ;;
            1)
                current_type="amd"
                ;;
            2)
                current_type="intel"
                ;;
        esac
    fi
    echo "- Patching node (add): ${node_name}"
    resource_name="${current_type}.com~1gpu"
    resource_count="${gpus_per_node}"

    curl --header "Content-Type: application/json-patch+json" \
         --request PATCH \
         --data '[{"op":"add","path":"/status/capacity/'${resource_name}'","value":"'${resource_count}'"}]' \
         http://localhost:8001/api/v1/nodes/${node_name}/status
done

echo "[5/5] Cleaning up..."
cleanup

echo "🎉 Done: Nodes have GPU annotations, capacities, and allocatables set."
echo "Cluster: ${cluster_name}, Nodes: ${nodes}, GPUs per node: ${gpus_per_node}, GPU type: ${gpu_type}"
