#!/bin/bash
#
# Workload-Variant-Autoscaler Kubernetes Environment-Specific Configuration
# This script provides Kubernetes-specific functions and variable overrides
# It is sourced by the main install.sh script
# Note: it is NOT meant to be executed directly
#

set -e  # Exit on error
set -o pipefail  # Exit on pipe failure

#
# Kubernetes-specific Prometheus Configuration
# Note: overriding defaults from common script
#
PROMETHEUS_BASE_URL="https://kube-prometheus-stack-prometheus.$MONITORING_NAMESPACE.svc.cluster.local"
PROMETHEUS_PORT="9090"
PROMETHEUS_URL=${PROMETHEUS_URL:-"$PROMETHEUS_BASE_URL:$PROMETHEUS_PORT"}
DEPLOY_PROMETHEUS=${DEPLOY_PROMETHEUS:-true}

check_specific_prerequisites() {
    log_info "Checking Kubernetes-specific prerequisites..."
    
    local missing_tools=()
    
    # Check for required tools (including Kubernetes-specific ones)
    for tool in kubectl helm yq; do
        if ! command -v $tool &> /dev/null; then
            missing_tools+=($tool)
        fi
    done
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Please install the missing tools and try again"
        exit 1
    fi
    
    log_success "All Kubernetes prerequisites met"
}

# Deploy WVA prerequisites for Kubernetes
deploy_wva_prerequisites() {
    log_info "Deploying Workload-Variant-Autoscaler prerequisites for Kubernetes..."

    # Extract Prometheus CA certificate
    log_info "Extracting Prometheus TLS certificate"
    kubectl get secret $PROMETHEUS_SECRET_NAME -n $MONITORING_NAMESPACE -o jsonpath='{.data.tls\.crt}' | base64 -d > $PROM_CA_CERT_PATH

    # Update Prometheus URL in configmap
    log_info "Updating Prometheus URL in config/manager/configmap.yaml"
    sed -i.bak "s|PROMETHEUS_BASE_URL:.*|PROMETHEUS_BASE_URL: \"$PROMETHEUS_URL\"|" config/manager/configmap.yaml

    log_success "WVA prerequisites complete"
}

detect_cluster_environment() {
    log_info "Detecting cluster environment..."
    
    # Auto-detect cluster type if not specified
    if [ "$CLUSTER_TYPE" = "auto" ]; then
        # Check if this is a Kind cluster
        if kubectl config current-context | grep -q "kind"; then
            CLUSTER_TYPE="kind"
            log_info "Detected Kind cluster"
        elif kubectl get nodes -o json | jq -r '.items[].metadata.labels["kubernetes.io/hostname"]' | grep -q "kind"; then
            CLUSTER_TYPE="kind"
            log_info "Detected Kind cluster (by node hostname)"
        else
            CLUSTER_TYPE="production"
            log_info "Detected production cluster"
        fi
    fi
    
    # Auto-detect TLS verification setting if not specified
    if [ "$SKIP_TLS_VERIFY" = "auto" ]; then
        case "$CLUSTER_TYPE" in
            "kind")
                SKIP_TLS_VERIFY="true"
                log_info "Kind cluster detected - enabling TLS skip verification for self-signed certificates"
                ;;
            "production")
                SKIP_TLS_VERIFY="false"
                log_info "Production cluster detected - enabling strict TLS verification"
                ;;
            *)
                SKIP_TLS_VERIFY="false"
                log_warning "Unknown cluster type - defaulting to strict TLS verification"
                ;;
        esac
    fi
    
    # Set logging level based on environment
    if [ "$CLUSTER_TYPE" = "kind" ]; then
        WVA_LOG_LEVEL="debug"
        log_info "Development environment - using debug logging"
    else
        WVA_LOG_LEVEL="info"
        log_info "Production environment - using info logging"
    fi
    
    export CLUSTER_TYPE
    export SKIP_TLS_VERIFY
    export WVA_LOG_LEVEL
    
    log_success "Environment detection complete:"
    echo "    Cluster Type:        $CLUSTER_TYPE"
    echo "    Skip TLS Verify:     $SKIP_TLS_VERIFY"
    echo "    Log Level:           $WVA_LOG_LEVEL"
    echo ""
}

detect_gpu_type() {
    log_info "Detecting GPU type in cluster..."
    
    # Check if GPUs are visible to Kubernetes
    local gpu_count=$(kubectl get nodes -o json | jq -r '.items[].status.allocatable["nvidia.com/gpu"]' | grep -v null | head -1)
    
    if [ -z "$gpu_count" ] || [ "$gpu_count" == "null" ]; then
        log_warning "No GPUs visible to Kubernetes"
        log_warning "GPUs may exist on host but need NVIDIA Device Plugin or GPU Operator"
        
        # Check if GPUs exist on host
        if nvidia-smi &> /dev/null; then
            log_info "nvidia-smi detected GPUs on host:"
            nvidia-smi --query-gpu=name,memory.total --format=csv,noheader | head -5
            log_warning "Install NVIDIA GPU Operator or set DEPLOY_VLLM_EMULATOR=true for demo"
        else
            log_warning "No GPUs detected on host either"
            log_info "Setting DEPLOY_VLLM_EMULATOR=true for demo mode"
            DEPLOY_VLLM_EMULATOR=true
        fi
    else
        log_success "GPUs visible to Kubernetes: $gpu_count GPU(s) per node"
        
        # Detect GPU type from labels
        local gpu_product=$(kubectl get nodes -o json | jq -r '.items[] | select(.status.allocatable["nvidia.com/gpu"] != null) | .metadata.labels["nvidia.com/gpu.product"]' | head -1)
        
        if [ -n "$gpu_product" ]; then
            log_success "Detected GPU: $gpu_product"
            
            # Map GPU product to accelerator type
            case "$gpu_product" in
                *H100*)
                    ACCELERATOR_TYPE="H100"
                    ;;
                *A100*)
                    ACCELERATOR_TYPE="A100"
                    ;;
                *L40S*)
                    ACCELERATOR_TYPE="L40S"
                    ;;
                *)
                    log_warning "Unknown GPU type: $gpu_product, using default: $ACCELERATOR_TYPE"
                    ;;
            esac
        fi
    fi
    
    export ACCELERATOR_TYPE
    export DEPLOY_VLLM_EMULATOR
    log_info "Using detected accelerator type: $ACCELERATOR_TYPE"
    log_info "Emulator mode: $DEPLOY_VLLM_EMULATOR"
}

create_namespaces() {
    log_info "Creating namespaces..."
    
    for ns in $WVA_NS $MONITORING_NAMESPACE $LLMD_NS; do
        if kubectl get namespace $ns &> /dev/null; then
            log_info "Namespace $ns already exists"
        else
            kubectl create namespace $ns
            log_success "Namespace $ns created"
        fi
    done
}

# Deploy Prometheus on Kubernetes
deploy_prometheus_stack() {
    log_info "Deploying kube-prometheus-stack with TLS..."
    
    # Add helm repo
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts || true
    helm repo update
    
    # Create self-signed TLS certificate for Prometheus
    log_info "Creating self-signed TLS certificate for Prometheus"
    openssl req -x509 -newkey rsa:2048 -nodes \
        -keyout /tmp/prometheus-tls.key \
        -out /tmp/prometheus-tls.crt \
        -days 365 \
        -subj "/CN=prometheus" \
        -addext "subjectAltName=DNS:kube-prometheus-stack-prometheus.${MONITORING_NAMESPACE}.svc.cluster.local,DNS:kube-prometheus-stack-prometheus.${MONITORING_NAMESPACE}.svc,DNS:prometheus,DNS:localhost" \
        &> /dev/null
    
    # Create Kubernetes secret with TLS certificate
    log_info "Creating Kubernetes secret for Prometheus TLS"
    kubectl create secret tls $PROMETHEUS_SECRET_NAME \
        --cert=/tmp/prometheus-tls.crt \
        --key=/tmp/prometheus-tls.key \
        -n $MONITORING_NAMESPACE \
        --dry-run=client -o yaml | kubectl apply -f - &> /dev/null
    
    # Clean up temp files
    rm -f /tmp/prometheus-tls.{key,crt}
    
    # Install kube-prometheus-stack with TLS enabled
    log_info "Installing kube-prometheus-stack with TLS configuration"
    helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
        -n $MONITORING_NAMESPACE \
        --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
        --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false \
        --set prometheus.service.type=ClusterIP \
        --set prometheus.service.port=$PROMETHEUS_PORT \
        --set prometheus.prometheusSpec.web.tlsConfig.cert.secret.name=$PROMETHEUS_SECRET_NAME \
        --set prometheus.prometheusSpec.web.tlsConfig.cert.secret.key=tls.crt \
        --set prometheus.prometheusSpec.web.tlsConfig.keySecret.name=$PROMETHEUS_SECRET_NAME \
        --set prometheus.prometheusSpec.web.tlsConfig.keySecret.key=tls.key \
        --timeout=5m \
        --wait
    
    log_success "kube-prometheus-stack deployed with TLS"
    log_info "Prometheus URL: $PROMETHEUS_URL"
}

# Kubernetes-specific Undeployment functions
undeploy_prometheus_stack() {
    log_info "Uninstalling kube-prometheus-stack..."
    
    helm uninstall kube-prometheus-stack -n $MONITORING_NAMESPACE 2>/dev/null || \
        log_warning "Prometheus stack not found or already uninstalled"

    kubectl delete secret $PROMETHEUS_SECRET_NAME -n $MONITORING_NAMESPACE --ignore-not-found

    log_success "Prometheus stack uninstalled"
}

delete_namespaces() {
    log_info "Deleting namespaces..."
    
    for ns in $LLMD_NS $WVA_NS $MONITORING_NAMESPACE; do
        if kubectl get namespace $ns &> /dev/null; then
            if [[ "$ns" == "$LLMD_NS" && "$DEPLOY_LLM_D" == "false" ]] || [[ "$ns" == "$WVA_NS" && "$DEPLOY_WVA" == "false" ]] || [[ "$ns" == "$MONITORING_NAMESPACE" && "$DEPLOY_PROMETHEUS" == "false" ]] ; then
                log_info "Skipping deletion of namespace $ns as it was not deployed"
            else 
                log_info "Deleting namespace $ns..."
                kubectl delete namespace $ns 2>/dev/null || \
                    log_warning "Failed to delete namespace $ns"
            fi
        fi
    done
    
    log_success "Namespaces deleted"
}

# Environment-specific functions are now sourced by the main install.sh script
# Do not call functions directly when sourced

