# Configuration Guide

This guide explains how to configure Workload-Variant-Autoscaler for your workloads.

> **Important: Architecture Limitations**
> 
> WVA's performance model is designed and calibrated for **dense transformer architectures**. If you are using:
> - **Hybrid State Space Models (HSSM)**
> - **Mixture of Experts (MoE)** - e.g., Mixtral, DeepSeek MoE variants
> - **Other optimized/custom architectures**
> 
> Please read the [Architecture Limitations documentation](../design/architecture-limitations.md) to understand how these models may behave differently and what additional considerations are needed.

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

WVA exposes metrics for monitoring. See:
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

