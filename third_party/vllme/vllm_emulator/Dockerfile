FROM python:3.10
WORKDIR /app

# Set the default model name environment variable
ENV MODEL_NAME=default

COPY requirements.txt server.py vllm_model.py metrics.py /app/

RUN pip install --no-cache-dir --upgrade -r requirements.txt

CMD ["fastapi", "run", "server.py", "--port", "80"]
