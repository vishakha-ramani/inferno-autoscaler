### INSTALL
```
cd ~/projects/workload-variant-autoscaler
export WVA_PROJECT=$PWD
export BASE_NAME="inference-scheduling"
export NAMESPACE="llm-d-$BASE_NAME"
export MONITORING_NAMESPACE="openshift-user-workload-monitoring"
kubectl create namespace $NAMESPACE
kubectl get secret thanos-querier-tls -n openshift-monitoring -o jsonpath='{.data.tls\.crt}' | base64 -d > /tmp/prometheus-ca.crt

cd $WVA_PROJECT
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm upgrade -i prometheus-adapter prometheus-community/prometheus-adapter \
  -n $MONITORING_NAMESPACE \
  -f config/samples/prometheus-adapter-values-ocp.yaml

kubectl create namespace workload-variant-autoscaler-system \
  --dry-run=client -o yaml | \
  kubectl apply -f - \
  && kubectl label ns workload-variant-autoscaler-system \
     app.kubernetes.io/name=workload-variant-autoscaler \
     control-plane=controller-manager --overwrite

cd $WVA_PROJECT/hack
helm install workload-variant-autoscaler ./wva-llmd-infra \
  -n workload-variant-autoscaler-system \
  --set-file prometheus.caCert=/tmp/prometheus-ca.crt \
  --set hfToken=$HF_TOKEN \
  --set variantAutoscaling.accelerator=L40S \
  --set variantAutoscaling.modelID=unsloth/Meta-Llama-3.1-8B \
  --set vllmService.enabled=true \
  --set vllmService.nodePort=30000 \
  --set probes.enabled=true

oc adm policy add-cluster-role-to-user cluster-monitoring-view -z prometheus-adapter -n openshift-user-workload-monitoring

oc scale deployment.apps/prometheus-adapter -n openshift-user-workload-monitoring --replicas=0
oc scale deployment.apps/prometheus-adapter -n openshift-user-workload-monitoring --replicas=2
```

### INSTALL LLM-D
```
cd $WVA_PROJECT
export HF_TOKEN="<your_token_here>"
kubectl create secret generic llm-d-hf-token \
    --from-literal="HF_TOKEN=${HF_TOKEN}" \
    --namespace "${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -

export OWNER="llm-d-incubation"
export PROJECT="llm-d-infra"
export RELEASE="v1.3.1"
git clone -b $RELEASE -- https://github.com/$OWNER/$PROJECT.git $PROJECT

cd $WVA_PROJECT/$PROJECT/quickstart
bash dependencies/install-deps.sh
bash gateway-control-plane-providers/install-gateway-provider-dependencies.sh

yq eval '.releases[].version = "v2.0.3"' -i "gateway-control-plane-providers/kgateway.helmfile.yaml"
helmfile apply -f "gateway-control-plane-providers/kgateway.helmfile.yaml"

export EXAMPLES_DIR="$WVA_PROJECT/$PROJECT/quickstart/examples/$BASE_NAME"
yq eval '.gateway.service.type = "NodePort"' -i $WVA_PROJECT/$PROJECT/charts/llm-d-infra/values.yaml

cd $EXAMPLES_DIR
sed -i '' "s/llm-d-inference-scheduler/$NAMESPACE/g" helmfile.yaml.gotmpl
yq eval '(.. | select(. == "Qwen/Qwen3-0.6B")) = "unsloth/Meta-Llama-3.1-8B" | (.. | select(. == "hf://Qwen/Qwen3-0.6B")) = "hf://unsloth/Meta-Llama-3.1-8B"' -i ms-$BASE_NAME/values.yaml
helmfile apply -e kgateway

kubectl patch gatewayparameters.gateway.kgateway.dev infra-inference-scheduling-inference-gateway \
  -n llm-d-inference-scheduling \
  --type='merge' \
  -p '{"spec":{"kube":{"service":{"type":"NodePort"}}}}'

cd $WVA_PROJECT
```

### CLEANUP
```
helm delete prometheus-adapter -n openshift-user-workload-monitoring
helm delete workload-variant-autoscaler -n workload-variant-autoscaler-system
```