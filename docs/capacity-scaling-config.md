# Capacity Scaling Configuration

## Overview

The Workload Variant Autoscaler supports capacity-based scaling using KV cache utilization and queue length metrics. This feature is enabled by default and configured via a ConfigMap.

**Key features:**
- ✅ ConfigMap-based configuration with global defaults and per-model overrides
- ✅ **Efficient caching** with single read on startup (zero API calls during reconciliation)
- ✅ **Automatic reload** via ConfigMap watch (immediate response to changes)
- ✅ **Thread-safe** concurrent access with RWMutex
- ✅ Graceful degradation to defaults if ConfigMap missing

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

**Note:** Changes take effect immediately! The controller watches the ConfigMap and automatically:
1. Reloads the cache when changes are detected
2. Triggers reconciliation of all VariantAutoscaling resources
3. Applies the new configuration without requiring pod restart

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

### Caching Architecture

The controller uses an **efficient caching mechanism** with ConfigMap watch for optimal performance:

**Initialization (on controller startup):**
```go
// cmd/main.go
reconciler := &controller.VariantAutoscalingReconciler{...}
reconciler.SetupWithManager(mgr)  // Sets up ConfigMap watch

// Initialize cache on startup
if err := reconciler.InitializeCapacityConfigCache(context.Background()); err != nil {
    setupLog.Warn("Failed to load initial capacity scaling config, will use defaults")
}
```

**Reconciliation (zero API calls):**
```go
// In Reconcile loop - uses cached config (fast, no API call)
capacityConfigs := r.getCapacityConfigFromCache()

// For a specific VariantAutoscaling resource
capacityConfig := r.getCapacityScalingConfigForVariant(
    capacityConfigs,
    va.Spec.ModelID,
    va.Namespace,
)

// Use capacityConfig for capacity-based scaling decisions
if currentKvUtil >= capacityConfig.KvCacheThreshold {
    // Apply capacity scaling logic
}
```

### Automatic Cache Updates

The controller watches the `capacity-scaling-config` ConfigMap for changes:

1. **ConfigMap change detected** → Watch event triggered
2. **Cache automatically reloaded** → New configuration loaded
3. **All VariantAutoscaling resources reconciled** → New config applied immediately

**Log output on ConfigMap change:**
```
INFO  Capacity scaling ConfigMap changed, reloading cache
INFO  Capacity scaling config cache updated entries=3 has_default=true
INFO  Triggering reconciliation for all VariantAutoscaling resources due to ConfigMap change count=5
```

### Performance Characteristics

| Operation | Before (Without Cache) | After (With Cache) |
|-----------|------------------------|-------------------|
| Startup | N/A | Single ConfigMap read |
| Per Reconciliation | ConfigMap API call | Memory read only |
| Config Change | Manual pod restart needed | Automatic reload + reconcile |
| Latency Impact | Network round-trip per reconcile | Zero (memory access) |
| Concurrency | Serial API calls | Thread-safe concurrent reads |

**Cache benefits:**
- ✅ **Single read on startup** instead of per-reconciliation
- ✅ **Zero API calls during reconciliation** (cached access)
- ✅ **Event-driven updates** (immediate response to changes)
- ✅ **Thread-safe concurrent access** (RWMutex)
- ✅ **Defensive copying** prevents external modification

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

### Config Changes Not Taking Effect

**Symptom:** Updated ConfigMap but controller still uses old values

**Solution:** The controller watches for ConfigMap changes and automatically reloads. Check:

1. **Verify ConfigMap was updated:**
   ```bash
   kubectl get cm capacity-scaling-config -n workload-variant-autoscaler-system -o yaml
   ```

2. **Check controller logs for reload confirmation:**
   ```bash
   kubectl logs -n workload-variant-autoscaler-system deployment/wva-controller | grep "Capacity scaling"
   ```

   Expected logs:
   ```
   INFO  Capacity scaling ConfigMap changed, reloading cache
   INFO  Capacity scaling config cache updated entries=2 has_default=true
   INFO  Triggering reconciliation for all VariantAutoscaling resources
   ```

3. **If no logs appear, verify watch is working:**
   - Check controller pod is running: `kubectl get pods -n workload-variant-autoscaler-system`
   - Check for errors: `kubectl logs -n workload-variant-autoscaler-system deployment/wva-controller --tail=100`

4. **Manual restart (last resort):**
   ```bash
   kubectl rollout restart deployment/wva-controller -n workload-variant-autoscaler-system
   ```

### Cache Initialization Failed

**Symptom:** Warning on controller startup
```
WARN Failed to load initial capacity scaling config, will use defaults
```

**Solution:** This is non-fatal. The controller continues with hardcoded defaults. To fix:

1. Deploy the ConfigMap:
   ```bash
   kubectl apply -f deploy/configmap-capacity-scaling.yaml
   ```

2. The watch mechanism will automatically reload the cache once ConfigMap is available

3. Verify cache loaded:
   ```bash
   kubectl logs -n workload-variant-autoscaler-system deployment/wva-controller | grep "Capacity scaling configuration loaded"
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

## Architecture Notes

### Caching Implementation Details

The caching mechanism uses the following components:

**Thread Safety:**
- Uses `sync.RWMutex` for concurrent access control
- Multiple reconciliation loops can read cache simultaneously
- Write operations (cache reload) are exclusive

**Defensive Copy:**
- `getCapacityConfigFromCache()` returns a deep copy
- Prevents external code from modifying cached configuration
- Each caller gets an independent copy

**Watch Mechanism:**
- Kubernetes watch on `capacity-scaling-config` ConfigMap
- Predicate filters to only relevant ConfigMap events
- Event handler reloads cache and triggers reconciliation

**Graceful Degradation:**
- Controller starts successfully even if ConfigMap missing
- Uses hardcoded defaults as fallback
- Automatically loads config once ConfigMap becomes available

## Future Enhancements

Potential future features:
- Integration of threshold values with Inference Scheduler
- Time-based configuration (e.g., aggressive scaling during peak hours)
- Dynamic threshold adjustment based on historical metrics
- Metric-based cache invalidation (detect stale configs)
