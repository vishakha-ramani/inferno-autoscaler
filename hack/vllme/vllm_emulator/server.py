## https://towardsdatascience.com/how-to-build-an-openai-compatible-api-87c8edea2f06

import asyncio
import datetime
import os
import re
import time
from contextlib import asynccontextmanager
from typing import List, Optional

from fastapi import FastAPI
from prometheus_client import make_asgi_app
from pydantic import BaseModel
from pydantic_settings import BaseSettings
from starlette.routing import Mount
from vllm_model import *

############################################  SETUP vLLM Emulator #####################################################


class Settings(BaseSettings):
    # Use the environment variable for the model name
    model: str = os.getenv('MODEL_NAME', 'default')  # Default to 'default' if not set
    # Time for one decode run in ms (inter-token latency) #TODO: Assumed independent of batch size or no of tokens generated
    decode_time: int = DECODE_TIME
    # Model size in MB
    model_size: int = MODEL_SIZE
    # KVCache size for one token in MB
    kvc_per_token: int = KVC_PER_TOKEN
    # average number of tokens in message
    avg_generated_len: int = os.getenv('AVG_TOKENS', 100)
    # distribution of number of tokens per request
    tokens_distribution: str = os.getenv('TOKENS_DISTRIBUTION', "uniform")
    # maximum batch size
    max_batch_size: int = os.getenv('MAX_BATCH_SIZE', 1)


settings = Settings()

clock = Clock(start_time = 0, step_time = settings.decode_time)
model = Model(model_name = settings.model, model_size = settings.model_size, kvcache_per_token = settings.kvc_per_token)

labels=dict(model_name=model)
metrics = Metrics(labelnames=labels) # register metrics

gpu   = Device(device_id = 1, net_memory = M, metrics = metrics, model_name = settings.model, useable_ratio = 0.8)

vllmi = vLLM( device=gpu, clock=clock, model=model, metrics=metrics, max_batch_size=settings.max_batch_size)
load  = Load( settings.avg_generated_len, settings.tokens_distribution)


######################################################################################################################


class ChatMessage(BaseModel):
    role: str
    content: str

class ChatCompletionRequest(BaseModel):
    model: str = "mock-gpt-model"
    messages: List[ChatMessage]
    max_tokens: Optional[int] = 512
    temperature: Optional[float] = 0.1
    stream: Optional[bool] = False



@asynccontextmanager
async def lifespan(app: FastAPI):
  vllmTask = asyncio.create_task(vllmi.run())
  print("Starting vLLM")
  yield
  vllmTask.cancel()
  print("vLLM stopped")

app = FastAPI(title="OpenAI-compatible API", lifespan=lifespan)

@app.post("/v1/chat/completions")
async def chat_completions(request: ChatCompletionRequest):
    input_seq = request.messages[-1].content

    input_len  = len(input_seq)
    output_len = load.get_output_len(input_len)
    now    = datetime.datetime.now()
    req_id = now.strftime("%Y-%m-%dT%H:%M:%S-") + str(random.randint(0,100))

    reqi   = RequestElement(req_id=req_id, input_token_length=input_len, output_token_length=output_len)

    await vllmi.add_new_request_wait(reqi)

    if request.messages and request.messages[0].role == 'user':
      resp_content = f"Request stats: arrival time = {reqi.arrival_time}, completion time = {reqi.completion_time}, ttft = {reqi.ttft_met_time}, input_token_len = {reqi.InputTokenLength}, output_token_len = {reqi.token_len}"
    else:
      resp_content = "Empty message sent!"


    return {
        "id": req_id,
        "object": "chat.completion",
        "created": int(time.time()),
        "model": request.model,
        "choices": [{
            "index" : 0,
            "message": ChatMessage(role="assistant", content=resp_content)
        }],
        "usage": {
            "prompt_tokens": reqi.InputTokenLength,
            "completion_tokens": reqi.token_len,
        }
    }

# Add prometheus asgi middleware to route /metrics requests
metrics_app = make_asgi_app()
route = Mount("/metrics", metrics_app)
route.path_regex = re.compile('^/metrics(?P<path>.*)$') # see https://github.com/prometheus/client_python/issues/1016#issuecomment-2088243791
app.routes.append(route)
