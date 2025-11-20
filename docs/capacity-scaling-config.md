# Capacity Scaling Configuration

## Overview

The Workload Variant Autoscaler supports capacity-based scaling using KV cache utilization and queue length metrics. This feature is enabled by default and configured via a ConfigMap.

## Configuration

### ConfigMap Structure

The capacity scaling configuration is stored in a ConfigMap named `capacity-scaling-config` in the `workload-variant-autoscaler-system` namespace.

**Location:** `deploy/configmap-capacity-scaling.yaml`

### Parameters

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `kvCacheThreshold` | float64 | Replica is considered saturated if KV cache utilization ≥ threshold (0.0-1.0) | 0.80 |
| `queueLengthThreshold` | int | Replica is considered saturated if queue length ≥ threshold | 5 |
| `kvSpareTrigger` | float64 | Scale-up signal if average spare KV capacity < trigger (0.0-1.0) | 0.10 |
| `queueSpareTrigger` | int | Scale-up signal if average spare queue capacity < trigger | 3 |

### Default Configuration

The default configuration is automatically used if:
- The ConfigMap is not deployed
- The ConfigMap exists but has no `default` entry
- An entry fails validation

**Default values:**
```yaml
kvCacheThreshold: 0.80
queueLengthThreshold: 5
kvSpareTrigger: 0.1
queueSpareTrigger: 3
```

## Usage

### 1. Using Default Configuration

Simply deploy the controller without the ConfigMap. The system will log a warning and use hardcoded defaults:

```
WARN Capacity scaling ConfigMap not found, using hardcoded defaults
```

### 2. Customizing Global Defaults

Edit `deploy/configmap-capacity-scaling.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: capacity-scaling-config
  namespace: workload-variant-autoscaler-system
data:
  default: |
    kvCacheThreshold: 0.75
    queueLengthThreshold: 10
    kvSpareTrigger: 0.15
    queueSpareTrigger: 5
```

Apply the ConfigMap:
```bash
kubectl apply -f deploy/configmap-capacity-scaling.yaml
```

### 3. Per-Model Overrides

Add model-specific configuration entries to override defaults for specific model/namespace pairs:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: capacity-scaling-config
  namespace: workload-variant-autoscaler-system
data:
  default: |
    kvCacheThreshold: 0.80
    queueLengthThreshold: 5
    kvSpareTrigger: 0.1
    queueSpareTrigger: 3

  # Override for granite model in production namespace
  granite-production: |
    model_id: ibm/granite-13b
    namespace: production
    kvCacheThreshold: 0.85
    kvSpareTrigger: 0.15

  # Override for llama model in lab namespace
  llama-lab: |
    model_id: meta/llama-70b
    namespace: lab
    queueLengthThreshold: 20
    queueSpareTrigger: 10
```

**Key points:**
- Entry keys (e.g., `granite-production`) can be any descriptive name
- Each override must include `model_id` and `namespace` fields
- Only specified fields are overridden; others inherit from `default`
- Multiple overrides can exist for different model/namespace combinations

### 4. Partial Overrides

You can override only specific parameters while inheriting the rest from defaults:

```yaml
  my-model-override: |
    model_id: my-org/my-model
    namespace: my-namespace
    kvCacheThreshold: 0.90
    # Other fields inherit from default
```

## Validation

The controller validates all configuration entries on load. Invalid entries are logged and skipped:

### Validation Rules

1. **KvCacheThreshold:** Must be between 0.0 and 1.0
2. **QueueLengthThreshold:** Must be ≥ 0
3. **KvSpareTrigger:** Must be between 0.0 and 1.0
4. **QueueSpareTrigger:** Must be ≥ 0
5. **Consistency:** `kvCacheThreshold` must be ≥ `kvSpareTrigger`

### Example Validation Errors

**Invalid entry (logged and skipped):**
```yaml
  invalid-config: |
    model_id: test/model
    namespace: test
    kvCacheThreshold: 1.5  # ERROR: Must be ≤ 1.0
```

**Log output:**
```
WARN Invalid capacity scaling config entry, skipping key=invalid-config error=kvCacheThreshold must be between 0 and 1, got 1.50
```

## Integration with Controller

The capacity scaling configuration is read during controller reconciliation:

```go
// In Reconcile loop
capacityConfigs, err := r.readCapacityScalingConfig(ctx, "capacity-scaling-config", configMapNamespace)
if err != nil {
    return ctrl.Result{}, err
}

// For a specific VariantAutoscaling resource
capacityConfig := r.getCapacityScalingConfigForVariant(
    capacityConfigs,
    va.Spec.ModelID,
    va.Namespace,
)

// Use capacityConfig for capacity-based scaling decisions
if capacityConfig.KvCacheThreshold > 0 {
    // Apply capacity scaling logic
}
```

## Troubleshooting

### ConfigMap Not Found

**Symptom:** Warning log message
```
WARN Capacity scaling ConfigMap not found, using hardcoded defaults configmap=capacity-scaling-config namespace=workload-variant-autoscaler-system
```

**Solution:** Deploy the ConfigMap:
```bash
kubectl apply -f deploy/configmap-capacity-scaling.yaml
```

### Invalid Configuration Entry

**Symptom:** Warning log message
```
WARN Invalid capacity scaling config entry, skipping key=my-config error=...
```

**Solution:** Fix the validation error in the ConfigMap entry and reapply.

### Missing Default Entry

**Symptom:** Warning log message
```
WARN No 'default' entry in capacity scaling ConfigMap, using hardcoded defaults
```

**Solution:** Add a `default` entry to the ConfigMap:
```yaml
data:
  default: |
    kvCacheThreshold: 0.80
    queueLengthThreshold: 5
    kvSpareTrigger: 0.1
    queueSpareTrigger: 3
```

### Override Not Applied

**Symptom:** Model-specific override is not being used

**Checklist:**
1. Verify `model_id` exactly matches `va.Spec.ModelID`
2. Verify `namespace` exactly matches the VariantAutoscaling resource namespace
3. Check controller logs for validation errors
4. Ensure entry passed validation (check for WARN logs)

**Debug log (when override is applied):**
```
DEBUG Applied capacity scaling override key=my-override modelID=ibm/granite-13b namespace=production config={...}
```

## Example: Production Setup

**deploy/configmap-capacity-scaling.yaml:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: capacity-scaling-config
  namespace: workload-variant-autoscaler-system
data:
  # Conservative defaults for most workloads
  default: |
    kvCacheThreshold: 0.80
    queueLengthThreshold: 5
    kvSpareTrigger: 0.1
    queueSpareTrigger: 3

  # High-priority production workload - scale aggressively
  granite-prod: |
    model_id: ibm/granite-13b
    namespace: production
    kvCacheThreshold: 0.70
    queueLengthThreshold: 3
    kvSpareTrigger: 0.20
    queueSpareTrigger: 5

  # Development workload - allow higher saturation
  llama-dev: |
    model_id: meta/llama-70b
    namespace: development
    kvCacheThreshold: 0.90
    queueLengthThreshold: 15
    kvSpareTrigger: 0.05
    queueSpareTrigger: 2
```

Apply the configuration:
```bash
kubectl apply -f deploy/configmap-capacity-scaling.yaml
```

Verify deployment:
```bash
kubectl get cm capacity-scaling-config -n workload-variant-autoscaler-system
kubectl describe cm capacity-scaling-config -n workload-variant-autoscaler-system
```

## API Reference

### Go Structs

**CapacityScalingConfig:**
```go
type CapacityScalingConfig struct {
    ModelID              string  `yaml:"model_id,omitempty"`
    Namespace            string  `yaml:"namespace,omitempty"`
    KvCacheThreshold     float64 `yaml:"kvCacheThreshold"`
    QueueLengthThreshold int     `yaml:"queueLengthThreshold"`
    KvSpareTrigger       float64 `yaml:"kvSpareTrigger"`
    QueueSpareTrigger    int     `yaml:"queueSpareTrigger"`
}
```

**Methods:**
- `DefaultCapacityScalingConfig() CapacityScalingConfig` - Returns hardcoded defaults
- `Validate() error` - Validates configuration values
- `Merge(override CapacityScalingConfig)` - Applies partial override

## Future Enhancements

Potential future features:
- Per-accelerator type overrides
- Time-based configuration (e.g., aggressive scaling during peak hours)
- Dynamic threshold adjustment based on historical metrics
- Global enable/disable flag via environment variable
