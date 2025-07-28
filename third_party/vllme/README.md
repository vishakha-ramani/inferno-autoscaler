# Running vllm emulator with Inferno

## Container Image

To build the container image running the emulator, run this command:

```sh
$ docker build -t vllme .
```

## Switch context to kind-kind
```bash
kubectl config use-context kind-kind 
```

## Load image in the kind cluster
Use the kind load docker-image command. You'll need to specify the image name and tag. In your case, it's vllme:latest. If you have multiple kind clusters, you'll also need to specify the cluster name using the --name flag.

Assuming your kind cluster is named kind (the default), the command would be:
```bash
kind load docker-image vllme:latest
```


## vllme metrics in Prometheus
We use prometheus operator to monitor the deployments. 

The [official document](https://github.com/prometheus-operator/prometheus-operator/tree/main) says the Prometheus Operator provides [Kubernetes](https://kubernetes.io/) native deployment and management of [Prometheus](https://prometheus.io/) and related monitoring components.
Specifically, when you want to configure prometheus and other monitoring components inside your kubernetes cluster, you use prometheus-operator stack. 

### prometheus-operator and ServiceMonitor
We want an object which will use a Service as a service discovery endpoint to find all the pods of the Deployment. This object is aptly named ServiceMonitor. It is a service to find service. The end point of this service will be used by Prometheus to scrape the desired metrics. 
The [following discussion](https://prometheus-operator.dev/docs/developer/getting-started/) explains how to use `ServiceMonitor` object to monitor targets for a sample application. 
1. Pre-requisites:
	1. A Kubernetes cluster with [admin](https://kubernetes.io/docs/reference/access-authn-authz/rbac/) permissions.
	2. A running Prometheus Operator (install using helm chart)
		1. Create a Kuberenetes namespace `monitoring`.
		2. Get Helm Repository Info 
```sh
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
```
Install Helm Chart:
```sh
helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack -n monitoring
```

Check installation:
`kubectl get pods -n monitoring`

#### A running Prometheus instance
Follow [this guide](https://prometheus-operator.dev/docs/platform/platform-guide/) to deploy Prometheus instance, except that the prometheus.yaml file should be:
```yaml
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: prometheus
spec:
  serviceAccountName: prometheus
  serviceMonitorNamespaceSelector: {}
  serviceMonitorSelector: {}
  podMonitorSelector: {}
  resources:
    requests:
      memory: 400Mi

```

To verify that the instance is up and running, run:
```
kubectl get -n default prometheus prometheus -w
```
A `prometheus-operated` service should also have been created:
```sh
kubectl get services

NAME                  TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)    AGE
prometheus-operated   ClusterIP   None         <none>        9090/TCP   17s

```

Access the server by forwarding a local port to the service:
```sh
kubectl port-forward svc/prometheus-operated 9090:9090
```

The browser address to access prometheus is `http://localhost:9090`.

### Using ServiceMonitor
First we create the following deployment:
```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllme-deployment
  labels:
    inferno.server.managed: "true"
    inferno.server.name: vllm-001
    inferno.server.model: llama_13b
    inferno.server.class: Premium
    inferno.server.allocation.accelerator: MI250
    inferno.server.allocation.maxbatchsize: "8"
    inferno.server.load.rpm: "30.2"
    inferno.server.load.numtokens: "1560"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: vllme
  template:
    metadata:
      labels:
        app: vllme
    spec:
      containers:
      - name: vllme
        image: vllme:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 80
```
Create the associated service:
```
apiVersion: v1
kind: Service
metadata:
  name: vllme-service
  labels:
    app: vllme
spec:
  selector:
    app: vllme  
  ports:
    - name: vllme
      port: 80
      protocol: TCP
      targetPort: 80
      nodePort: 30000
  type: NodePort
```
And now create a ServiceMonitor:
```
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: vllme-servicemonitor
  labels:
    app: vllme
spec:
  selector:
    matchLabels:
      app: vllme
  endpoints:
  - port: vllme
    path: /metrics
    interval: 15s
  namespaceSelector:
    any: true
```

#### Looking at the Prometheus Dashboard:
Go to 'Status' -> 'Targets', and see if the `vllme-servicemonitor` appears. 

Start load generator by running the following script in `vllm_emulator` directory: `./premium-llama-13b-loadgen.sh`.

Go to 'Graphs', and type in the following query: `vllm:requests_count_total`, and see the associated metrics for each pod.

You can query `sum(rate(...))` directly from your terminal by using **`curl`** or any HTTP client to send a query to Prometheus's HTTP API.
```sh
curl -G http://localhost:9090/api/v1/query \
     --data-urlencode 'query=sum(rate(vllm:requests_count_total[1m])) * 60'
```

### Accessing the Grafana dashboard
Run 
```sh
kubectl port-forward svc/kube-prometheus-stack-grafana 3000:80 -n monitoring
```
and access this on address: `http://localhost:3000`, with the following credentials:
- username: `admin`
- password: `prom-operator`

We installed the Kube-Prometheus-Stack using helm. The chart provides a collection of Kubernetes manifests, Grafana dashboards, and Prometheus rules combined with documentation and scripts to provide easy to operate end-to-end Kubernetes cluster monitoring with Prometheus using the Prometheus Operator.
If you install prometheus operator using helm, a Grafana service is already created for you. Additionally, you get a configured added prometheus data source by default in Grafana. 
Check Connections->Data Source. 
The defualt has URL pointing to prometheus instance running in monitoring namespace. 
The prometheus instance scraping our servicemonitors is in default namespace.
So we need to create another Prometheus source pointing to the instance in the default namespace.
Also, make sure for our own purpose that you port-forward prometheus as:
```sh
kubectl port-forward svc/prometheus-operated 9090:9090
```

The URL for prometheus source in default namespace is:
```sh
http://prometheus-operated.default.svc.cluster.local:9090
```


## Sanity checks
First we need to send the client requests. Run
```bash
kubectl port-forward svc/vllme-service 30000:80
```

Then on another terminal, run 
```bash
./loadgen.sh
```

To see if vllme metrics are properly exposed, the first step is to:
```bash
kubectl port-forward svc/vllme-service 8000:80
```
go to `http://localhost:8000/metrics` and check if you see metrics starting with `vllm:`. Refresh to see the values changing with the load generator on.

To see prometheus dashboard:
```bash
kubectl port-forward svc/prometheus-operated 9090:9090
```
Then go to `http://localhost:9090`. First see if under status -> target health, you can see vllme-servicemonitor up.
Then go to Query tab and write `vllm:`. Prometheus should show you all the vllm: metrics exposed by vllme. If not, then there is some problem. 

