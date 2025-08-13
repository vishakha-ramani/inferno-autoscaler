#!/usr/bin/env bash

set -euo pipefail

# Configuration
KUBECTL=${KUBECTL:-kubectl}
NAMESPACE=${NAMESPACE:-"inferno-autoscaler-system"}

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
    
    # Check if Thanos querier is available
    if ! ${KUBECTL} get svc thanos-querier -n openshift-monitoring &>/dev/null; then
        log_warning "Thanos querier service not found. Please check OpenShift monitoring setup."
    else
        log_success "Thanos querier service is available"
    fi
    
    log_success "OpenShift user workload monitoring is enabled"
}

# Check if required tools are available
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if kustomize is available
    if ! command -v bin/kustomize &>/dev/null; then
        log_error "kustomize not found. Please run 'make kustomize' first."
        exit 1
    fi
    
    # Check if controller-gen is available
    if ! command -v bin/controller-gen &>/dev/null; then
        log_error "controller-gen not found. Please run 'make controller-gen' first."
        exit 1
    fi
    
    log_success "All prerequisites are available"
}

# Create OpenShift service CA secret
create_openshift_service_ca_secret() {
    log_info "Creating OpenShift service CA secret..."
    
    # Check if secret already exists
    if ${KUBECTL} get secret openshift-service-ca -n ${NAMESPACE} &>/dev/null; then
        log_info "OpenShift service CA secret already exists"
        return 0
    fi
    
    # Get the service CA certificate from OpenShift
    if ! ${KUBECTL} get configmap openshift-service-ca.crt -n openshift-config-managed &>/dev/null; then
        log_error "OpenShift service CA configmap not found. Please check OpenShift installation."
        exit 1
    fi
    
    # Extract and create the secret
    log_info "Extracting OpenShift service CA certificate..."
    ${KUBECTL} get configmap openshift-service-ca.crt -n openshift-config-managed -o jsonpath='{.data.service-ca\.crt}' | \
    ${KUBECTL} create secret generic openshift-service-ca \
        --from-literal=ca.crt="$(cat)" \
        -n ${NAMESPACE}
    
    log_success "OpenShift service CA secret created"
}

# Update cluster role binding for correct service account
update_cluster_role_binding() {
    log_info "Updating cluster role binding for correct service account..."
    
    # Check if the cluster role binding exists
    if ! ${KUBECTL} get clusterrolebinding inferno-autoscaler-monitoring-view &>/dev/null; then
        log_warning "Cluster role binding inferno-autoscaler-monitoring-view not found. Creating it..."
        ${KUBECTL} create clusterrolebinding inferno-autoscaler-monitoring-view \
            --clusterrole=cluster-monitoring-view \
            --serviceaccount=${NAMESPACE}:inferno-autoscaler-controller-manager
    else
        # Update the existing cluster role binding
        log_info "Updating existing cluster role binding..."
        ${KUBECTL} patch clusterrolebinding inferno-autoscaler-monitoring-view --type='json' \
            -p="[{\"op\": \"replace\", \"path\": \"/subjects/0/name\", \"value\": \"inferno-autoscaler-controller-manager\"}, {\"op\": \"replace\", \"path\": \"/subjects/0/namespace\", \"value\": \"${NAMESPACE}\"}]"
    fi
    
    log_success "Cluster role binding updated"
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
    
    # Create OpenShift service CA secret
    create_openshift_service_ca_secret
    
    # Update cluster role binding
    update_cluster_role_binding
    
    # Deploy Inferno controller using Kustomize directly
    log_info "Deploying Inferno controller-manager with OpenShift Prometheus..."
    log_info "Using image: ${IMG}"
    
    # Get the project root directory
    PROJECT_ROOT=$(pwd)
    
    # Set image and namespace in Kustomize
    cd config/manager && ${PROJECT_ROOT}/bin/kustomize edit set image controller=${IMG}
    cd ../openshift && ${PROJECT_ROOT}/bin/kustomize edit set namespace ${NAMESPACE}
    
    # Build and apply the manifests
    log_info "Applying manifests..."
    ${PROJECT_ROOT}/bin/kustomize build . | ${KUBECTL} apply -f -
    
    # Wait for deployment rollout
    log_info "Waiting for deployment rollout..."
    ${KUBECTL} rollout status deployment inferno-autoscaler-controller-manager -n ${NAMESPACE} --timeout=300s
    
    log_success "Inferno Autoscaler deployed successfully!"
}

# Verify deployment
verify_deployment() {
    log_info "=== Verifying Deployment ==="
    
    # Check if OpenShift service CA secret exists
    log_info "Checking OpenShift service CA secret..."
    if ${KUBECTL} get secret openshift-service-ca -n ${NAMESPACE} &>/dev/null; then
        log_success "OpenShift service CA secret exists"
    else
        log_error "OpenShift service CA secret is missing"
        return 1
    fi
    
    # Check Prometheus configuration
    log_info "Verifying Prometheus configuration..."
    PROMETHEUS_URL=$(${KUBECTL} get deployment inferno-autoscaler-controller-manager -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="PROMETHEUS_BASE_URL")].value}')
    INSECURE_SKIP=$(${KUBECTL} get deployment inferno-autoscaler-controller-manager -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="PROMETHEUS_TLS_INSECURE_SKIP_VERIFY")].value}')
    
    log_info "Prometheus URL: ${PROMETHEUS_URL}"
    log_info "Insecure Skip Verify: ${INSECURE_SKIP}"
    
    if [[ "${INSECURE_SKIP}" == "false" ]]; then
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
    
    # Check Thanos querier
    log_info "Checking Thanos querier availability..."
    if ${KUBECTL} get svc thanos-querier -n openshift-monitoring &>/dev/null; then
        log_success "Thanos querier service is available"
    else
        log_warning "Thanos querier service not found"
    fi
    
    # Check ServiceMonitor
    log_info "Checking ServiceMonitor configuration..."
    if ${KUBECTL} get servicemonitor -n ${NAMESPACE} &>/dev/null; then
        log_success "ServiceMonitor is configured"
    else
        log_warning "ServiceMonitor not found"
    fi
    
    # Check controller logs for Prometheus connection
    log_info "Checking controller logs for Prometheus connection..."
    if ${KUBECTL} logs -n ${NAMESPACE} -l app.kubernetes.io/name=inferno-autoscaler --tail=10 | grep -q "Prometheus API validation successful"; then
        log_success "Prometheus API validation successful"
    else
        log_warning "Prometheus API validation status unclear. Check logs manually:"
        log_warning "kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=inferno-autoscaler --tail=20"
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
    echo " OpenShift Prometheus endpoint (Thanos Querier):"
    echo "   https://thanos-querier.openshift-monitoring.svc.cluster.local:9091"
    echo ""
    echo " Verify Prometheus connection:"
    echo "   kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=inferno-autoscaler | grep 'Prometheus API validation'"
    echo ""
}

# Run main function
main "$@" 