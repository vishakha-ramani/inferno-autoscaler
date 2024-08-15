# Inference system optimizer

The inference system optimizer assigns GPU types to inference model servers and decides on the number of replicas for each model for a given request traffic load and classes of service, as well as the batch size. ([slides](docs/slides/inferno-dynamic.pdf))

![problem-scope](docs/figs/Slide5.png)

![timing-definitions](docs/figs/Slide30.png)

![request-batching](docs/figs/Slide6.png)

![token-time-fitting](docs/figs/Slide7.png)

![modeling-batching](docs/figs/Slide9.png)

![qn-model](docs/figs/Slide8.png)

![system-occupancy](docs/figs/Slide32.png)

![impact-batch](docs/figs/Slide33.png)

![target-service](docs/figs/Slide34.png)

Decision variables

For each pair of (class of service, model):

- gpuProfile: the GPU type allocated
- numReplicas: the number of replicas
- batchSize: the batch size, given continuous batching

## Specifications: Accelerators and models

![accelerators](docs/figs/Slide13.png)

![models](docs/figs/Slide14.png)

## Example 1: Unlimited accelerators

![unlimited-assign](docs/figs/Slide16.png)

![unlimited-perf](docs/figs/Slide17.png)

## Example 2: Load change - Unlimited accelerators

![unlimited-change-assign](docs/figs/Slide19.png)

![unlimited-change](docs/figs/Slide20.png)

![unlimited-change-perf](docs/figs/Slide21.png)

## Example 3: Limited accelerators

![limited-count](docs/figs/Slide22.png)

![limited-assign](docs/figs/Slide23.png)

![limited-perf](docs/figs/Slide24.png)

## Example 4: Load change - Limited accelerators

![limited-change-assign](docs/figs/Slide26.png)

![limited-change](docs/figs/Slide27.png)

![limited-change-perf](docs/figs/Slide28.png)
