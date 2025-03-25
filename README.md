# Inference system optimizer

The inference system optimizer assigns GPU types to inference model servers and decides on the number of replicas for each model for a given request traffic load and classes of service, as well as the batch size. ([slides](docs/slides/inferno-dynamic.pdf))

## Building

Since a couple of module dependencies are hosted in intranet git repositories, and in order to avoid sharing ssh keys in the build VM, it is recommended to run `go mod vendor` before building the image. The resulting vendor directory may be deleted once the build step is completed.

```bash
docker build -t  inferno . --load
```

## Running

![inferno-service](docs/slides/inferno-service.png)

- Create or have access to a cluster.
- Clone this repository and set environment variable `INFERNO_REPO` to the path to it.
- Create deployments representing inference servers in namespace *infer*.

    ```bash
    cd $INFERNO_REPO/services/yamls
    kubectl apply -f ns.yaml
    kubectl apply -f dep1.yaml,dep2.yaml,dep3.yaml
    ```

    Note that the deployment should have the following labels set (a missing service class name defaults to *Free*)

    ```bash
    labels:
        inferno.server.managed: "true"
        inferno.server.name: vllm-001
        inferno.server.model: llama_13b
        inferno.server.class: Premium
        inferno.server.allocation.accelerator: MI250
    ```

    and some optional labels (if metrics are not available from  Pometheus).

    ```bash
    labels:
        inferno.server.allocation.maxbatchsize: "8"
        inferno.server.load.rpm: "30"
        inferno.server.load.numtokens: "2048"
    ```

- Create namespace *inferno*, where all optimizer components will reside.

    ```bash
    cd $INFERNO_REPO/manifests/yamls
    kubectl apply -f ns.yaml
    ```

- Create a configmap populated with inferno static data, e.g. samples taken from the *large* directory.

    ```bash
    INFERNO_DATA_PATH=$INFERNO_REPO/samples/large
    kubectl create configmap inferno-static-data -n inferno \
    --from-file=/$INFERNO_DATA_PATH/accelerator-data.json \
    --from-file=/$INFERNO_DATA_PATH/model-data.json \
    --from-file=/$INFERNO_DATA_PATH/serviceclass-data.json \
    --from-file=/$INFERNO_DATA_PATH/optimizer-data.json
    ```

- Create a configmap populated with inferno dynamic data (count of accelerator types).

    ```bash
    kubectl create configmap inferno-dynamic-data -n inferno --from-file=/$INFERNO_DATA_PATH/capacity-data.json 
    ```

- Deploy inferno in the cluster.

    ```bash
    kubectl apply -f sa.yaml
    kubectl apply -f deploy.yaml
    ```

- Get the inferno pod name.

    ```bash
    POD=$(kubectl get pod -l app=inferno -n inferno -o jsonpath="{.items[0].metadata.name}")
    ```

- Inspect logs.

    ```bash
    kubectl logs -f $POD -n inferno -c controller
    kubectl logs -f $POD -n inferno -c collector
    kubectl logs -f $POD -n inferno -c optimizer
    kubectl logs -f $POD -n inferno -c actuator
    ```

- (Optional) Start a load emulator to inference servers.

    ```bash
    kubectl apply -f load-emulator.yaml
    kubectl logs -f load-emulator -n inferno
    ```

- Invoke an inferno control loop.

    ```bash
    kubectl port-forward deployment/inferno -n inferno 8080:3300
    curl http://localhost:8080/invoke
    ```

- Cleanup

    ```bash
    cd $INFERNO_REPO/manifests/yamls
    kubectl delete -f load-emulator.yaml
    kubectl delete -f deploy.yaml 
    kubectl delete -f sa.yaml
    kubectl delete configmap inferno-static-data inferno-dynamic-data -n inferno
    kubectl delete -f ns.yaml

    cd $INFERNO_REPO/services/yamls
    kubectl delete -f dep1.yaml,dep2.yaml,dep3.yaml
    kubectl delete -f ns.yaml
    ```

## Description

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
