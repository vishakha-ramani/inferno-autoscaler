# Guide to Offline Benchmarking for WVA's Model Analyzer
This guide explains how to collect queueing parameters for WVA's model analyzer using offline benchmarking. We will use `vllm` for model serving and `guidellm` for benchmarking, all deployed on OpenShift.


## 1. Setting Up the Environment
### Step 1.1: Deploying vLLM
1. Follow the instructions at [vllm-samples](vllm-samples.md) to deploy vLLM on OpenShift.

2. To stay away from server saturation, we want to limit the max batch size to a fixed value. In the following, we use $64$ as the max batch size for demonstration purposes.  Such a parameter can be set using `--max-num-seqs` as follows. 
```yaml
spec:
  containers:
    - name: vllm-server
      image: <your-vllm-image>
      args:
        - "vllm serve unsloth/Meta-Llama-3.1-8B --trust-remote-code --download-dir /models-cache --max-num-seqs 64"
```


### Step 1.2: Deploying guidellm
1. Build a guidellm image by following the instructions at [guidellm-sample](guidellm-sample.md).

## 2. Deriving Queueing Parameters
We'll use two separate guidellm jobs to generate the data points needed to solve for the `alpha` and `beta` parameters. `alpha` represents the fixed overhead per request, and `beta` represents the variable overhead per request.
In the WVA's model analyzer, the ITL follows the following linear relationship

$$ITL = \alpha + \beta \times batchsize.$$

### Step 2.1: Run a Synchronous Benchmark Job
Run the following `Job` to perform a synchronous benchmark. This will give you the baseline ITL when a request is processed alone (batch size of 1).
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: synchronous-job
  namespace: vllm-test
spec:
  template:
    spec:
      containers:
      - name: guidellm-benchmark-container
        image: <your-guidellm-image>
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
        - "synchronous"
        - "--max-seconds"
        - "360"
        - "--model"
        - "unsloth/Meta-Llama-3.1-8B"
        - "--data"
        - "prompt_tokens=128,output_tokens=128"
        - "--output-path"
        - "/tmp/benchmarks.json" 
      restartPolicy: Never
  backoffLimit: 4
```

After the job completes, inspect the pod logs to find the benchmark results. 

For example, the expected output from the logs is as follows
```sh
oc logs <synchronous-job-pod-name>
Creating backend...
Backend openai_http connected to http://vllm:8000 for model                     
unsloth/Meta-Llama-3.1-8B.                                                      
Creating request loader...
Created loader with 1000 unique requests from                                   
prompt_tokens=128,output_tokens=128.                                            
                                                                                
                                                                                
╭─ Benchmarks ─────────────────────────────────────────────────────────────────╮
│ [2… syn… (c… Req:    1.1 req/s,    0.91s Lat,     1.0 Conc,      65 Comp,  … │
│              Tok:  140.7 gen/s,  423.4 tot/s,  15.0ms TTFT,    7.0ms ITL,  … │
╰──────────────────────────────────────────────────────────────────────────────╯
Generating... ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ (1/1) [ 0:01:00 < 0:00:00 ]                    
                    
Benchmarks Metadata:
    Run id:bc5e6427-839c-45c6-9e56-90731a52c630
    Duration:60.2 seconds
    Profile:type=synchronous, strategies=['synchronous']
    Args:max_number=None, max_duration=60.0, warmup_number=None,                
    warmup_duration=None, cooldown_number=None, cooldown_duration=None          
    Worker:type_='generative_requests_worker' backend_type='openai_http'        
    backend_target='http://vllm:8000' backend_model='unsloth/Meta-Llama-3.1-8B' 
    backend_info={'max_output_tokens': 16384, 'timeout': 300, 'http2': True,    
    'authorization': False, 'organization': None, 'project': None,              
    'text_completions_path': '/v1/completions', 'chat_completions_path':        
    '/v1/chat/completions'}                                                     
    Request Loader:type_='generative_request_loader'                            
    data='prompt_tokens=128,output_tokens=128' data_args=None                   
    processor='unsloth/Meta-Llama-3.1-8B' processor_args=None                   
    Extras:None
                
                
Benchmarks Info:
================================================================================
====================================================================            
Metadata                                    |||| Requests Made  ||| Prompt      
Tok/Req ||| Output Tok/Req ||| Prompt Tok Total  ||| Output Tok Total  ||       
  Benchmark| Start Time| End Time| Duration (s)|  Comp|  Inc|  Err|  Comp|      
Inc| Err|  Comp|   Inc| Err|   Comp|   Inc|   Err|   Comp|   Inc|   Err         
-----------|-----------|---------|-------------|------|-----|-----|------|------
|----|------|------|----|-------|------|------|-------|------|------            
synchronous|   20:41:22| 20:42:22|         60.0|    65|    1|    0| 129.1|      
128.0| 0.0| 128.0| 123.0| 0.0|   8390|   128|     0|   8320|   123|     0       
================================================================================
====================================================================            
                 
                 
Benchmarks Stats:
================================================================================
============================================================                    
Metadata   | Request Stats         || Out Tok/sec| Tot Tok/sec| Req Latency     
(ms)||| TTFT (ms)       ||| ITL (ms)       ||| TPOT (ms)      ||                
  Benchmark| Per Second| Concurrency|        mean|        mean| mean| median|   
p99| mean| median|  p99| mean| median| p99| mean| median| p99                   
-----------|-----------|------------|------------|------------|-----|-------|---
--|-----|-------|-----|-----|-------|----|-----|-------|----                    
synchronous|       1.10|        1.00|       140.7|       423.4| 0.91|   0.91|   
0.93| 15.0|   14.8| 17.0|  7.0|    7.0| 7.2|  7.0|    7.0| 7.1                  
================================================================================
============================================================                    
                           
Saving benchmarks report...
Benchmarks report saved to /tmp/benchmarks.json
                      
Benchmarking complete.
```

In this example, ITL = 7, thus we have $\alpha + \beta = 7.$

### Step 2.2: Run a Throughput Benchmark Job
Now, run a second Job to measure the ITL at the maximum batch size you configured in the vLLM deployment (batch size is equal to `--max-num-seqs` = 64).
**Ensure that GUIDELLM__MAX_CONCURRENCY matches the max-num-seqs value you set in your vLLM deployment.**

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: throughput-job
  namespace: vllm-test
spec:
  template:
    spec:
      containers:
      - name: guidellm-benchmark-container
        image: <your-guidellm-image>
        imagePullPolicy: IfNotPresent
        env:
        - name: HF_HOME
          value: "/tmp"
        - name: GUIDELLM__MAX_CONCURRENCY
          value: "64"
        command: ["/usr/local/bin/guidellm"]
        args:
        - "benchmark"
        - "--target"
        - "http://vllm:8000"
        - "--rate-type"
        - "throughput"
        - "--max-seconds"
        - "360"
        - "--model"
        - "unsloth/Meta-Llama-3.1-8B"
        - "--data"
        - "prompt_tokens=128,output_tokens=128"
        - "--output-path"
        - "/tmp/benchmarks.json" 
      restartPolicy: Never
  backoffLimit: 4
```

Again, check the pod logs after the job finishes. 
For example, the expected logs for the throughput job are as follows:
```sh
oc logs <throughput-job-pod-name>
Creating backend...
Backend openai_http connected to http://vllm:8000 for model                     
unsloth/Meta-Llama-3.1-8B.                                                      
Creating request loader...
Created loader with 1000 unique requests from                                   
prompt_tokens=128,output_tokens=128.                                            
                                                                                
                                                                                
╭─ Benchmarks ─────────────────────────────────────────────────────────────────╮
│ [2… th… (c… Req:   56.7 req/s,    1.13s Lat,    63.9 Conc,   20353 Comp,   … │
│             Tok: 7251.6 gen/s, 21819.9 tot/s,  26.0ms TTFT,    8.7ms ITL,  … │
╰──────────────────────────────────────────────────────────────────────────────╯
Generating... ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ (1/1) [ 0:06:00 < 0:00:00 ]                    
                    
Benchmarks Metadata:
    Run id:921345ac-fe8b-4a02-afd6-4ea6c2775a4b
    Duration:361.1 seconds
    Profile:type=throughput, strategies=['throughput'], max_concurrency=None
    Args:max_number=None, max_duration=360.0, warmup_number=None,               
    warmup_duration=None, cooldown_number=None, cooldown_duration=None          
    Worker:type_='generative_requests_worker' backend_type='openai_http'        
    backend_target='http://vllm:8000' backend_model='unsloth/Meta-Llama-3.1-8B' 
    backend_info={'max_output_tokens': 16384, 'timeout': 300, 'http2': True,    
    'authorization': False, 'organization': None, 'project': None,              
    'text_completions_path': '/v1/completions', 'chat_completions_path':        
    '/v1/chat/completions'}                                                     
    Request Loader:type_='generative_request_loader'                            
    data='prompt_tokens=128,output_tokens=128' data_args=None                   
    processor='unsloth/Meta-Llama-3.1-8B' processor_args=None                   
    Extras:None
                
                
Benchmarks Info:
================================================================================
=============================================================                   
Metadata                                   |||| Requests Made||| Prompt Tok/Req 
||| Output Tok/Req ||| Prompt Tok Total||| Output Tok Total||                   
 Benchmark| Start Time| End Time| Duration (s)|  Comp| Inc| Err|  Comp|   Inc|  
Err|  Comp|   Inc| Err|    Comp|  Inc| Err|    Comp|  Inc| Err                  
----------|-----------|---------|-------------|------|----|----|------|------|--
--|------|------|----|--------|-----|----|--------|-----|----                   
throughput|   20:09:27| 20:15:27|        360.0| 20353|  64|   0| 129.1| 128.0|  
0.0| 128.0| 100.1| 0.0| 2627045| 8192|   0| 2605184| 6406|   0                  
================================================================================
=============================================================                   
                 
                 
Benchmarks Stats:
================================================================================
===========================================================                     
Metadata  | Request Stats         || Out Tok/sec| Tot Tok/sec| Req Latency      
(ms)||| TTFT (ms)       ||| ITL (ms)       ||| TPOT (ms)      ||                
 Benchmark| Per Second| Concurrency|        mean|        mean| mean| median|    
p99| mean| median|  p99| mean| median| p99| mean| median| p99                   
----------|-----------|------------|------------|------------|-----|-------|----
-|-----|-------|-----|-----|-------|----|-----|-------|----                     
throughput|      56.65|       63.94|      7251.6|     21819.9| 1.13|   1.12|    
1.19| 26.0|   25.5| 35.4|  8.7|    8.6| 8.8|  8.6|    8.6| 8.7                  
================================================================================
===========================================================                     
                           
Saving benchmarks report...
Benchmarks report saved to /tmp/benchmarks.json
                      
Benchmarking complete.
```

In this example, ITL = 8.7, thus we have $\alpha + (\beta \times 64) = 8.7.$

## 3. Solving for alpha and beta
You now have a system of two linear equations with two unknowns, `alpha` and `beta`:
1. $ITL_\text{synchronous} = \alpha + \beta$
2. $ITL_\text{throughput} = \alpha + \beta \times batchsize$

Using the values you obtained from the benchmark job logs for $ITL_\text{synchronous}$ and $ITL_\text{throughput}$, you can solve for `alpha` and `beta`:

$$\beta = \frac{ITL_\text{throughput} - ITL_\text{synchronous}}{batchsize - 1},$$
$$\alpha=ITL_\text{synchronous} - \beta$$


In our example, we obtain $\alpha \approx 6.973$ and $\beta \approx 0.027$.
