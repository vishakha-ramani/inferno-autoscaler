## Setting up GuideLLM load generator

We use `guidellm` as the load generator and we run our load generator from a different pod within the same OpenShift cluster. 

### Step 1 : Create an image
First create a `Dockerfile`
```dockerfile
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
CMD ["tail", "-f", "/dev/null"]
```

Then create a `requirements.txt` with the following contents in the same directory as your `Dockerfile`
```Plaintext
guidellm
```

Build the image for the correct target CPU architecture. 
You can get the architecture of the OpenShift node by `oc get nodes -o custom-columns=NAME:.metadata.name,ARCH:.status.nodeInfo.architecture`.
In our case, it was `linux/amd64`
```sh
docker build --platform linux/amd64 -t <image-repo>:<tag> .
```

Push the image
```sh
docker push <image-repo>:<tag>
```

Make the image **public**.


### Create Job
Create a  load generator job (`oc apply -f guidellm-job.yaml`) based on the following template using the image created in step 1.
```yaml
# guidellm-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: guidellm-job
  namespace: vllm-test
spec:
  template:
    spec:
      containers:
      - name: guidellm-benchmark-container
        image: <image-repo>:<tag>
        imagePullPolicy: IfNotPresent
        env:
        - name: HF_HOME
          value: "/tmp"
        command: ["/usr/local/bin/guidellm"]
        args:
        - "benchmark"
        - "--target"
        - "http://vllm:8000"
        - "--rate-type"
        - "constant"
        - "--rate"
        - "<rate>"
        - "--max-seconds"
        - "<max-seconds>"
        - "--model"
        - "unsloth/Meta-Llama-3.1-8B"
        - "--data"
        - "prompt_tokens=128,output_tokens=512"
        - "--output-path"
        - "/tmp/benchmarks.json" 
      restartPolicy: Never
  backoffLimit: 4
```