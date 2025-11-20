# WVA Architecture Assumptions and Limitations

## Overview

WVA is currently designed and optimized for **dense transformer architectures** (e.g., Llama, GPT variants) when using the predictive scaling. WVA supports two scaling approaches:

1. **Predictive Scaling** (optional): Uses performance parameters (α, β, γ, δ) to predict future performance and proactively scale. Requires accurate performance parameters (see [Parameter Estimation Guide](../tutorials/parameter-estimation.md)) and assumes linear batch size scaling, which works well for dense transformers but may not accurately predict behavior for other architectures.

2. **Saturation-Based Scaling** (default for v0.4): Reactively scales when observed arrival rate exceeds current capacity. Architecture-agnostic and doesn't require performance parameters.

**Important:** Just because vLLM can execute a model doesn't mean WVA can accurately predict its performance. vLLM handles execution; WVA needs to model performance characteristics. The default **saturation-based scaling** approach works with both dense transformers and non-standard architectures.

### Saturation-Based Scaling (Default)

**Default behavior:** Saturation-based scaling is the default scaling approach for v0.4 release.

**When to use:**
- Default for all architectures (works with dense transformers and non-standard architectures)
- Recommended for HSSM, MoE, or other non-standard architectures
- When performance parameters are uncertain or unavailable
- When predictive scaling shows inaccuracies

**Benefits:**
- Doesn't require performance parameters (α, β, γ, δ)
- Scales reactively based on observed metrics (resource utilization and queue length vs. capacity)
- Less sensitive to architecture-specific behavior differences

**Limitations:**
- Reactive only (scales after saturation, not proactively)
- May be less cost-efficient than accurate predictive scaling
- May scale incrementally (+1 per cycle) rather than calculating exact replica needs, potentially taking multiple cycles to reach optimal capacity

**How to use:**
- WVA will detect saturation based on utilization

## Supported Architectures

### Dense Transformers (Well-Supported)

**Examples:** Llama, GPT, standard transformer models

**Status:**
- Well-tested and validated
- Performance model assumptions hold
- Proper parameter estimation required

### Hybrid State Space Models (HSSM) (Use with Caution)

**Examples:** IBM Granite 3.3-8B-Instruct (if using HSSM architecture)

**Limitations:**
- Different computation patterns (state space dynamics vs. attention)
- May not follow linear batch size scaling
- Performance predictions may be inaccurate

### Mixture of Experts (MoE) (Use with Caution)

**Examples:** Mixtral-8x7B, DeepSeek MoE

**Limitations:**
- Sparse activation and dynamic expert routing
- Batch efficiency varies with routing patterns
- Linear scaling assumptions may not hold

### Other Optimized Architectures (Use with Caution)

**Examples:** DeepSeek variants, custom architectures

**Limitations:**
- May use non-standard computation patterns
- Performance characteristics less well-documented
- May not follow established transformer scaling laws

## What You Need to Know

### Testing with Non-Standard Architectures

**Required Actions:**
1. **Benchmark carefully** - Performance parameters must be estimated per model-accelerator combination
2. **Monitor actual performance** - Compare WVA's predictions with observed behavior
3. **Validate scaling decisions** - Watch for SLO violations that indicate model mismatch

### Best Practices

**For Dense Transformers:**
- Use predictive scaling with proper performance parameters
- Follow the [parameter estimation guide](../tutorials/parameter-estimation.md)

**For HSSM/MoE/Other Architectures:**
- Consider saturation-based scaling as the primary approach
- Perform extensive benchmarking if using predictive scaling
- Monitor actual performance closely
- Report findings to help improve WVA

## Summary

| Architecture Type | Support Level | Recommendation |
|------------------|---------------|----------------|
| Dense Transformers | Well-supported | Use predictive scaling with proper parameters |
| HSSM | Limited | Prefer saturation-based scaling; benchmark if using predictive |
| MoE | Limited | Prefer saturation-based scaling; benchmark if using predictive |
| Custom/Optimized | Limited | Prefer saturation-based scaling; extensive benchmarking required |

**Key Points:**
- WVA's performance model assumes dense transformer behavior
- vLLM execution capability ≠ WVA performance prediction capability
- Saturation-based scaling is default for all architectures to support non-standard architectures
- Monitor actual performance regardless of approach used

## References

- [Modeling and Optimization Design Doc](modeling-optimization.md) - Detailed technical background
- [Parameter Estimation Guide](../tutorials/parameter-estimation.md) - How to benchmark your model
