# API Reference

## Packages
- [llmd.ai/v1alpha1](#llmdaiv1alpha1)


## llmd.ai/v1alpha1

Package v1alpha1 contains API Schema definitions for the llmd v1alpha1 API group.

### Resource Types
- [VariantAutoscaling](#variantautoscaling)
- [VariantAutoscalingList](#variantautoscalinglist)



#### AcceleratorProfile







_Appears in:_
- [ModelProfile](#modelprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `acc` _string_ |  |  | MinLength: 1 <br /> |
| `accCount` _integer_ |  |  | Minimum: 1 <br /> |
| `alpha` _string_ |  |  | Pattern: `^\d+(\.\d+)?$` <br /> |
| `beta` _string_ |  |  | Pattern: `^\d+(\.\d+)?$` <br /> |
| `maxBatchSize` _integer_ |  |  | Minimum: 1 <br /> |
| `atTokens` _integer_ |  |  | Minimum: 1 <br /> |


#### ActuationStatus







_Appears in:_
- [VariantAutoscalingStatus](#variantautoscalingstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `applied` _boolean_ |  |  |  |
| `lastAttemptTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ |  |  |  |
| `lastSuccessTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ |  |  |  |


#### Allocation







_Appears in:_
- [VariantAutoscalingStatus](#variantautoscalingstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accelerator` _string_ |  |  | MinLength: 1 <br /> |
| `numReplicas` _integer_ |  |  | Minimum: 0 <br /> |
| `maxBatch` _integer_ |  |  | Minimum: 0 <br /> |
| `variantCost` _string_ |  |  | Pattern: `^\d+(\.\d+)?$` <br /> |
| `itlAverage` _string_ |  |  | Pattern: `^\d+(\.\d+)?$` <br /> |
| `waitAverage` _string_ |  |  | Pattern: `^\d+(\.\d+)?$` <br /> |
| `load` _[LoadProfile](#loadprofile)_ |  |  |  |


#### ConfigMapKeyRef







_Appears in:_
- [VariantAutoscalingSpec](#variantautoscalingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  | MinLength: 1 <br /> |
| `key` _string_ |  |  | MinLength: 1 <br /> |


#### LoadProfile







_Appears in:_
- [Allocation](#allocation)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `arrivalRate` _string_ |  |  |  |
| `avgLength` _string_ |  |  |  |


#### ModelProfile







_Appears in:_
- [VariantAutoscalingSpec](#variantautoscalingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accelerators` _[AcceleratorProfile](#acceleratorprofile) array_ |  |  | MinItems: 1 <br /> |


#### OptimizedAlloc







_Appears in:_
- [VariantAutoscalingStatus](#variantautoscalingstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `lastRunTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ |  |  |  |
| `accelerator` _string_ |  |  | MinLength: 2 <br /> |
| `numReplicas` _integer_ |  |  | Minimum: 0 <br /> |


#### VariantAutoscaling







_Appears in:_
- [VariantAutoscalingList](#variantautoscalinglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `llmd.ai/v1alpha1` | | |
| `kind` _string_ | `VariantAutoscaling` | | |
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[VariantAutoscalingSpec](#variantautoscalingspec)_ |  |  |  |
| `status` _[VariantAutoscalingStatus](#variantautoscalingstatus)_ |  |  |  |


#### VariantAutoscalingList









| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `llmd.ai/v1alpha1` | | |
| `kind` _string_ | `VariantAutoscalingList` | | |
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[VariantAutoscaling](#variantautoscaling) array_ |  |  |  |


#### VariantAutoscalingSpec







_Appears in:_
- [VariantAutoscaling](#variantautoscaling)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `modelID` _string_ |  |  | MinLength: 1 <br /> |
| `sloClassRef` _[ConfigMapKeyRef](#configmapkeyref)_ |  |  |  |
| `modelProfile` _[ModelProfile](#modelprofile)_ |  |  |  |


#### VariantAutoscalingStatus







_Appears in:_
- [VariantAutoscaling](#variantautoscaling)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `currentAlloc` _[Allocation](#allocation)_ |  |  |  |
| `desiredOptimizedAlloc` _[OptimizedAlloc](#optimizedalloc)_ |  |  |  |
| `actuation` _[ActuationStatus](#actuationstatus)_ |  |  |  |


