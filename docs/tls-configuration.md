# TLS Configuration for Inferno Autoscaler

This document describes how to configure TLS encryption for the Inferno Autoscaler's connections to external services, particularly Prometheus.

## Platform-Specific Configurations

- **[OpenShift TLS Configuration](tls-configuration-openshift.md)** - Specific configuration for OpenShift clusters with OpenShift's built-in monitoring stack
- **Generic TLS Configuration** - This document covers generic TLS setup for other Kubernetes distributions

## Overview

The Inferno Autoscaler supports TLS encryption for secure communication with Prometheus and other external services. This ensures that sensitive metrics data and configuration information are transmitted securely.

## Prometheus TLS Configuration

### Environment Variables

The following environment variables can be used to configure TLS for Prometheus connections:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PROMETHEUS_BASE_URL` | Prometheus server URL | `http://prometheus-operated.inferno-autoscaler-monitoring.svc.cluster.local:9090` | No |
| `PROMETHEUS_TLS_ENABLED` | Enable TLS encryption | `false` | No |
| `PROMETHEUS_TLS_INSECURE_SKIP_VERIFY` | Skip certificate verification | `false` | No |
| `PROMETHEUS_CA_CERT_PATH` | Path to CA certificate | - | No |
| `PROMETHEUS_CLIENT_CERT_PATH` | Path to client certificate | - | No |
| `PROMETHEUS_CLIENT_KEY_PATH` | Path to client private key | - | No |
| `PROMETHEUS_SERVER_NAME` | Server name for certificate validation | - | No |
| `PROMETHEUS_BEARER_TOKEN` | Bearer token for authentication | - | No |

### ConfigMap Configuration

You can also configure TLS settings via ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: inferno-autoscaler-variantautoscaling-config
  namespace: inferno-autoscaler-system
data:
  PROMETHEUS_BASE_URL: "https://prometheus-operated.inferno-autoscaler-monitoring.svc.cluster.local:9090"
  PROMETHEUS_TLS_ENABLED: "true"
  PROMETHEUS_TLS_INSECURE_SKIP_VERIFY: "false"
  PROMETHEUS_CA_CERT_PATH: "/etc/prometheus-certs/ca.crt"
  PROMETHEUS_CLIENT_CERT_PATH: "/etc/prometheus-certs/tls.crt"
  PROMETHEUS_CLIENT_KEY_PATH: "/etc/prometheus-certs/tls.key"
  PROMETHEUS_SERVER_NAME: "prometheus-operated.inferno-autoscaler-monitoring.svc.cluster.local"
  PROMETHEUS_BEARER_TOKEN: "your-bearer-token-here"
```

## Certificate Management

### Manual Certificate Management

For manual certificate management:

1. Create a secret with your certificates:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: prometheus-client-cert
  namespace: inferno-autoscaler-system
type: kubernetes.io/tls
data:
  ca.crt: <base64-encoded-ca-cert>
  tls.crt: <base64-encoded-client-cert>
  tls.key: <base64-encoded-client-key>
```

2. Mount the secret in the deployment

## Configuration Examples

### Development Environment

For development with self-signed certificates:

```yaml
env:
- name: PROMETHEUS_BASE_URL
  value: "https://prometheus:9090"
- name: PROMETHEUS_TLS_ENABLED
  value: "true"
- name: PROMETHEUS_TLS_INSECURE_SKIP_VERIFY
  value: "true"
```

### Production Environment

For production with proper certificate validation:

```yaml
env:
- name: PROMETHEUS_BASE_URL
  value: "https://prometheus-operated.monitoring.svc.cluster.local:9090"
- name: PROMETHEUS_TLS_ENABLED
  value: "true"
- name: PROMETHEUS_TLS_INSECURE_SKIP_VERIFY
  value: "false"
- name: PROMETHEUS_CA_CERT_PATH
  value: "/etc/prometheus-certs/ca.crt"
- name: PROMETHEUS_CLIENT_CERT_PATH
  value: "/etc/prometheus-certs/tls.crt"
- name: PROMETHEUS_CLIENT_KEY_PATH
  value: "/etc/prometheus-certs/tls.key"
- name: PROMETHEUS_SERVER_NAME
  value: "prometheus-operated.monitoring.svc.cluster.local"
```

## Troubleshooting

### Common Issues

1. **Certificate verification failed**
   - Ensure the CA certificate is properly mounted
   - Verify the server name matches the certificate
   - Check certificate expiration dates

2. **Connection refused**
   - Verify Prometheus is running and accessible
   - Check network policies and firewall rules
   - Ensure the correct port is being used

3. **TLS handshake failed**
   - Verify TLS is enabled on the Prometheus server
   - Check certificate compatibility
   - Ensure proper certificate chain

### Debugging

Enable debug logging to troubleshoot TLS issues:

```yaml
env:
- name: LOG_LEVEL
  value: "debug"
```

Check the controller logs for TLS-related messages:

```bash
kubectl logs -n inferno-autoscaler-system deployment/inferno-autoscaler-controller-manager
```

## Security Considerations

1. **Never use `insecureSkipVerify: true` in production**
2. **Use proper certificate validation**
3. **Rotate certificates regularly**
4. **Use strong cipher suites**
5. **Monitor certificate expiration**

## Migration from HTTP to HTTPS

To migrate from HTTP to HTTPS:

1. Configure Prometheus to serve HTTPS
2. Update the `PROMETHEUS_BASE_URL` to use `https://`
3. Set `PROMETHEUS_TLS_ENABLED=true`
4. Configure certificates
5. Test the connection
6. Update any monitoring configurations 