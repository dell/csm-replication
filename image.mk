include semver.mk

# Figure out if podman or docker should be used (use podman if found)
ifneq (, $(shell which podman 2>/dev/null))
        CONTAINER_TOOL ?= podman
else
        CONTAINER_TOOL ?= docker
endif

# Options for '--no-cache'
NOCACHE ?= false
ifeq ($(NOCACHE), true)
NOCACHE_ARG = --no-cache
else
NOCACHE_ARG =
endif

# Version of the sidecar & common controller. e.g. - v1.0.0.001
VERSION ?="v$(MAJOR).$(MINOR).$(PATCH).$(BUILD)"

REGISTRY ?= "localhost:5000"

# Default Sidecar image name
SIDECAR_IMAGE_NAME ?= dell-csi-replicator
SIDECAR_M_IMAGE_NAME ?= dell-csi-migrator
SIDECAR_NR_IMAGE_NAME ?= dell-csi-node-rescanner
# Default Common controller image name
CONTROLLER_IMAGE_NAME ?= dell-replication-controller

SIDECAR_IMAGE_NR_TAG ?= "$(REGISTRY)/$(SIDECAR_NR_IMAGE_NAME):$(VERSION)"
SIDECAR_IMAGE_M_TAG ?= "$(REGISTRY)/$(SIDECAR_M_IMAGE_NAME):$(VERSION)"
SIDECAR_IMAGE_TAG ?= "$(REGISTRY)/$(SIDECAR_IMAGE_NAME):$(VERSION)"
CONTROLLER_IMAGE_TAG ?= "$(REGISTRY)/$(CONTROLLER_IMAGE_NAME):$(VERSION)"

sidecar: download-csm-common
	$(eval include csm-common.mk)
	$(CONTAINER_TOOL) build --pull . -t ${SIDECAR_IMAGE_TAG} -f Dockerfiles/Dockerfile --target sidecar --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE) ${NOCACHE_ARG}

sidecar-push:
	$(CONTAINER_TOOL) push ${SIDECAR_IMAGE_TAG}

sidecar-node-rescanner: download-csm-common
	$(eval include csm-common.mk)
	$(CONTAINER_TOOL) build --pull . -t ${SIDECAR_IMAGE_NR_TAG} -f Dockerfiles/Dockerfile --target node-rescanner --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE) ${NOCACHE_ARG}

sidecar-node-rescanner-push:
	$(CONTAINER_TOOL) push ${SIDECAR_IMAGE_NR_TAG}

sidecar-migrator: download-csm-common
	$(eval include csm-common.mk)
	$(CONTAINER_TOOL) build --pull . -t ${SIDECAR_IMAGE_M_TAG} -f Dockerfiles/Dockerfile --target migrator --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE) ${NOCACHE_ARG}

sidecar-migrator-push:
	$(CONTAINER_TOOL) push ${SIDECAR_IMAGE_M_TAG}

controller: download-csm-common
	$(eval include csm-common.mk)
	$(CONTAINER_TOOL) build --pull . -t ${CONTROLLER_IMAGE_TAG} -f Dockerfiles/Dockerfile --target controller --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE) ${NOCACHE_ARG}

controller-push:
	$(CONTAINER_TOOL) push ${CONTROLLER_IMAGE_TAG}

images: sidecar controller sidecar-migrator sidecar-node-rescanner
images-push: sidecar-push controller-push sidecar-migrator-push sidecar-node-rescanner-push

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy-controller:
	kubectl create configmap dell-replication-controller-config --namespace dell-replication-controller --from-file deploy/config.yaml -o yaml --dry-run | kubectl apply -f -
	cd config/manager && kustomize edit set image controller=${CONTROLLER_IMAGE_TAG}
	kustomize build config/default | kubectl apply -f -

# Build controller image in dev environment with Golang
controller-dev: download-csm-common
	$(eval include csm-common.mk)
	$(CONTAINER_TOOL) build --pull . -t ${CONTROLLER_IMAGE_TAG} -f Dockerfiles/Dockerfile.dev --target controller --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE)

# Build sidecar image in dev environment with Golang
sidecar-dev: download-csm-common
	$(eval include csm-common.mk)
	$(CONTAINER_TOOL) build --pull . -t ${SIDECAR_IMAGE_TAG} -f Dockerfiles/Dockerfile.dev --target sidecar --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE)

sidecar-migrator-dev: download-csm-common
	$(eval include csm-common.mk)
	$(CONTAINER_TOOL) build --pull . -t ${SIDECAR_IMAGE_M_TAG} -f Dockerfiles/Dockerfile.dev --target migrator --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE)

sidecar-node-rescanner-dev: download-csm-common
	$(eval include csm-common.mk)
	$(CONTAINER_TOOL) build --pull . -t ${SIDECAR_IMAGE_NR_TAG} -f Dockerfiles/Dockerfile.dev --target node-rescanner --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE)

download-csm-common:
	curl -O -L https://raw.githubusercontent.com/dell/csm/main/config/csm-common.mk