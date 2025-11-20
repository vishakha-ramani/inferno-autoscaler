# Validation Webhook

WVA includes an admission webhook that validates VariantAutoscaling resources at creation/update time, preventing invalid configurations from entering the cluster.

## What It Validates

### Required Fields
- ✅ `spec.modelID` must not be empty
- ✅ `spec.modelProfile.accelerators` must contain at least one accelerator

### Accelerator Profile Validation
- ✅ `acc` (accelerator name) must not be empty
- ✅ `accCount` must be >= 1
- ✅ `maxBatchSize` must be >= 1

### Performance Parameters Validation
- ✅ If `perfParms.decodeParms` provided, must contain `alpha` and `beta` keys
- ✅ If `perfParms.prefillParms` provided, must contain `gamma` and `delta` keys

### SLOClassRef Consistency
- ✅ If `sloClassRef.name` is set, `sloClassRef.key` must also be set (and vice versa)

## Example Validation Errors

### Missing ModelID
```bash
$ kubectl apply -f invalid-va.yaml
Error from server: admission webhook "vvariantautoscaling.kb.io" denied the request:
validation failed:
  - spec.modelID is required and cannot be empty
```

### Invalid Accelerator
```yaml
spec:
  modelProfile:
    accelerators:
      - acc: "A100"
        accCount: 0  # ❌ Invalid: must be >= 1
```

**Error:**
```
validation failed:
  - spec.modelProfile.accelerators[0].accCount must be at least 1
```

### Incomplete Performance Parameters
```yaml
spec:
  modelProfile:
    accelerators:
      - acc: "A100"
        perfParms:
          decodeParms:
            alpha: "20.5"
            # ❌ Missing 'beta'
```

**Error:**
```
validation failed:
  - spec.modelProfile.accelerators[0].perfParms.decodeParms must contain 'beta' key
```

### Multiple Errors

The webhook accumulates all validation errors and returns them together:

```yaml
spec:
  # ❌ Missing modelID
  modelProfile:
    accelerators: []  # ❌ Empty accelerators list
  sloClassRef:
    name: "premium"  # ❌ key is missing
```

**Error:**
```
validation failed:
  - spec.modelID is required and cannot be empty
  - spec.modelProfile.accelerators must contain at least one accelerator
  - spec.sloClassRef.key is required when spec.sloClassRef.name is specified
```

## Enabling/Disabling the Webhook

### Enable (Default in Production)
```yaml
# Deployment environment variable
env:
  - name: ENABLE_WEBHOOKS
    value: "true"  # Default
```

### Disable (Development/Testing Only)
```yaml
env:
  - name: ENABLE_WEBHOOKS
    value: "false"
```

**⚠️ Warning:** Disabling webhooks allows invalid VAs to be created, which may cause controller errors.

## Certificate Management

The webhook requires TLS certificates for secure admission.

### Development: Auto-Generated Certificates

For development/testing, WVA auto-generates self-signed certificates:
- **No configuration needed**
- Certificates are ephemeral (regenerated on pod restart)
- Not recommended for production

### Production: cert-manager (Recommended)

Use [cert-manager](https://cert-manager.io/) for production certificate management:

#### 1. Install cert-manager
```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
```

#### 2. Create Issuer
```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: wva-webhook-issuer
  namespace: llm-d-scheduler
spec:
  selfSigned: {}
```

#### 3. Create Certificate
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: wva-webhook-cert
  namespace: llm-d-scheduler
spec:
  secretName: wva-webhook-tls
  duration: 8760h  # 1 year
  renewBefore: 720h  # 30 days
  subject:
    organizations:
      - llm-d
  dnsNames:
    - wva-webhook-service.llm-d-scheduler.svc
    - wva-webhook-service.llm-d-scheduler.svc.cluster.local
  issuerRef:
    name: wva-webhook-issuer
    kind: Issuer
```

#### 4. Configure WVA to Use Certificate
```yaml
# In WVA deployment
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
          - --webhook-cert-path=/tmp/k8s-webhook-server/serving-certs
          - --webhook-cert-name=tls.crt
          - --webhook-cert-key=tls.key
        volumeMounts:
          - name: cert
            mountPath: /tmp/k8s-webhook-server/serving-certs
            readOnly: true
      volumes:
        - name: cert
          secret:
            secretName: wva-webhook-tls
```

#### 5. Certificate Auto-Renewal

cert-manager automatically renews certificates before expiration:
- **CertWatcher** in WVA detects certificate changes
- Automatically reloads without pod restart
- Zero-downtime certificate rotation

### Manual Certificate Provisioning

If not using cert-manager, provide your own certificates:

```bash
# Generate certificates
openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -days 365 -nodes \
  -subj "/CN=wva-webhook-service.llm-d-scheduler.svc"

# Create secret
kubectl create secret tls wva-webhook-tls \
  --cert=tls.crt \
  --key=tls.key \
  -n llm-d-scheduler
```

## Troubleshooting

### Webhook Not Running

**Symptom:** VAs created without validation
```bash
# Check webhook configuration
kubectl get validatingwebhookconfigurations | grep variantautoscaling

# Check webhook endpoint
kubectl get svc -n llm-d-scheduler | grep webhook
```

### Certificate Errors

**Symptom:** Webhook admission failures with TLS errors

```bash
# Check certificate secret
kubectl get secret wva-webhook-tls -n llm-d-scheduler

# View certificate details
kubectl get secret wva-webhook-tls -n llm-d-scheduler -o json | \
  jq -r '.data."tls.crt"' | base64 -d | openssl x509 -text | grep "Not After"

# Check cert-manager status (if using cert-manager)
kubectl get certificate -n llm-d-scheduler
kubectl describe certificate wva-webhook-cert -n llm-d-scheduler
```

### Bypassing Webhook (Emergency)

If webhook is blocking legitimate VAs:

```bash
# Temporarily disable webhook
kubectl delete validatingwebhookconfigurations vvariantautoscaling.kb.io

# Or set ENABLE_WEBHOOKS=false in deployment
kubectl set env deployment/wva-controller -n llm-d-scheduler ENABLE_WEBHOOKS=false
```

**⚠️ Remember to re-enable after fixing the issue.**

## Status Conditions

When webhook rejects a VA, it won't be created. However, if a VA is created with missing `modelID` (webhook disabled), the controller sets status conditions:

```yaml
status:
  conditions:
    - type: OptimizationReady
      status: "False"
      reason: InvalidConfiguration
      message: "ModelID is required but not specified in spec"
```

Check conditions:
```bash
kubectl get va my-va -o jsonpath='{.status.conditions[?(@.type=="OptimizationReady")]}'
```

## Best Practices

1. **Always Enable in Production** - Catches configuration errors early
2. **Use cert-manager** - Automatic certificate management and rotation
3. **Monitor Webhook Health** - Alert on webhook admission failures
4. **Test Validation** - Verify webhook catches invalid configs before deploying
5. **Document Certificate Expiry** - Set calendar reminders for manual cert renewal

## Related Documentation

- [CRD Reference](crd-reference.md) - Complete field validation rules
- [Configuration Guide](configuration.md) - Valid configuration examples
- [Troubleshooting](troubleshooting.md) - Common webhook issues
