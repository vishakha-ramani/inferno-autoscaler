# OpenShift TLS Configuration for Inferno Autoscaler

This document describes how to configure TLS encryption for the Inferno Autoscaler on OpenShift clusters, specifically for connections to OpenShift's Prometheus service.

## Overview

OpenShift provides its own monitoring stack with Prometheus and Thanos, which requires specific TLS configuration. This guide covers the OpenShift-specific setup for secure communication with OpenShift's Prometheus service.

## OpenShift Prometheus Architecture

OpenShift uses a multi-tier monitoring architecture:
- **Thanos Querier**: Provides the Prometheus API endpoint (`thanos-querier.openshift-monitoring.svc:9091`)
- **User Workload Monitoring**: Separate Prometheus instance for user workloads
- **Service CA**: OpenShift's built-in certificate authority for internal services

## Prerequisites

1. **OpenShift Cluster**: Must be running OpenShift 4.x
2. **User Workload Monitoring**: Must be enabled
3. **Cluster Monitoring View Role**: Required for accessing Prometheus metrics

## Environment Variables for OpenShift

The following environment variables are configured for OpenShift:

| Variable | Value | Description |
|----------|-------|-------------|
| `PROMETHEUS_BASE_URL` | `https://thanos-querier.openshift-monitoring.svc.cluster.local:9091` | OpenShift's Thanos querier endpoint |
| `PROMETHEUS_TLS_ENABLED` | `true` | Enable TLS encryption |
| `PROMETHEUS_TLS_INSECURE_SKIP_VERIFY` | `false` | Verify certificates (production secure) |
| `PROMETHEUS_CA_CERT_PATH` | `/etc/openshift-ca/ca.crt` | OpenShift service CA certificate |
| `PROMETHEUS_CLIENT_CERT_PATH` | `""` | Not needed for server-side validation |
| `PROMETHEUS_CLIENT_KEY_PATH` | `""` | Not needed for server-side validation |
| `PROMETHEUS_SERVER_NAME` | `thanos-querier.openshift-monitoring.svc` | Server name for certificate validation |
| `PROMETHEUS_TOKEN_PATH` | `/var/run/secrets/kubernetes.io/serviceaccount/token` | Service account token for authentication |

## OpenShift-Specific Configuration

### 1. OpenShift Service CA Certificate

OpenShift automatically provides a service CA certificate that signs internal service certificates. This certificate is used for server-side validation:

```bash
# The certificate is available in the openshift-config-managed namespace
kubectl get configmap openshift-service-ca.crt -n openshift-config-managed
```

**Note**: OpenShift deployment uses OpenShift's built-in service CA certificates and does not require any additional certificate management tools.

### 2. Service Account and RBAC

The controller needs the `cluster-monitoring-view` role to access Prometheus metrics:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: inferno-autoscaler-monitoring-view
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-monitoring-view
subjects:
- kind: ServiceAccount
  name: inferno-autoscaler-controller-manager
  namespace: inferno-autoscaler-test
```

### 3. Secret for OpenShift Service CA

The OpenShift service CA certificate is mounted as a secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: openshift-service-ca
  namespace: inferno-autoscaler-test
type: Opaque
data:
  ca.crt: <base64-encoded-openshift-service-ca-cert>
```

## Deployment Configuration

### Kustomize Patch for OpenShift

The OpenShift-specific configuration is applied via a Kustomize patch:

```yaml
# config/openshift/prometheus-patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: PROMETHEUS_BASE_URL
          value: "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091"
        - name: PROMETHEUS_TLS_ENABLED
          value: "true"
        - name: PROMETHEUS_TLS_INSECURE_SKIP_VERIFY
          value: "false"
        - name: PROMETHEUS_TOKEN_PATH
          value: "/var/run/secrets/kubernetes.io/serviceaccount/token"
        - name: PROMETHEUS_CA_CERT_PATH
          value: "/etc/openshift-ca/ca.crt"
        - name: PROMETHEUS_CLIENT_CERT_PATH
          value: ""
        - name: PROMETHEUS_CLIENT_KEY_PATH
          value: ""
        - name: PROMETHEUS_SERVER_NAME
          value: "thanos-querier.openshift-monitoring.svc"
        volumeMounts:
        - name: openshift-service-ca
          mountPath: /etc/openshift-ca
          readOnly: true
      volumes:
      - name: openshift-service-ca
        secret:
          secretName: openshift-service-ca
          items:
          - key: ca.crt
            path: ca.crt
```

## Deployment Process

### Automated Deployment

Use the provided deployment script which handles all OpenShift-specific setup:

```bash
NAMESPACE=inferno-autoscaler-system IMG=quay.io/mmunirab/inferno-autoscaler:multi-arch0.5tls make deploy-inferno-on-openshift
```

The script automatically:
1. Creates the OpenShift service CA secret
2. Updates cluster role bindings
3. Applies the OpenShift-specific configuration
4. Verifies the deployment

### Manual Deployment Steps

If deploying manually:

1. **Create the OpenShift service CA secret**:
```bash
kubectl get configmap openshift-service-ca.crt -n openshift-config-managed -o jsonpath='{.data.service-ca\.crt}' | \
kubectl create secret generic openshift-service-ca \
    --from-literal=ca.crt="$(cat)" \
    -n inferno-autoscaler-system
```

2. **Create cluster role binding**:
```bash
kubectl create clusterrolebinding inferno-autoscaler-monitoring-view \
    --clusterrole=cluster-monitoring-view \
    --serviceaccount=inferno-autoscaler-test:inferno-autoscaler-controller-manager
```

3. **Deploy with OpenShift configuration**:
```bash
make deploy-inferno-on-openshift NAMESPACE=inferno-autoscaler-system IMG=your-image:tag
```

## Verification

### Check TLS Configuration

Verify the TLS configuration is applied correctly:

```bash
kubectl get deployment inferno-autoscaler-controller-manager -n inferno-autoscaler-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="PROMETHEUS_TLS_ENABLED")].value}'
```

### Verify Prometheus Connection

Check the controller logs for successful Prometheus API validation:

```bash
kubectl logs -n inferno-autoscaler-system -l app.kubernetes.io/name=inferno-autoscaler | grep "Prometheus API validation"
```

Expected output:
```
{"level":"INFO","msg":"Prometheus API validation successful with queryqueryup"}
```

### Verify Certificate Loading

Check that the OpenShift service CA certificate is loaded:

```bash
kubectl logs -n inferno-autoscaler-system -l app.kubernetes.io/name=inferno-autoscaler | grep "CA certificate loaded"
```

Expected output:
```
{"level":"INFO","msg":"CA certificate loaded successfullypath/etc/openshift-ca/ca.crt"}
```

## References

- [OpenShift Monitoring Documentation](https://docs.openshift.com/container-platform/latest/monitoring/monitoring-overview.html)
- [OpenShift User Workload Monitoring](https://docs.openshift.com/container-platform/latest/monitoring/enabling-monitoring-for-user-defined-projects.html)
- [OpenShift Service CA](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html) 