# Inference system optimizer

The inference system optimizer assigns GPU types to inference model servers and decides on the number of replicas for each model for a given request traffic load and classes of service, as well as the batch size.

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
