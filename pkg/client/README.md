# A Controller for the Optimizer

The [Controller](controller.go) is the main client user of the [REST API Server](../main.go) to the Optimizer.
It performs a periodic optimization loop, consisting of (1) updating server dynamic data, (2) calling the Optimizer, and (3) orchestrating the resulting decisions.
It relies on a [Collector](collector.go) and [Orchestrator](orchestrator.go) to accomplish the following functions.

- Keep static data about accelerators, models, and service classes.
- Update dynamic data about servers through a Collector.
- Call the Optimizer to get servers desired state.
- Implement desired state through an Orchestrator.

The current implementation is very basic. Dynamic server data is generated randomly, and decisions are implemented by simply updating the state. Updating the Collector to get data from Prometheus or directly from vLLM, as well as implementing a real Orchestrator are planned.
