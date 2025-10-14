#!/bin/bash
#
# Workload-Variant-Autoscaler Kubernetes Deployment Script
# Automated deployment of WVA, llm-d infrastructure, Prometheus, and HPA
#
# Prerequisites:
# - kubectl installed and configured
# - helm installed
# - yq installed (optional)
# - Access to a Kubernetes cluster
# - HuggingFace token (for llm-d deployment)
#

set -e  # Exit on error
set -o pipefail  # Exit on pipe failure

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
WVA_PROJECT=${WVA_PROJECT:-$PWD}
BASE_NAME=${BASE_NAME:-"inference-scheduling"}
NAMESPACE=${NAMESPACE:-"llm-d-$BASE_NAME"}
MONITORING_NAMESPACE=${MONITORING_NAMESPACE:-"workload-variant-autoscaler-monitoring"}
WVA_NAMESPACE=${WVA_NAMESPACE:-"workload-variant-autoscaler-system"}
WVA_IMAGE=${WVA_IMAGE:-"quay.io/mmunirab/inferno-autoscaler:0.1.2-multi-arch"}
LLM_D_OWNER=${LLM_D_OWNER:-"llm-d-incubation"}
LLM_D_PROJECT=${LLM_D_PROJECT:-"llm-d-infra"}
LLM_D_RELEASE=${LLM_D_RELEASE:-"v1.3.1"}
MODEL_ID=${MODEL_ID:-"unsloth/Meta-Llama-3.1-8B"}  # Use unsloth version to avoid gated model issues
ACCELERATOR_TYPE=${ACCELERATOR_TYPE:-"A100"}

# Flags for deployment steps
DEPLOY_PROMETHEUS=${DEPLOY_PROMETHEUS:-true}
DEPLOY_WVA=${DEPLOY_WVA:-true}
DEPLOY_LLM_D=${DEPLOY_LLM_D:-true}
DEPLOY_PROMETHEUS_ADAPTER=${DEPLOY_PROMETHEUS_ADAPTER:-true}
DEPLOY_HPA=${DEPLOY_HPA:-true}
USE_VLLM_EMULATOR=${USE_VLLM_EMULATOR:-false}  # Set to true if no GPUs available
SKIP_CHECKS=${SKIP_CHECKS:-false}

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local missing_tools=()
    
    # Check for required tools
    for tool in kubectl helm; do
        if ! command -v $tool &> /dev/null; then
            missing_tools+=($tool)
        fi
    done
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Please install the missing tools and try again"
        exit 1
    fi
    
    # Check Kubernetes connection
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster. Please check your kubeconfig"
        exit 1
    fi
    
    log_success "All prerequisites met"
    log_info "Connected to Kubernetes cluster"
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
            log_warning "Install NVIDIA GPU Operator or set USE_VLLM_EMULATOR=true for demo"
        else
            log_warning "No GPUs detected on host either"
            log_info "Setting USE_VLLM_EMULATOR=true for demo mode"
            USE_VLLM_EMULATOR=true
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
    export USE_VLLM_EMULATOR
    log_info "Using accelerator type: $ACCELERATOR_TYPE"
    log_info "Emulator mode: $USE_VLLM_EMULATOR"
}

create_namespaces() {
    log_info "Creating namespaces..."
    
    for ns in $WVA_NAMESPACE $MONITORING_NAMESPACE $NAMESPACE; do
        if kubectl get namespace $ns &> /dev/null; then
            log_warning "Namespace $ns already exists"
        else
            kubectl create namespace $ns
            log_success "Namespace $ns created"
        fi
    done
}

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
    kubectl create secret tls prometheus-web-tls \
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
        --set prometheus.service.port=9090 \
        --set prometheus.prometheusSpec.web.tlsConfig.cert.secret.name=prometheus-web-tls \
        --set prometheus.prometheusSpec.web.tlsConfig.cert.secret.key=tls.crt \
        --set prometheus.prometheusSpec.web.tlsConfig.keySecret.name=prometheus-web-tls \
        --set prometheus.prometheusSpec.web.tlsConfig.keySecret.key=tls.key \
        --timeout=5m \
        --wait
    
    export PROMETHEUS_URL="https://kube-prometheus-stack-prometheus.$MONITORING_NAMESPACE.svc.cluster.local:9090"
    log_success "kube-prometheus-stack deployed with TLS"
    log_info "Prometheus URL: $PROMETHEUS_URL"
}

deploy_wva_controller() {
    log_info "Deploying Workload-Variant-Autoscaler..."
    
    # Deploy ConfigMaps
    log_info "Deploying service classes and accelerator cost ConfigMaps"
    kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: service-classes-config
  namespace: $WVA_NAMESPACE
data:
  premium.yaml: |
    name: Premium
    priority: 1
    data:
      - model: $MODEL_ID
        slo-tpot: 9
        slo-ttft: 1000
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: accelerator-unit-costs
  namespace: $WVA_NAMESPACE
data:
  $ACCELERATOR_TYPE: |
    {
    "device": "NVIDIA-$ACCELERATOR_TYPE-PCIE-40GB",
    "cost": "40.00"
    }
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: workload-variant-autoscaler-variantautoscaling-config
  namespace: $WVA_NAMESPACE
data:
  PROMETHEUS_BASE_URL: "$PROMETHEUS_URL"
  PROMETHEUS_TLS_INSECURE_SKIP_VERIFY: "true"
  GLOBAL_OPT_INTERVAL: "60s"
  WVA_SCALE_TO_ZERO: "false"
EOF
    
    # Deploy WVA using kustomize with custom image
    log_info "Deploying WVA controller with image: $WVA_IMAGE"
    cd $WVA_PROJECT
    
    # Update image in kustomization
    cd config/manager
    kustomize edit set image controller=$WVA_IMAGE
    cd ../..
    
    # Deploy
    kubectl apply -k config/default
    
    # Deploy ServiceMonitor for WVA
    log_info "Deploying ServiceMonitor for WVA"
    kubectl apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    app.kubernetes.io/name: workload-variant-autoscaler
    control-plane: controller-manager
  name: workload-variant-autoscaler-controller-manager-metrics-monitor
  namespace: $MONITORING_NAMESPACE
spec:
  endpoints:
  - bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    interval: 10s
    path: /metrics
    port: https
    scheme: https
    tlsConfig:
      insecureSkipVerify: true
  namespaceSelector:
    matchNames:
    - $WVA_NAMESPACE
  selector:
    matchLabels:
      app.kubernetes.io/name: workload-variant-autoscaler
      control-plane: controller-manager
EOF
    
    # Wait for WVA to be ready
    log_info "Waiting for WVA controller to be ready..."
    kubectl wait --for=condition=Ready pod -l control-plane=controller-manager -n $WVA_NAMESPACE --timeout=120s || true
    
    log_success "WVA deployment complete"
}

deploy_llm_d_infrastructure() {
    log_info "Deploying llm-d infrastructure..."
    
    # Check for HF_TOKEN
    if [ -z "$HF_TOKEN" ]; then
        log_error "HF_TOKEN environment variable is not set"
        log_info "Please set your HuggingFace token: export HF_TOKEN='your-token-here'"
        exit 1
    fi
    
    # Create HF token secret
    log_info "Creating HuggingFace token secret"
    kubectl create secret generic llm-d-hf-token \
        --from-literal="HF_TOKEN=${HF_TOKEN}" \
        --namespace "${NAMESPACE}" \
        --dry-run=client -o yaml | kubectl apply -f -
    
    # Clone llm-d-infra if not exists
    cd $(dirname $WVA_PROJECT)
    if [ ! -d "$LLM_D_PROJECT" ]; then
        log_info "Cloning llm-d-infra repository (release: $LLM_D_RELEASE)"
        git clone -b $LLM_D_RELEASE -- https://github.com/$LLM_D_OWNER/$LLM_D_PROJECT.git $LLM_D_PROJECT
    else
        log_warning "llm-d-infra directory already exists, using existing"
    fi
    
    # Install dependencies
    log_info "Installing llm-d dependencies"
    cd $LLM_D_PROJECT/quickstart
    bash dependencies/install-deps.sh
    bash gateway-control-plane-providers/install-gateway-provider-dependencies.sh
    
    # Install Gateway provider (kgateway v2.0.3)
    log_info "Installing kgateway v2.0.3"
    if command -v yq &> /dev/null; then
        yq eval '.releases[].version = "v2.0.3"' -i "gateway-control-plane-providers/kgateway.helmfile.yaml"
    fi
    helmfile apply -f "gateway-control-plane-providers/kgateway.helmfile.yaml"
    
    # Configure llm-d for Kubernetes
    log_info "Configuring llm-d infrastructure"
    if command -v yq &> /dev/null; then
        yq eval '.gateway.service.type = "ClusterIP"' -i ../charts/llm-d-infra/values.yaml
    fi
    
    export EXAMPLES_DIR="../quickstart/examples/$BASE_NAME"
    cd $EXAMPLES_DIR
    sed -i.bak "s/llm-d-inference-scheduler/$NAMESPACE/g" helmfile.yaml.gotmpl
    
    # Update model in values.yaml to use unsloth version (avoids gated model issues)
    if command -v yq &> /dev/null; then
        yq eval "(.. | select(. == \"Qwen/Qwen3-0.6B\")) = \"$MODEL_ID\" | (.. | select(. == \"hf://Qwen/Qwen3-0.6B\")) = \"hf://$MODEL_ID\"" -i ms-$BASE_NAME/values.yaml
    else
        # Fallback to sed if yq is not available
        sed -i 's|Qwen/Qwen3-0.6B|'"$MODEL_ID"'|g' ms-$BASE_NAME/values.yaml
        sed -i 's|hf://Qwen/Qwen3-0.6B|hf://'"$MODEL_ID"'|g' ms-$BASE_NAME/values.yaml
    fi
    
    # Also remove modelName from routing section to avoid schema validation errors
    if command -v yq &> /dev/null; then
        yq eval 'del(.routing.modelName)' -i ms-$BASE_NAME/values.yaml
    else
        sed -i '/^routing:/,/^[^ ]/{/modelName:/d;}' ms-$BASE_NAME/values.yaml
    fi
    
    # Deploy llm-d core components
    log_info "Deploying llm-d core components"
    helmfile apply -e kgateway
    
    # Deploy vLLM Service
    log_info "Deploying vLLM Service"
    kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: vllm-service
  namespace: $NAMESPACE
  labels:
    llm-d.ai/model: ms-$BASE_NAME-llm-d-modelservice
spec:
  selector:
    llm-d.ai/model: ms-$BASE_NAME-llm-d-modelservice
  ports:
    - name: vllm
      port: 8200
      protocol: TCP
      targetPort: 8200
  type: ClusterIP
EOF
    
    # Deploy ServiceMonitor for vLLM
    log_info "Deploying ServiceMonitor for vLLM"
    kubectl apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: vllm-servicemonitor
  namespace: $MONITORING_NAMESPACE
  labels:
    llm-d.ai/model: ms-$BASE_NAME-llm-d-modelservice
spec:
  selector:
    matchLabels:
      llm-d.ai/model: ms-$BASE_NAME-llm-d-modelservice
  endpoints:
  - port: vllm
    path: /metrics
    interval: 15s
  namespaceSelector:
    any: true
EOF
    
    # Apply EPP ConfigMap fix (disable prefix-cache-scorer)
    log_info "Applying EPP ConfigMap fix"
    kubectl apply -f - <<EOF
apiVersion: v1
data:
  default-plugins.yaml: |
    apiVersion: inference.networking.x-k8s.io/v1alpha1
    kind: EndpointPickerConfig
    plugins:
    - type: low-queue-filter
      parameters:
        threshold: 128
    - type: lora-affinity-filter
      parameters:
        threshold: 0.999
    - type: least-queue-filter
    - type: least-kv-cache-filter
    - type: random-picker
      parameters:
        maxNumOfEndpoints: 1
    - type: single-profile-handler
    schedulingProfiles:
    - name: default
      plugins:
      - pluginRef: low-queue-filter
      - pluginRef: least-queue-filter
      - pluginRef: random-picker
  plugins-v2.yaml: |
    apiVersion: inference.networking.x-k8s.io/v1alpha1
    kind: EndpointPickerConfig
    plugins:
    - type: queue-scorer
    - type: kv-cache-scorer
    - type: max-score-picker
      parameters:
        maxNumOfEndpoints: 1
    - type: single-profile-handler
    schedulingProfiles:
    - name: default
      plugins:
      - pluginRef: queue-scorer
        weight: 1
      - pluginRef: kv-cache-scorer
        weight: 1
      - pluginRef: max-score-picker
kind: ConfigMap
metadata:
  annotations:
    meta.helm.sh/release-name: gaie-$BASE_NAME
    meta.helm.sh/release-namespace: $NAMESPACE
  labels:
    app.kubernetes.io/managed-by: Helm
  name: gaie-$BASE_NAME-epp
  namespace: $NAMESPACE
EOF
    
    # Restart EPP deployment
    kubectl rollout restart deployment gaie-$BASE_NAME-epp -n $NAMESPACE 2>/dev/null || true
    
    cd $WVA_PROJECT
    log_success "llm-d infrastructure deployment complete"
}

deploy_vllm_emulator() {
    log_info "Deploying vLLM Metrics Emulator (No GPU required)..."
    
    kubectl apply -f $WVA_PROJECT/deploy/examples/vllm-emulator/vllme-setup/vllme-deployment-with-service-and-servicemon.yaml
    
    # Update the deployment to match our model and namespace
    kubectl patch deployment -n $NAMESPACE -l llm-d.ai/model=ms-$BASE_NAME-llm-d-modelservice \
        --type json \
        -p '[{"op": "add", "path": "/spec/template/spec/containers/0/env/-", "value": {"name":"MODEL_NAME","value":"'$MODEL_ID'"}}]' \
        2>/dev/null || log_warning "Could not patch vLLM emulator deployment"
    
    log_success "vLLM Emulator deployment complete"
}

deploy_prometheus_adapter() {
    log_info "Deploying Prometheus Adapter..."
    
    # Add Prometheus community helm repo
    log_info "Adding Prometheus community helm repo"
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts || true
    helm repo update
    
    # Extract Prometheus CA certificate and create ConfigMap
    log_info "Creating prometheus-ca ConfigMap for TLS verification"
    kubectl get secret prometheus-web-tls -n $MONITORING_NAMESPACE -o jsonpath='{.data.tls\.crt}' | base64 -d > /tmp/prometheus-ca.crt
    
    # Create or update prometheus-ca ConfigMap
    kubectl create configmap prometheus-ca --from-file=ca.crt=/tmp/prometheus-ca.crt -n $MONITORING_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    log_success "prometheus-ca ConfigMap created/updated"
    
    # Create prometheus-adapter values for Kubernetes
    cat > /tmp/prometheus-adapter-values-k8s.yaml <<'YAML'
prometheus:
  url: https://kube-prometheus-stack-prometheus.workload-variant-autoscaler-monitoring.svc.cluster.local
  port: 9090

rules:
  external:
  - seriesQuery: 'inferno_desired_replicas{variant_name!="",exported_namespace!=""}'
    resources:
      overrides:
        exported_namespace: {resource: "namespace"}
        variant_name: {resource: "deployment"}  
    name:
      matches: "^inferno_desired_replicas"
      as: "inferno_desired_replicas"
    metricsQuery: 'inferno_desired_replicas{<<.LabelMatchers>>}'

replicas: 2
logLevel: 4

tls:
  enable: false # Inbound TLS (Client → Adapter)

extraVolumes:
  - name: prometheus-ca
    configMap:
      name: prometheus-ca

extraVolumeMounts:
  - name: prometheus-ca
    mountPath: /etc/prometheus-ca
    readOnly: true

extraArguments:
  - --prometheus-ca-file=/etc/prometheus-ca/ca.crt

podSecurityContext:
  fsGroup: null

securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  runAsUser: null
  seccompProfile:
    type: RuntimeDefault
YAML
    
    # Update with actual monitoring namespace
    sed -i "s/workload-variant-autoscaler-monitoring/$MONITORING_NAMESPACE/g" /tmp/prometheus-adapter-values-k8s.yaml
    
    # Deploy Prometheus Adapter
    log_info "Installing Prometheus Adapter via Helm"
    helm upgrade -i prometheus-adapter prometheus-community/prometheus-adapter \
        -n $MONITORING_NAMESPACE \
        -f /tmp/prometheus-adapter-values-k8s.yaml \
        --timeout=3m \
        --wait || {
            log_warning "Prometheus Adapter deployment timed out or failed, but continuing..."
            log_warning "HPA may not work until adapter is healthy"
            log_info "Check adapter status: kubectl get pods -n $MONITORING_NAMESPACE | grep prometheus-adapter"
            log_info "Check adapter logs: kubectl logs -n $MONITORING_NAMESPACE deployment/prometheus-adapter"
        }
    
    log_success "Prometheus Adapter deployment initiated (may still be starting)"
}

create_variant_autoscaling() {
    log_info "Creating VariantAutoscaling resource..."
    
    kubectl apply -f - <<EOF
apiVersion: llmd.ai/v1alpha1
kind: VariantAutoscaling
metadata:
  name: ms-$BASE_NAME-llm-d-modelservice-decode 
  namespace: $NAMESPACE
  labels:
    inference.optimization/acceleratorName: $ACCELERATOR_TYPE
spec:
  modelID: "$MODEL_ID"
  sloClassRef:
    name: service-classes-config
    key: premium.yaml
  modelProfile:
    accelerators:
      - acc: "$ACCELERATOR_TYPE"
        accCount: 1
        perfParms: 
          decodeParms:
            alpha: "6.958"
            beta: "0.042"
          prefillParms:
            gamma: "5.2"
            delta: "0.1"
        maxBatchSize: 512
EOF
    
    log_success "VariantAutoscaling resource created"
}

deploy_hpa() {
    log_info "Deploying HorizontalPodAutoscaler..."
    
    kubectl apply -f - <<EOF
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: vllm-deployment-hpa
  namespace: $NAMESPACE
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ms-$BASE_NAME-llm-d-modelservice-decode
  maxReplicas: 10
  minReplicas: 1
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
      - type: Pods
        value: 10
        periodSeconds: 15
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Pods
        value: 1
        periodSeconds: 60
  metrics:
  - type: External
    external:
      metric:
        name: inferno_desired_replicas
        selector:
          matchLabels:
            variant_name: ms-$BASE_NAME-llm-d-modelservice-decode
      target:
        type: AverageValue
        averageValue: "1"
EOF
    
    log_success "HPA deployment complete"
}

verify_deployment() {
    log_info "Verifying deployment..."
    
    local all_good=true
    
    # Check WVA pods
    log_info "Checking WVA controller pods..."
    sleep 10
    if kubectl get pods -n $WVA_NAMESPACE -l control-plane=controller-manager 2>/dev/null | grep -q Running; then
        log_success "WVA controller is running"
    else
        log_error "WVA controller is not running"
        all_good=false
    fi
    
    # Check Prometheus
    log_info "Checking Prometheus..."
    if kubectl get pods -n $MONITORING_NAMESPACE -l app.kubernetes.io/name=prometheus 2>/dev/null | grep -q Running; then
        log_success "Prometheus is running"
    else
        log_warning "Prometheus may still be starting"
    fi
    
    # Check llm-d infrastructure
    if [ "$DEPLOY_LLM_D" = "true" ]; then
        log_info "Checking llm-d infrastructure..."
        if kubectl get deployment -n $NAMESPACE 2>/dev/null | grep -q gaie; then
            log_success "llm-d infrastructure deployed"
        else
            log_warning "llm-d infrastructure may still be deploying"
        fi
    fi
    
    # Check VariantAutoscaling
    log_info "Checking VariantAutoscaling resource..."
    if kubectl get variantautoscaling ms-$BASE_NAME-llm-d-modelservice-decode -n $NAMESPACE &> /dev/null; then
        log_success "VariantAutoscaling resource exists"
        
        # Show the status with new MetricsReady column
        log_info "VariantAutoscaling Status:"
        kubectl get variantautoscaling -n $NAMESPACE -o wide
    else
        log_error "VariantAutoscaling resource not found"
        all_good=false
    fi
    
    # Check HPA
    if [ "$DEPLOY_HPA" = "true" ]; then
        log_info "Checking HPA..."
        if kubectl get hpa vllm-deployment-hpa -n $NAMESPACE &> /dev/null; then
            log_success "HPA exists"
        else
            log_warning "HPA not found"
        fi
    fi
    
    # Check ServiceMonitors
    log_info "Checking ServiceMonitors..."
    local sm_count=$(kubectl get servicemonitor -n $MONITORING_NAMESPACE 2>/dev/null | grep -E "(vllm|workload-variant)" | wc -l)
    if [ "$sm_count" -gt 0 ]; then
        log_success "ServiceMonitors deployed in $MONITORING_NAMESPACE"
    else
        log_warning "ServiceMonitors not found"
    fi
    
    if [ "$all_good" = true ]; then
        log_success "All components verified successfully!"
    else
        log_warning "Some components failed verification. Check the logs above."
    fi
}

print_summary() {
    echo ""
    echo "=========================================="
    echo " Deployment Summary"
    echo "=========================================="
    echo ""
    echo "WVA Namespace:          $WVA_NAMESPACE"
    echo "LLMD Namespace:         $NAMESPACE"
    echo "Monitoring Namespace:   $MONITORING_NAMESPACE"
    echo "Model:                  $MODEL_ID"
    echo "Accelerator:            $ACCELERATOR_TYPE"
    echo "WVA Image:              $WVA_IMAGE"
    echo "Emulator Mode:          $USE_VLLM_EMULATOR"
    echo ""
    echo "Deployed Components:"
    echo "===================="
    echo "✓ kube-prometheus-stack (Prometheus + Grafana)"
    echo "✓ WVA Controller (with metrics validation)"
    echo "✓ llm-d Infrastructure (Gateway, GAIE, ModelService)"
    if [ "$DEPLOY_PROMETHEUS_ADAPTER" = "true" ]; then
        echo "✓ Prometheus Adapter (external metrics API)"
    fi
    if [ "$DEPLOY_HPA" = "true" ]; then
        echo "✓ HPA (HorizontalPodAutoscaler)"
    fi
    echo "✓ ServiceMonitors (in $MONITORING_NAMESPACE)"
    echo "✓ VariantAutoscaling CR"
    echo ""
    echo "Next Steps:"
    echo "==========="
    echo ""
    echo "1. Check VariantAutoscaling status (NEW MetricsReady column!):"
    echo "   kubectl get variantautoscaling -n $NAMESPACE -o wide"
    echo ""
    echo "2. View detailed status with conditions:"
    echo "   kubectl describe variantautoscaling ms-$BASE_NAME-llm-d-modelservice-decode -n $NAMESPACE"
    echo ""
    echo "3. View WVA logs (see metrics validation in action):"
    echo "   kubectl logs -n $WVA_NAMESPACE -l control-plane=controller-manager -f"
    echo ""
    echo "4. Check external metrics API:"
    echo "   kubectl get --raw \"/apis/external.metrics.k8s.io/v1beta1/namespaces/$NAMESPACE/inferno_desired_replicas\" | jq"
    echo ""
    echo "5. Generate load (if vLLM is running):"
    echo "   kubectl apply -f $WVA_PROJECT/charts/workload-variant-autoscaler/templates/guidellm-job.yaml"
    echo ""
    echo "Important Notes:"
    echo "================"
    echo ""
    echo "• GPU Requirements:"
    echo "  - GPUs must be visible to Kubernetes (check: kubectl get nodes -o json | jq '.items[].status.allocatable[\"nvidia.com/gpu\"]')"
    echo "  - If using KIND cluster, GPUs need special configuration (or use USE_VLLM_EMULATOR=true)"
    echo "  - NVIDIA Device Plugin or GPU Operator must be installed for GPU passthrough"
    echo ""
    echo "• Model Loading:"
    echo "  - Using unsloth/Meta-Llama-3.1-8B (avoids gated model access issues)"
    echo "  - Model loading takes 2-3 minutes on A100 GPUs"
    echo "  - Metrics will appear once model is fully loaded"
    echo "  - WVA will automatically detect metrics and start optimization"
    echo ""
    echo "• Metrics Validation:"
    echo "  - WVA validates vLLM metrics before optimization"
    echo "  - 'Metrics unavailable' warnings are NORMAL during model loading"
    echo "  - System gracefully skips optimization until metrics are available"
    echo "  - Check logs: kubectl logs -n $WVA_NAMESPACE -l control-plane=controller-manager | grep Metrics"
    echo ""
    echo "Troubleshooting:"
    echo "================"
    echo ""
    echo "• Check WVA controller logs (structured JSON format):"
    echo "  kubectl logs -n $WVA_NAMESPACE -l control-plane=controller-manager --tail=50"
    echo ""
    echo "• Check vLLM model loading progress:"
    echo "  kubectl logs -n $NAMESPACE -l llm-d.ai/model=ms-$BASE_NAME-llm-d-modelservice -c vllm --tail=30"
    echo ""
    echo "• Check if metrics are being scraped by Prometheus:"
    echo "  kubectl port-forward -n $MONITORING_NAMESPACE svc/kube-prometheus-stack-prometheus 9090:9090"
    echo "  # Then visit http://localhost:9090 and query: vllm:request_success_total"
    echo ""
    echo "• Check metrics endpoint directly (once model loaded):"
    echo "  POD=\$(kubectl get pod -n $NAMESPACE -l llm-d.ai/model=ms-$BASE_NAME-llm-d-modelservice -o jsonpath='{.items[0].metadata.name}')"
    echo "  kubectl exec -n $NAMESPACE \$POD -c vllm -- curl -s http://localhost:8200/metrics | grep vllm:"
    echo ""
    echo "• View Grafana dashboards:"
    echo "  kubectl port-forward -n $MONITORING_NAMESPACE svc/kube-prometheus-stack-grafana 3000:80"
    echo "  # Username: admin, Password: prom-operator"
    echo ""
    echo "=========================================="
}

cleanup() {
    log_warning "Cleaning up deployment..."
    
    # Delete HPA
    kubectl delete hpa vllm-deployment-hpa -n $NAMESPACE 2>/dev/null || true
    
    # Delete VariantAutoscaling
    kubectl delete variantautoscaling ms-$BASE_NAME-llm-d-modelservice-decode -n $NAMESPACE 2>/dev/null || true
    
    # Delete llm-d infrastructure
    if [ "$DEPLOY_LLM_D" = "true" ]; then
        cd $(dirname $WVA_PROJECT)/$LLM_D_PROJECT/quickstart/examples/$BASE_NAME
        helmfile destroy -e kgateway 2>/dev/null || true
        cd -
    fi
    
    # Delete Prometheus Adapter
    helm uninstall prometheus-adapter -n $MONITORING_NAMESPACE 2>/dev/null || true
    
    # Delete kube-prometheus-stack
    helm uninstall kube-prometheus-stack -n $MONITORING_NAMESPACE 2>/dev/null || true
    
    # Delete WVA
    cd $WVA_PROJECT
    kubectl delete -k config/default 2>/dev/null || true
    
    # Delete namespaces
    kubectl delete namespace $NAMESPACE 2>/dev/null || true
    kubectl delete namespace $WVA_NAMESPACE 2>/dev/null || true
    kubectl delete namespace $MONITORING_NAMESPACE 2>/dev/null || true
    
    log_success "Cleanup complete"
}

# Main deployment flow
main() {
    log_info "Starting Workload-Variant-Autoscaler Kubernetes Deployment"
    log_info "==========================================================="
    echo ""
    
    # Check prerequisites
    if [ "$SKIP_CHECKS" != "true" ]; then
        check_prerequisites
    fi
    
    # Detect GPU type
    detect_gpu_type
    
    # Create namespaces
    create_namespaces
    
    # Deploy Prometheus Stack
    if [ "$DEPLOY_PROMETHEUS" = "true" ]; then
        deploy_prometheus_stack
    else
        log_info "Skipping Prometheus deployment (DEPLOY_PROMETHEUS=false)"
        PROMETHEUS_URL="https://kube-prometheus-stack-prometheus.$MONITORING_NAMESPACE.svc.cluster.local:9090"
    fi
    
    # Deploy WVA
    if [ "$DEPLOY_WVA" = "true" ]; then
        deploy_wva_controller
    else
        log_info "Skipping WVA deployment (DEPLOY_WVA=false)"
    fi
    
    # Deploy llm-d
    if [ "$DEPLOY_LLM_D" = "true" ]; then
        deploy_llm_d_infrastructure
    else
        log_info "Skipping llm-d deployment (DEPLOY_LLM_D=false)"
    fi
    
    # Deploy vLLM Emulator if needed
    if [ "$USE_VLLM_EMULATOR" = "true" ]; then
        deploy_vllm_emulator
    fi
    
    # Deploy Prometheus Adapter
    if [ "$DEPLOY_PROMETHEUS_ADAPTER" = "true" ]; then
        deploy_prometheus_adapter
    else
        log_info "Skipping Prometheus Adapter deployment (DEPLOY_PROMETHEUS_ADAPTER=false)"
    fi
    
    # Create VariantAutoscaling
    create_variant_autoscaling
    
    # Deploy HPA
    if [ "$DEPLOY_HPA" = "true" ]; then
        deploy_hpa
    else
        log_info "Skipping HPA deployment (DEPLOY_HPA=false)"
    fi
    
    # Verify deployment
    verify_deployment
    
    # Print summary
    print_summary
    
    log_success "Deployment complete!"
}

# Handle cleanup if script is called with 'cleanup' argument
if [ "$1" == "cleanup" ]; then
    cleanup
    exit 0
fi

# Run main function
main "$@"

