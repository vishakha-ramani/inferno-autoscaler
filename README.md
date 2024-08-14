# Inference system optimizer

The inference system optimizer assigns GPU types to inference model servers and decides on the number of replicas for each model for a given request traffic load and classes of service, as well as the batch size. ([slides](docs/slides/inferno-dynamic.pdf))

![description](docs/figs/problem-scope.png)

![description](docs/figs/timing-definitions.png)

![description](docs/figs/request-batching.png)

![description](docs/figs/token-time-fitting.png)

![description](docs/figs/modeling-batching.png)

![description](docs/figs/qn-model.png)

![description](docs/figs/system-occupancy.png)

![description](docs/figs/impact-batch.png)

![description](docs/figs/target-waiting.png)

![description](docs/figs/target-service.png)

Decision variables

For each pair of (class of service, model):

- gpuProfile: the GPU type allocated
- numReplicas: the number of replicas
- batchSize: the batch size, given continuous batching

## Example 1: Unlimited accelerators

![unlimited](docs/cases/unlimited/assignment.png)

![unlimited](docs/cases/unlimited/cost.png)

![unlimited](docs/cases/unlimited/figs.png)

## Example 2: Limited accelerators

![limited](docs/cases/limited/assignment.png)

![limited](docs/cases/limited/cost.png)

![limited](docs/cases/limited/figs.png)

## Example 3: Load change - Unlimited accelerators

![change](docs/cases/change/assignment.png)

![change](docs/cases/change/cost.png)

![change](docs/cases/change/figs.png)
