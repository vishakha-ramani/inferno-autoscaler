# --------------------------------------------------------------------
# Configuration
# --------------------------------------------------------------------
IMAGE_NAME ?= quay.io/infernoautoscaler/vllme  # vllme registry/image
TAG ?= 0.2.3-multi-arch
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


# TODO: this target is temporary and will be removed
# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
BUILDER_NAME ?= vllme-builder

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- docker buildx create --name vllme-builder
	docker buildx use vllme-builder
	- docker buildx build --push --platform=$(PLATFORMS) --tag ${IMAGE} -f Dockerfile.cross .
	- docker buildx rm vllme-builder
	rm Dockerfile.cross
