# Configuration Guide

This guide explains how to configure Workload-Variant-Autoscaler for your workloads.

## VariantAutoscaling Resource

The `VariantAutoscaling` CR is the primary configuration interface for WVA.

### Basic Example

```yaml
apiVersion: llmd.ai/v1alpha1
kind: VariantAutoscaling
metadata:
  name: llama-8b-autoscaler
  namespace: llm-inference
spec:
  modelName: "meta/llama-3.1-8b"
  serviceClass: "Premium"
  acceleratorType: "A100"
  minReplicas: 1
  maxBatchSize: 256
```

### Complete Reference

For complete field documentation, see the [CRD Reference](crd-reference.md).

## Operating Modes

WVA supports two operating modes controlled by the `EXPERIMENTAL_PROACTIVE_MODEL` environment variable.

### CAPACITY-ONLY Mode (Default)

**Recommended for production.**

- **Behavior**: Reactive scaling based on saturation detection
- **How It Works**: Monitors KV cache usage and queue lengths, scales when thresholds exceeded
- **Configuration**: Uses `capacity-scaling-config` ConfigMap
- **Pros**: Fast response (<30s), predictable, no model training needed
- **Cons**: Reactive (scales after saturation detected)

**Enable:**
```yaml
# Already enabled by default, no configuration needed
# Or explicitly set:
env:
  - name: EXPERIMENTAL_PROACTIVE_MODEL
    value: "false"
```

### HYBRID Mode (Experimental)

**Not recommended for production.**

- **Behavior**: Combines capacity analyzer with model-based optimizer
- **How It Works**:
  1. Runs capacity analyzer for saturation detection
  2. Runs model-based optimizer for proactive scaling
  3. Arbitrates between the two (capacity safety overrides)
- **Pros**: Proactive scaling (can scale before saturation)
- **Cons**: Slower (~60s), requires model training, experimental

**Enable:**
```yaml
env:
  - name: EXPERIMENTAL_PROACTIVE_MODEL
    value: "true"
```

**Recommendation:** Stick with CAPACITY-ONLY mode unless you have specific proactive scaling requirements.

See [Capacity Analyzer Documentation](../../docs/capacity-analyzer.md) for configuration details.

## ConfigMaps

WVA uses two ConfigMaps for cluster-wide configuration.

### Accelerator Unit Cost ConfigMap

Defines GPU pricing for cost optimization:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: accelerator-unitcost
  namespace: workload-variant-autoscaler-system
data:
  accelerators: |
    - name: A100
      type: NVIDIA-A100-PCIE-80GB
      cost: 40
      memSize: 81920
    - name: MI300X
      type: AMD-MI300X-192GB
      cost: 65
      memSize: 196608
    - name: H100
      type: NVIDIA-H100-80GB-HBM3
      cost: 80
      memSize: 81920
```

### Service Class ConfigMap

Defines SLO requirements for different service tiers:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: serviceclass
  namespace: workload-variant-autoscaler-system
data:
  serviceClasses: |
    - name: Premium
      model: meta/llama-3.1-8b
      priority: 1
      slo-itl: 24        # Time per output token (ms)
      slo-ttw: 500       # Time to first token (ms)
      
    - name: Standard
      model: meta/llama-3.1-8b
      priority: 5
      slo-itl: 50
      slo-ttw: 1000
      
    - name: Freemium
      model: meta/llama-3.1-8b
      priority: 10
      slo-itl: 100
      slo-ttw: 2000
```

## Configuration Options

### Model-Specific Settings

- **modelName**: Identifier for your model (e.g., "meta/llama-3.1-8b")
- **serviceClass**: Service tier (must match ConfigMap)
- **acceleratorType**: Preferred GPU type (e.g., "A100", "MI300X")

### Scaling Parameters

- **minReplicas**: Minimum number of replicas (default: 1)
- **maxBatchSize**: Maximum batch size for inference
- **keepAccelerator**: Pin to specific accelerator type (true/false)

### Cost Configuration

#### variantCost (Optional)

Specifies the cost per replica for this variant, used in capacity-based cost optimization.

```yaml
spec:
  modelID: "meta/llama-3.1-8b"
  variantCost: 15.5  # Cost per replica (default: 10.0)
  modelProfile:
    accelerators:
      - acc: "A100"
        accCount: 1
```

**Default:** 10.0
**Validation:** Must be >= 0

**Use Cases:**
- **Differentiated Pricing**: Higher cost for premium accelerators (H100) vs. standard (A100)
- **Multi-Tenant Cost Tracking**: Assign different costs per customer/tenant
- **Cost-Based Optimization**: Capacity analyzer prefers lower-cost variants when multiple variants can handle load

**Example:**
```yaml
# Premium variant (H100, higher cost)
spec:
  modelID: "meta/llama-3.1-70b"
  variantCost: 80.0
  modelProfile:
    accelerators:
      - acc: "H100"

# Standard variant (A100, lower cost)
spec:
  modelID: "meta/llama-3.1-70b"
  variantCost: 40.0
  modelProfile:
    accelerators:
      - acc: "A100"
```

**Behavior:**
- Capacity analyzer uses `variantCost` when deciding which variant to scale
- If costs are equal, chooses variant with most available capacity
- Does not affect model-based optimization (uses accelerator unit costs)

### Advanced Options

See [CRD Reference](crd-reference.md) for advanced configuration options.

## Best Practices

### Choosing Service Classes

- **Premium**: Latency-sensitive applications (chatbots, interactive AI)
- **Standard**: Moderate latency requirements (content generation)
- **Freemium**: Best-effort, cost-optimized (batch processing)

### Batch Size Tuning

Batch size affects throughput and latency performance:
- WVA **mirrors** the vLLM server's configured batch size (e.g., `--max-num-seqs`)
- Do not override `maxBatchSize` in VariantAutoscaling unless you also change the vLLM server configuration
- When tuning batch size, update **both** the vLLM server argument and the WVA VariantAutoscaling spec together
- Monitor SLO compliance after any batch size changes

## Monitoring Configuration

WVA exposes metrics for monitoring and integrates with HPA for automatic scaling.

### Safety Net Behavior

WVA includes a **safety net** that prevents HPA from using stale metrics during failures:

1. **Normal Operation**: Emits `inferno_desired_replicas` with optimized targets
2. **Capacity Analysis Fails**:
   - Uses previous desired replicas (from last successful run)
   - If unavailable, uses current replicas (safe no-op)
3. **Log Messages**: Watch for `"Safety net activated"` in controller logs

**Check Safety Net Activation:**
```bash
# Controller logs
kubectl logs -n llm-d-scheduler deployment/wva-controller | grep "Safety net activated"

# Should see:
# "Safety net activated: emitted fallback metrics"
#   variant=my-va
#   currentReplicas=2
#   desiredReplicas=2
#   fallbackSource=current-replicas
```

**Why This Matters:**
- Prevents HPA from scaling based on stale metrics
- Provides graceful degradation during Prometheus outages
- Emits safe no-op signals (current=desired) when no history available

### Prometheus Metrics

See:
- [Prometheus Integration](../integrations/prometheus.md)
- [Custom Metrics](../integrations/prometheus.md#custom-metrics)

## Examples

More configuration examples in:
- [config/samples/](../../config/samples/)
- [Tutorials](../tutorials/)

## Troubleshooting Configuration

### Common Issues

**SLOs not being met:**
- Verify service class configuration matches workload
- Check if accelerator has sufficient capacity
- Review model parameter estimates (alpha, beta values)

**Cost too high:**
- Consider allowing accelerator flexibility (`keepAccelerator: false`)
- Review service class priorities
- Check if min replicas can be reduced

## Next Steps

- [Run the Quick Start Demo](../tutorials/demo.md)
- [Integrate with HPA](../integrations/hpa-integration.md)
- [Set up Prometheus monitoring](../integrations/prometheus.md)

