# TLS Configuration for Inferno Autoscaler on OpenShift

This document describes how to configure TLS encryption for the Inferno Autoscaler on OpenShift clusters for secure communication with OpenShift's Prometheus service.

## Overview

OpenShift provides a built-in monitoring stack with Prometheus and Thanos that requires specific TLS configuration. The Inferno Autoscaler uses OpenShift's service CA certificates and service account tokens for secure authentication.

## OpenShift Monitoring Architecture

- **Thanos Querier**: Provides the Prometheus API endpoint (`thanos-querier.openshift-monitoring.svc:9091`)
- **User Workload Monitoring**: Separate Prometheus instance for user workloads
- **Service CA**: OpenShift's built-in certificate authority for internal services

## Prerequisites

- OpenShift 4.x cluster
- User Workload Monitoring enabled
- Cluster monitoring view role access

## Configuration

### Environment Variables

| Variable | Value | Description |
|----------|-------|-------------|
| `PROMETHEUS_BASE_URL` | `https://thanos-querier.openshift-monitoring.svc.cluster.local:9091` | OpenShift's Thanos querier endpoint |
| `PROMETHEUS_TLS_INSECURE_SKIP_VERIFY` | `false` | Verify certificates (production secure) |
| `PROMETHEUS_CA_CERT_PATH` | `/etc/openshift-ca/ca.crt` | OpenShift service CA certificate |
| `PROMETHEUS_SERVER_NAME` | `thanos-querier.openshift-monitoring.svc` | Server name for certificate validation |
| `PROMETHEUS_TOKEN_PATH` | `/var/run/secrets/kubernetes.io/serviceaccount/token` | Service account token for authentication |

### Deployment Configuration

The OpenShift-specific configuration is applied via Kustomize:

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
        - name: PROMETHEUS_TLS_INSECURE_SKIP_VERIFY
          value: "false"
        - name: PROMETHEUS_TOKEN_PATH
          value: "/var/run/secrets/kubernetes.io/serviceaccount/token"
        - name: PROMETHEUS_CA_CERT_PATH
          value: "/etc/openshift-ca/ca.crt"
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

## Deployment

### Automated Deployment

Use the provided deployment script which handles all OpenShift-specific setup:

```bash
NAMESPACE=inferno-autoscaler-system IMG=quay.io/infernoautoscaler/inferno-controller:latest make deploy-inferno-on-openshift
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
    --serviceaccount=inferno-autoscaler-system:inferno-autoscaler-controller-manager
```

3. **Deploy with OpenShift configuration**:
```bash
make deploy-inferno-on-openshift NAMESPACE=inferno-autoscaler-system IMG=your-image:tag
```

## Verification

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

### Debugging

Enable debug logging:

```yaml
env:
- name: LOG_LEVEL
  value: "debug"
```

Check controller logs:

```bash
kubectl logs -n inferno-autoscaler-system deployment/inferno-autoscaler-controller-manager
```

Test Thanos querier connectivity:

```bash
kubectl run test-thanos --image=curlimages/curl --rm -it --restart=Never -- \
  curl -k "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091/api/v1/query?query=up"
```

## Security Considerations

- OpenShift automatically manages service CA certificates
- Service account tokens are automatically rotated
- TLS 1.2+ is enforced by default
- Certificate validation is mandatory (no insecure skip verify)

## References

- [OpenShift Monitoring Documentation](https://docs.openshift.com/container-platform/latest/monitoring/monitoring-overview.html)
- [OpenShift Service CA](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html) 