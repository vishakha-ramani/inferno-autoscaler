#!/bin/bash
#
# Workload-Variant-Autoscaler OpenShift Deployment Script
# This script automates the deployment of WVA and llm-d infrastructure on OpenShift
#
# Prerequisites:
# - OpenShift CLI (oc) installed and configured
# - kubectl installed
# - helm installed
# - yq installed
# - Access to an OpenShift cluster with admin privileges
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
MONITORING_NAMESPACE=${MONITORING_NAMESPACE:-"openshift-user-workload-monitoring"}
WVA_IMAGE=${WVA_IMAGE:-"ghcr.io/llm-d/workload-variant-autoscaler:v0.0.1"}
LLM_D_OWNER=${LLM_D_OWNER:-"llm-d-incubation"}
LLM_D_PROJECT=${LLM_D_PROJECT:-"llm-d-infra"}
LLM_D_RELEASE=${LLM_D_RELEASE:-"v1.3.1"}
MODEL_ID=${MODEL_ID:-"unsloth/Meta-Llama-3.1-8B"}
ACCELERATOR_TYPE=${ACCELERATOR_TYPE:-"H100"}

# Flags for deployment steps
DEPLOY_WVA=${DEPLOY_WVA:-true}
DEPLOY_LLM_D=${DEPLOY_LLM_D:-true}
DEPLOY_PROMETHEUS_ADAPTER=${DEPLOY_PROMETHEUS_ADAPTER:-true}
DEPLOY_HPA=${DEPLOY_HPA:-true}
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
    for tool in oc kubectl helm yq; do
        if ! command -v $tool &> /dev/null; then
            missing_tools+=($tool)
        fi
    done
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Please install the missing tools and try again"
        exit 1
    fi
    
    # Check OpenShift connection
    if ! oc whoami &> /dev/null; then
        log_error "Not logged into OpenShift cluster. Please run 'oc login' first"
        exit 1
    fi
    
    log_success "All prerequisites met"
    log_info "Connected to OpenShift as: $(oc whoami)"
    log_info "Current project: $(oc project -q)"
}

detect_gpu_type() {
    log_info "Detecting GPU type in cluster..."
    
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
    else
        log_warning "No GPUs detected in cluster, using default: $ACCELERATOR_TYPE"
    fi
    
    export ACCELERATOR_TYPE
    log_info "Using accelerator type: $ACCELERATOR_TYPE"
}

find_thanos_url() {
    log_info "Finding Thanos querier URL..."
    
    local thanos_svc=$(kubectl get svc -n openshift-monitoring thanos-querier -o jsonpath='{.metadata.name}' 2>/dev/null || echo "")
    
    if [ -n "$thanos_svc" ]; then
        THANOS_URL="https://thanos-querier.openshift-monitoring.svc.cluster.local:9091"
        log_success "Found Thanos querier: $THANOS_URL"
    else
        log_error "Thanos querier service not found in openshift-monitoring namespace"
        exit 1
    fi
    
    export THANOS_URL
}

create_namespace() {
    log_info "Creating namespace: $NAMESPACE"
    
    if kubectl get namespace $NAMESPACE &> /dev/null; then
        log_warning "Namespace $NAMESPACE already exists"
    else
        kubectl create namespace $NAMESPACE
        log_success "Namespace $NAMESPACE created"
    fi
}

deploy_wva_controller() {
    log_info "Deploying Workload-Variant-Autoscaler..."
    
    # Update Prometheus URL in configmap
    log_info "Updating Prometheus URL in config/manager/configmap.yaml"
    sed -i.bak "s|PROMETHEUS_BASE_URL:.*|PROMETHEUS_BASE_URL: \"$THANOS_URL\"|" config/manager/configmap.yaml
    
    # Deploy WVA
    log_info "Running: make deploy IMG=$WVA_IMAGE"
    make deploy IMG=$WVA_IMAGE
    
    # Restore backup
    mv config/manager/configmap.yaml.bak config/manager/configmap.yaml
    
    log_success "Workload-Variant-Autoscaler deployed"
    
    # Add RBAC permissions
    log_info "Adding cluster-monitoring-view role to WVA service account"
    oc adm policy add-cluster-role-to-user cluster-monitoring-view \
        -z workload-variant-autoscaler-controller-manager \
        -n workload-variant-autoscaler-system
    
    # Deploy ConfigMaps
    log_info "Deploying service classes and accelerator cost ConfigMaps"
    kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: service-classes-config
  namespace: workload-variant-autoscaler-system
data:
  premium.yaml: |
    name: Premium
    priority: 1
    data:
      - model: default/default
        slo-tpot: 24
        slo-ttft: 500
      - model: llama0-70b
        slo-tpot: 80
        slo-ttft: 500
      - model: $MODEL_ID
        slo-tpot: 9
        slo-ttft: 1000
  freemium.yaml: |
    name: Freemium
    priority: 10
    data:
      - model: granite-13b
        slo-tpot: 200
        slo-ttft: 2000
      - model: llama0-7b
        slo-tpot: 150
        slo-ttft: 1500
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: accelerator-unit-costs
  namespace: workload-variant-autoscaler-system
data:
  A100: |
    {
    "device": "NVIDIA-A100-PCIE-80GB",
    "cost": "40.00"
    }
  MI300X: |
    {
    "device": "AMD-MI300X-192GB",
    "cost": "65.00"
    }
  G2: |
    {
    "device": "Intel-Gaudi-2-96GB",
    "cost": "23.00"
    }
  H100: |
    {
    "device": "NVIDIA-H100-80GB-HBM3",
    "cost": "100.0"
    }
  L40S: |
    {
    "device": "NVIDIA-L40S",
    "cost": "32.00"
    }
EOF
    
    # Deploy ServiceMonitor
    log_info "Deploying ServiceMonitor for WVA"
    kubectl apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    app.kubernetes.io/managed-by: kustomize
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
    - workload-variant-autoscaler-system
  selector:
    matchLabels:
      app.kubernetes.io/name: workload-variant-autoscaler
      control-plane: controller-manager
EOF
    
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
    if [ ! -d "$LLM_D_PROJECT" ]; then
        log_info "Cloning llm-d-infra repository (release: $LLM_D_RELEASE)"
        git clone -b $LLM_D_RELEASE -- https://github.com/$LLM_D_OWNER/$LLM_D_PROJECT.git $LLM_D_PROJECT
    else
        log_warning "llm-d-infra directory already exists, skipping clone"
    fi
    
    # Install dependencies
    log_info "Installing llm-d dependencies"
    cd $WVA_PROJECT/$LLM_D_PROJECT/quickstart
    bash dependencies/install-deps.sh
    bash gateway-control-plane-providers/install-gateway-provider-dependencies.sh
    
    # Install Gateway provider (kgateway v2.0.3)
    log_info "Installing kgateway v2.0.3"
    yq eval '.releases[].version = "v2.0.3"' -i "gateway-control-plane-providers/kgateway.helmfile.yaml"
    helmfile apply -f "gateway-control-plane-providers/kgateway.helmfile.yaml"
    
    # Configure llm-d for NodePort and correct model
    log_info "Configuring llm-d infrastructure"
    yq eval '.gateway.service.type = "NodePort"' -i $WVA_PROJECT/$LLM_D_PROJECT/charts/llm-d-infra/values.yaml
    
    export EXAMPLES_DIR="$WVA_PROJECT/$LLM_D_PROJECT/quickstart/examples/$BASE_NAME"
    cd $EXAMPLES_DIR
    sed -i.bak "s/llm-d-inference-scheduler/$NAMESPACE/g" helmfile.yaml.gotmpl
    yq eval "(.. | select(. == \"Qwen/Qwen3-0.6B\")) = \"$MODEL_ID\" | (.. | select(. == \"hf://Qwen/Qwen3-0.6B\")) = \"hf://$MODEL_ID\"" -i ms-$BASE_NAME/values.yaml
    
    # Deploy llm-d core components
    log_info "Deploying llm-d core components"
    helmfile apply -e kgateway
    
    # Deploy vLLM Service and ServiceMonitor
    log_info "Deploying vLLM Service and ServiceMonitor"
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
      nodePort: 30000
  type: NodePort
---
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
    - type: decision-tree-filter
      name: low-latency-filter
      parameters:
        current:
          pluginRef: low-queue-filter
        nextOnSuccess:
          decisionTree:
            current:
              pluginRef: lora-affinity-filter
            nextOnSuccessOrFailure:
              decisionTree:
                current:
                  pluginRef: least-queue-filter
                nextOnSuccessOrFailure:
                  decisionTree:
                    current:
                      pluginRef: least-kv-cache-filter
        nextOnFailure:
          decisionTree:
            current:
              pluginRef: least-queue-filter
            nextOnSuccessOrFailure:
              decisionTree:
                current:
                  pluginRef: lora-affinity-filter
                nextOnSuccessOrFailure:
                  decisionTree:
                    current:
                      pluginRef: least-kv-cache-filter
    - type: random-picker
      parameters:
        maxNumOfEndpoints: 1
    - type: single-profile-handler
    schedulingProfiles:
    - name: default
      plugins:
      - pluginRef: low-latency-filter
      - pluginRef: random-picker
  plugins-v2.yaml: |
    apiVersion: inference.networking.x-k8s.io/v1alpha1
    kind: EndpointPickerConfig
    plugins:
    - type: queue-scorer
    - type: kv-cache-scorer
    # - type: prefix-cache-scorer
      parameters:
        hashBlockSize: 64
        maxPrefixBlocksToMatch: 256
        lruCapacityPerServer: 31250
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
      # - pluginRef: prefix-cache-scorer
      #   weight: 1
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
    kubectl rollout restart deployment gaie-$BASE_NAME-epp -n $NAMESPACE
    
    cd $WVA_PROJECT
    log_success "llm-d infrastructure deployment complete"
}

deploy_prometheus_adapter() {
    log_info "Deploying Prometheus Adapter..."
    
    # Extract Thanos TLS certificate
    log_info "Extracting Thanos TLS certificate"
    kubectl get secret thanos-querier-tls -n openshift-monitoring -o jsonpath='{.data.tls\.crt}' | base64 -d > /tmp/prometheus-ca.crt
    
    # Create CA ConfigMap
    kubectl create configmap prometheus-ca \
        --from-file=ca.crt=/tmp/prometheus-ca.crt \
        -n $MONITORING_NAMESPACE \
        --dry-run=client -o yaml | kubectl apply -f -
    
    # Add Prometheus community helm repo
    log_info "Adding Prometheus community helm repo"
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts || true
    helm repo update
    
    # Create prometheus-adapter-values-ocp.yaml if it doesn't exist
    if [ ! -f config/samples/prometheus-adapter-values-ocp.yaml ]; then
        log_info "Creating prometheus-adapter-values-ocp.yaml"
        cat > config/samples/prometheus-adapter-values-ocp.yaml <<'YAML'
prometheus:
  url: https://thanos-querier.openshift-monitoring.svc.cluster.local
  port: 9091

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
  enable: false

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
  - --prometheus-token-file=/var/run/secrets/kubernetes.io/serviceaccount/token

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
    fi
    
    # Deploy Prometheus Adapter
    log_info "Installing Prometheus Adapter via Helm"
    helm upgrade -i prometheus-adapter prometheus-community/prometheus-adapter \
        -n $MONITORING_NAMESPACE \
        -f config/samples/prometheus-adapter-values-ocp.yaml
    
    # Add RBAC permissions
    log_info "Adding cluster-monitoring-view role to Prometheus Adapter"
    oc adm policy add-cluster-role-to-user cluster-monitoring-view \
        -z prometheus-adapter \
        -n $MONITORING_NAMESPACE
    
    log_success "Prometheus Adapter deployment complete"
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
    name: premium-slo
    key: opt-125m
  modelProfile:
    accelerators:
      - acc: "H100"
        accCount: 1
        perfParms: 
          decodeParms:
            alpha: "6.958"
            beta: "0.042"
          prefillParms:
            gamma: "5.2"
            delta: "0.1"
        maxBatchSize: 512
      - acc: "L40S"
        accCount: 1
        perfParms: 
          decodeParms:
            alpha: "22.619"
            beta: "0.181"
          prefillParms:
            gamma: "226.19"
            delta: "0.018"
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
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
      - type: Pods
        value: 10
        periodSeconds: 15
    scaleDown:
      stabilizationWindowSeconds: 0
      policies:
      - type: Pods
        value: 10
        periodSeconds: 15
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

apply_vllm_probes() {
    log_info "Applying probe configuration to vLLM deployment..."
    
    # Create probes-patch.yaml if it doesn't exist
    if [ ! -f config/samples/probes-patch.yaml ]; then
        cat > config/samples/probes-patch.yaml <<'YAML'
spec:
  template:
    spec:
      containers:
      - name: vllm
        readinessProbe:
            httpGet:
              path: /health
              port: 8200
              scheme: HTTP
            periodSeconds: 1
            successThreshold: 1
            failureThreshold: 1
            timeoutSeconds: 1
        startupProbe:
            failureThreshold: 600
            initialDelaySeconds: 30
            periodSeconds: 1
            httpGet:
              path: /health
              port: 8200
              scheme: HTTP
YAML
    fi
    
    kubectl patch deployment ms-$BASE_NAME-llm-d-modelservice-decode \
        -n $NAMESPACE \
        --patch-file config/samples/probes-patch.yaml
    
    log_success "Probe configuration applied"
}

verify_deployment() {
    log_info "Verifying deployment..."
    
    local all_good=true
    
    # Check WVA pods
    log_info "Checking WVA controller pods..."
    if kubectl get pods -n workload-variant-autoscaler-system -l app.kubernetes.io/name=workload-variant-autoscaler | grep -q Running; then
        log_success "WVA controller is running"
    else
        log_error "WVA controller is not running"
        all_good=false
    fi
    
    # Check llm-d pods
    log_info "Checking llm-d infrastructure..."
    if kubectl get deployment ms-$BASE_NAME-llm-d-modelservice-decode -n $NAMESPACE &> /dev/null; then
        log_success "vLLM deployment exists"
    else
        log_error "vLLM deployment not found"
        all_good=false
    fi
    
    # Check Prometheus Adapter
    log_info "Checking Prometheus Adapter..."
    if kubectl get pods -n $MONITORING_NAMESPACE -l app.kubernetes.io/name=prometheus-adapter | grep -q Running; then
        log_success "Prometheus Adapter is running"
    else
        log_error "Prometheus Adapter is not running"
        all_good=false
    fi
    
    # Check VariantAutoscaling
    log_info "Checking VariantAutoscaling resource..."
    if kubectl get variantautoscaling ms-$BASE_NAME-llm-d-modelservice-decode -n $NAMESPACE &> /dev/null; then
        log_success "VariantAutoscaling resource exists"
    else
        log_error "VariantAutoscaling resource not found"
        all_good=false
    fi
    
    # Check HPA
    log_info "Checking HPA..."
    if kubectl get hpa vllm-deployment-hpa -n $NAMESPACE &> /dev/null; then
        log_success "HPA exists"
    else
        log_error "HPA not found"
        all_good=false
    fi
    
    # Check external metrics API
    log_info "Checking external metrics API..."
    sleep 30  # Wait for metrics to be available
    if kubectl get --raw "/apis/external.metrics.k8s.io/v1beta1/namespaces/$NAMESPACE/inferno_desired_replicas" &> /dev/null; then
        log_success "External metrics API is accessible"
    else
        log_warning "External metrics API not yet available (may need more time)"
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
    echo "Namespace:              $NAMESPACE"
    echo "Monitoring Namespace:   $MONITORING_NAMESPACE"
    echo "Model:                  $MODEL_ID"
    echo "Accelerator:            $ACCELERATOR_TYPE"
    echo "WVA Image:              $WVA_IMAGE"
    echo ""
    echo "Next Steps:"
    echo "==========="
    echo ""
    echo "1. Check deployment status:"
    echo "   kubectl get pods -n $NAMESPACE"
    echo "   kubectl get variantautoscaling -n $NAMESPACE"
    echo "   kubectl get hpa -n $NAMESPACE"
    echo ""
    echo "2. View WVA logs:"
    echo "   kubectl logs -n workload-variant-autoscaler-system deployment/workload-variant-autoscaler-controller-manager -f"
    echo ""
    echo "3. Check external metrics:"
    echo "   kubectl get --raw \"/apis/external.metrics.k8s.io/v1beta1/namespaces/$NAMESPACE/inferno_desired_replicas\" | jq"
    echo ""
    echo "4. Run e2e tests:"
    echo "   make test-e2e-openshift"
    echo ""
    echo "=========================================="
}

# Main deployment flow
main() {
    log_info "Starting Workload-Variant-Autoscaler OpenShift Deployment"
    log_info "=========================================================="
    echo ""
    
    # Check prerequisites
    if [ "$SKIP_CHECKS" != "true" ]; then
        check_prerequisites
    fi
    
    # Detect GPU type
    detect_gpu_type
    
    # Find Thanos URL
    find_thanos_url
    
    # Create namespace
    create_namespace
    
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
    
    # Apply vLLM probes
    apply_vllm_probes
    
    # Verify deployment
    verify_deployment
    
    # Print summary
    print_summary
    
    log_success "Deployment complete!"
}

# Run main function
main "$@"

