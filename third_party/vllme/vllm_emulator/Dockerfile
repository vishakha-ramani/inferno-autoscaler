FROM python:3.10-slim AS base

# Set working directory
WORKDIR /app

# Ensure Python output is sent straight to terminal (no buffering)
ENV PYTHONUNBUFFERED=1
ENV MODEL_NAME=default

# Install only necessary system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        gcc \
    && rm -rf /var/lib/apt/lists/*

# Copy dependency list first for better caching
COPY requirements.txt .

# Install Python dependencies
RUN pip install --no-cache-dir --upgrade -r requirements.txt

# Copy application code
COPY server.py vllm_model.py metrics.py /app/

# Use uvicorn instead of "fastapi run" (faster & production‑ready)
CMD ["uvicorn", "server:app", "--host", "0.0.0.0", "--port", "80"]