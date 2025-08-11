#!/usr/bin/env bash

set -euo pipefail

# Configuration
KUBECTL=${KUBECTL:-kubectl}
NAMESPACE=${NAMESPACE:-"inferno-autoscaler-test"}
IMG=${IMG:-"quay.io/mmunirab/inferno-autoscaler:multi-arch0.5tls"}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# Check if we're connected to an OpenShift cluster
check_openshift() {
    log_info "Checking OpenShift cluster connection..."
    
    if ! ${KUBECTL} get clusterversion &>/dev/null; then
        log_error "Not connected to an OpenShift cluster. Please run 'oc login' first."
        exit 1
    fi
    
    log_success "Connected to OpenShift cluster: $(${KUBECTL} config current-context)"
}

# Check if OpenShift user workload monitoring is enabled
check_user_workload_monitoring() {
    log_info "Checking OpenShift user workload monitoring..."
    
    if ! ${KUBECTL} get namespace openshift-user-workload-monitoring &>/dev/null; then
        log_error "OpenShift user workload monitoring is not enabled."
        log_error "Please enable it by running:"
        log_error "oc patch clusterversion/version --type='merge' -p='{\"spec\":{\"capabilities\":{\"additionalEnabledCapabilities\":[\"openshift-user-workload-monitoring\"]}}}'"
        exit 1
    fi
    
    # Check if Prometheus pods are running
    if ! ${KUBECTL} get pods -n openshift-user-workload-monitoring -l app=prometheus --no-headers | grep -q Running; then
        log_warning "OpenShift Prometheus pods may not be running. Please check:"
        log_warning "kubectl get pods -n openshift-user-workload-monitoring"
    else
        log_success "OpenShift Prometheus is running"
    fi
    
    log_success "OpenShift user workload monitoring is enabled"
}

# Check if required tools are available
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if kustomize is available
    if ! command -v ../../bin/kustomize &>/dev/null; then
        log_error "kustomize not found. Please run 'make kustomize' first."
        exit 1
    fi
    
    # Check if controller-gen is available
    if ! command -v ../../bin/controller-gen &>/dev/null; then
        log_error "controller-gen not found. Please run 'make controller-gen' first."
        exit 1
    fi
    
    log_success "All prerequisites are available"
}

# Deploy Inferno Autoscaler
deploy_inferno() {
    log_info "=== Deploying Inferno Autoscaler with OpenShift Prometheus ==="
    
    # Create namespace
    log_info "Creating namespace..."
    ${KUBECTL} create ns ${NAMESPACE} 2>/dev/null || true
    
    # Install CRDs
    log_info "Installing Inferno CRDs..."
    make install
    sleep 5
    
    # Install configmaps
    log_info "Installing configuration maps..."
    ${KUBECTL} apply -f deploy/configmap-serviceclass.yaml
    ${KUBECTL} apply -f deploy/configmap-accelerator-unitcost.yaml
    
    # Create ServiceAccount for Prometheus access
    log_info "Creating ServiceAccount for Prometheus access..."
    ${KUBECTL} apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: inferno-autoscaler-prometheus
  namespace: ${NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: inferno-autoscaler-prometheus-reader
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints", "nodes"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get"]
- nonResourceURLs: ["/metrics"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: inferno-autoscaler-prometheus-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: inferno-autoscaler-prometheus-reader
subjects:
- kind: ServiceAccount
  name: inferno-autoscaler-prometheus
  namespace: ${NAMESPACE}
EOF
    
    # Deploy Inferno controller using Make target
    log_info "Deploying Inferno controller-manager with OpenShift Prometheus..."
    log_info "Using image: ${IMG}"
    
    # Use the Make target for deployment
    NAMESPACE=${NAMESPACE} IMG=${IMG} make deploy-inferno-on-openshift
    
    log_success "Inferno Autoscaler deployed successfully!"
}

# Verify deployment
verify_deployment() {
    log_info "=== Verifying Deployment ==="
    
    # Check Inferno controller
    log_info "Checking Inferno controller status..."
    if ${KUBECTL} get pods -n ${NAMESPACE} -l app.kubernetes.io/name=inferno-autoscaler | grep -q Running; then
        log_success "Inferno controller is running"
    else
        log_error "Inferno controller is not running properly"
        ${KUBECTL} get pods -n ${NAMESPACE} -l app.kubernetes.io/name=inferno-autoscaler
        return 1
    fi
    
    # Check Prometheus configuration
    log_info "Verifying Prometheus configuration..."
    PROMETHEUS_URL=$(${KUBECTL} get deployment inferno-autoscaler-controller-manager -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="PROMETHEUS_BASE_URL")].value}')
    TLS_ENABLED=$(${KUBECTL} get deployment inferno-autoscaler-controller-manager -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="PROMETHEUS_TLS_ENABLED")].value}')
    INSECURE_SKIP=$(${KUBECTL} get deployment inferno-autoscaler-controller-manager -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="PROMETHEUS_TLS_INSECURE_SKIP_VERIFY")].value}')
    
    log_info "Prometheus URL: ${PROMETHEUS_URL}"
    log_info "TLS Enabled: ${TLS_ENABLED}"
    log_info "Insecure Skip Verify: ${INSECURE_SKIP}"
    
    if [[ "${TLS_ENABLED}" == "true" && "${INSECURE_SKIP}" == "false" ]]; then
        log_success "TLS configuration is secure (certificate validation enabled)"
    else
        log_warning "TLS configuration may not be optimal for production"
    fi
    
    # Check OpenShift Prometheus
    log_info "Checking OpenShift Prometheus availability..."
    if ${KUBECTL} get pods -n openshift-user-workload-monitoring -l app=prometheus | grep -q Running; then
        log_success "OpenShift Prometheus is running"
    else
        log_warning "OpenShift Prometheus may not be running"
        ${KUBECTL} get pods -n openshift-user-workload-monitoring
    fi
    
    # Check ServiceMonitor
    log_info "Checking ServiceMonitor configuration..."
    if ${KUBECTL} get servicemonitor -n ${NAMESPACE} &>/dev/null; then
        log_success "ServiceMonitor is configured"
    else
        log_warning "ServiceMonitor not found"
    fi
    
    log_success "Deployment verification completed!"
}

# Main deployment function
main() {
    log_info "Starting OpenShift deployment of Inferno Autoscaler"
    log_info "Target namespace: ${NAMESPACE}"
    log_info "Image: ${IMG}"
    
    # Check prerequisites
    check_prerequisites
    
    # Check OpenShift connection
    check_openshift
    
    # Check user workload monitoring
    check_user_workload_monitoring
    
    # Deploy components
    deploy_inferno
    
    # Verify deployment
    verify_deployment
    
    # Print success message and next steps
    log_success "=== OpenShift Deployment Complete! ==="
    echo ""
    echo " Inferno Autoscaler successfully deployed on OpenShift!"
    echo ""
    echo " Next Steps:"
    echo "1. Deploy a VariantAutoscaling resource:"
    echo "   kubectl apply -f hack/vllme/deploy/vllme-setup/vllme-variantautoscaling.yaml"
    echo ""
    echo " Monitor the deployment:"
    echo "   kubectl get pods -n $NAMESPACE"
    echo ""
    echo " Check Inferno controller logs:"
    echo "   kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=inferno-autoscaler"
    echo ""
    echo " OpenShift Prometheus endpoint:"
    echo "   https://prometheus-user-workload.openshift-user-workload-monitoring.svc.cluster.local:9091"
    echo ""
    echo " For production TLS setup:"
    echo "   1. Install cert-manager: kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml"
    echo "   2. Create certificates: kubectl apply -f config/certmanager/prometheus_client_certificate.yaml"
    echo "   3. Apply certificate patch: kubectl apply -f config/default/cert_prometheus_client_patch.yaml"
    echo "   4. Restart controller: kubectl rollout restart deployment inferno-autoscaler-controller-manager -n $NAMESPACE"
}

# Run main function
main "$@" 