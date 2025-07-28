# --------------------------------------------------------------------
# Configuration
# --------------------------------------------------------------------
IMAGE_NAME ?= quay.io/infernoautoscaler/vllme  # vllme registry/image
TAG ?= slim-1.0
DOCKERFILE ?= Dockerfile
CONTEXT ?= .
PORT ?= 80

# Full image tag
IMAGE=$(strip $(IMAGE_NAME)):$(strip $(TAG))

# --------------------------------------------------------------------
# Targets
# --------------------------------------------------------------------

.PHONY: help build push run clean

help: ## Show available targets
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  make %-15s %s\n", $$1, $$2}'

build: ## Build Docker image
	docker build -t $(IMAGE) -f $(DOCKERFILE) $(CONTEXT)

push: build ## Push image to registry
	docker push $(IMAGE)

run: build ## Run locally for testing
	docker run --rm -p $(PORT):80 $(IMAGE)

clean: ## Remove local images
	docker rmi $(IMAGE) $(IMAGE_NAME):latest || true
