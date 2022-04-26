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

REGISTRY ?= "amaas-eos-mw1.cec.lab.emc.com:5017"

# Default Sidecar image name
SIDECAR_IMAGE_NAME ?= dell-csi-replicator
SIDECAR_M_IMAGE_NAME ?= dell-csi-migrator
# Default Common controller image name
CONTROLLER_IMAGE_NAME ?= dell-replication-controller

SIDECAR_IMAGE_M_TAG ?= "$(REGISTRY)/$(SIDECAR_M_IMAGE_NAME):$(VERSION)"
SIDECAR_IMAGE_TAG ?= "$(REGISTRY)/$(SIDECAR_IMAGE_NAME):$(VERSION)"
CONTROLLER_IMAGE_TAG ?= "$(REGISTRY)/$(CONTROLLER_IMAGE_NAME):$(VERSION)"

sidecar:
	$(CONTAINER_TOOL) build . -t ${SIDECAR_IMAGE_TAG} -f Dockerfiles/Dockerfile --target sidecar ${NOCACHE_ARG}

migrator-sidecar:
	$(CONTAINER_TOOL) build . -t ${SIDECAR_IMAGE_M_TAG} -f Dockerfiles/Dockerfile --target migrator ${NOCACHE_ARG}

sidecar-migrator-push:
	$(CONTAINER_TOOL) push ${SIDECAR_IMAGE_M_TAG}

sidecar-push:
	$(CONTAINER_TOOL) push ${SIDECAR_IMAGE_TAG}

controller:
	$(CONTAINER_TOOL) build . -t ${CONTROLLER_IMAGE_TAG} -f Dockerfiles/Dockerfile --target controller ${NOCACHE_ARG}

controller-push:
	$(CONTAINER_TOOL) push ${CONTROLLER_IMAGE_TAG}

images: sidecar controller sidecar-migrator
images-push: sidecar-push controller-push sidecar-migrator-push

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy-controller:
	kubectl create configmap dell-replication-controller-config --namespace dell-replication-controller --from-file deploy/config.yaml -o yaml --dry-run | kubectl apply -f -
	cd config/manager && kustomize edit set image controller=${CONTROLLER_IMAGE_TAG}
	kustomize build config/default | kubectl apply -f -

# Build controller image in dev environment with Golang
controller-dev:
	$(CONTAINER_TOOL) build . -t ${CONTROLLER_IMAGE_TAG} -f Dockerfiles/Dockerfile.dev --target controller

# Build sidecar image in dev environment with Golang
sidecar-dev:
	$(CONTAINER_TOOL) build . -t ${SIDECAR_IMAGE_TAG} -f Dockerfiles/Dockerfile.dev --target sidecar

migrator-sidecar-dev:
	$(CONTAINER_TOOL) build . -t ${SIDECAR_IMAGE_M_TAG} -f Dockerfiles/Dockerfile.dev --target sidecar
