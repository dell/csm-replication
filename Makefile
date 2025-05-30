# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Take tools from GOBIN
CONTROLLER_GEN ?= $(GOBIN)/controller-gen
KUSTOMIZE ?= $(GOBIN)/kustomize

IMG ?= "NOIMG"

#Generate semver.mk for setting the semantic version
gen-semver:
	(cd core; rm -f core_generated.go; go generate)
	go run core/semver/semver.go -f mk > semver.mk

# Run all _test.go files (including those in repctl/) in this repo and generate coverage report
test: generate fmt vet static-crd gen-semver
	go test ./... -coverprofile cover.out

# Build manager binary for csi-replicator
build-sidecar-manager: pre
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/dell-csi-replicator cmd/csi-replicator/main.go
build-sidecar-migrator: pre
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/dell-csi-migrator cmd/csi-migrator/main.go
# Build manager binary for csi-node re scanner
build-sidecar-node-rescanner: pre
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/dell-csi-node-rescanner cmd/csi-node-rescanner/main.go
# Build manager binary for replication-controller
build-controller-manager: pre
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/dell-replication-controller cmd/replication-controller/main.go
# Build all binaries for replication
build: build-sidecar-manager build-sidecar-migrator build-sidecar-node-rescanner build-controller-manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run-sidecar: pre static-crd
	go run cmd/csi-replicator/main.go

run-controller: pre static-crd
	go run cmd/replication-controller/main.go

run-migrator: pre static-crd
	go run cmd/csi-migrator/main.go

run-node-rescanner: pre static-crd
	go run cmd/csi-node-rescanner/main.go

static-crd: manifests
	$(KUSTOMIZE) build config/crd > deploy/replicationcrds.all.yaml


update-image:
ifeq ($(IMG),"NOIMG")
	@{ \
        echo "***" ;\
        echo "Controller image would be set to the default value - controller:latest" ;\
        echo "***" ;\
	}
else
	@{ \
        echo $IMG ;\
        cd config/install ;\
        $(KUSTOMIZE) edit set image controller=$(IMG) ;\
        }
endif


static-controller: manifests controller-rbac update-image
	$(KUSTOMIZE) build config/install > deploy/controller.yaml

# Install CRDs into a cluster
install: static-crd
	kubectl apply -f deploy/replicationcrds.all.yaml

# Uninstall CRDs from a cluster
uninstall: manifests
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy-controller: manifests
	make -f image.mk deploy-controller

# Generate CRD manifests
manifests: tools
	$(CONTROLLER_GEN) paths="./..." crd output:crd:artifacts:config=config/crd/bases

controller-rbac: tools
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./cmd/replication-controller" paths="./controllers/replication-controller"

sidecar-rbac: tools
	$(CONTROLLER_GEN) rbac:roleName=sidecar-manager-role paths="./controllers/csi-replicator" paths="./cmd/csi-replicator" output:rbac:artifacts:config=config/sidecar-rbac

# Run go fmt against code
fmt: gen-semver
	go fmt ./...

# Run go vet against code
vet: gen-semver
	go vet ./...

# Install Go tools to build the code
tools: controller-gen kustomize

# Generate code
generate: tools
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt",year=$(shell date "+%Y") paths="./api/..."

# Pre-requisite for the build/run targets
pre: gen-semver fmt vet tools generate

# Build the container image
image-sidecar: gen-semver
	make -f image.mk sidecar

image-migrator: gen-semver
	make -f image.mk sidecar-migrator

image-migrator-push: gen-semver
	make -f image.mk sidecar-migrator-push

image-node-rescanner: gen-semver
	make -f image.mk sidecar-node-rescanner

image-node-rescanner-push: gen-semver
	make -f image.mk sidecar-node-rescanner-push

image-controller: gen-semver
	make -f image.mk controller

image-sidecar-push: gen-semver
	make -f image.mk sidecar-push

image-controller-push: gen-semver
	make -f image.mk controller-push

build-base-image: gen-semver
	make -f image.mk build-base-image

images: gen-semver
	make -f image.mk images

images-push: gen-semver
	make -f image.mk images-push

image-sidecar-dev: build-sidecar-manager
	make -f image.mk sidecar-dev

image-controller-dev: build-controller-manager
	make -f image.mk controller-dev

image-migrator-dev: build-sidecar-migrator
	make -f image.mk sidecar-migrator-dev

image-node-rescanner-dev: build-sidecar-node-rescanner
	make -f image.mk sidecar-node-rescanner-dev

#To start mock-grpc server
start-server-win:
	go run test/mock-server/main.go --csi-address localhost:4772 --stubs test/mock-server/stubs

start-server-unix:
	go run test/mock-server/main.go --stubs test/mock-server/stubs

#To run sidecar tests
run-sidecar-tests:
	(cd ./test/e2e-framework/sidecar; bash run.sh)

run-fake-e2e-test:
	go test test/e2e-framework/fake_integration_test.go -v

# To run e2e tests for controller
run-controller-tests:
	go test test/e2e-framework/controller_integration_test.go -v

# Execute unit tests for the csi-replicator controller and generate coverage report
unit-test-csi-replicator:
	go test ./controllers/csi-replicator/ ./pkg/connection/ -v -race -coverpkg=./controllers/csi-replicator/ -coverprofile cover.out

# Execute unit tests for the replication-controller and generate coverage report
unit-test-replication-controller:
	go test ./controllers/replication-controller/ -v -race -coverpkg=./controllers/replication-controller/ -coverprofile cover.out

# Execute all unit tests and generate coverage report
unit-test: clean test-cmd test-pkg test-controllers

clean:
	go clean -cache

# Execute unit tests in ./cmd and generate coverage report
test-cmd:
	( cd cmd; go test -race -cover ./... -coverprofile cmd-cover.out )

# Execute unit tests in ./pkg and generate coverage report
test-pkg:
	( cd pkg; go test -race -cover ./... -coverprofile pkg-cover.out )

# Execute unit tests in ./controllers and generate coverage report
test-controllers:
	( cd controllers; go test -race -cover ./... -coverprofile ctrl-cover.out )

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.15.0

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_TOOLS_VERSION} ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# find or download kustomize
kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_TMP_DIR ;\
	go mod init tmp ;\
	GO111MODULE=on go install sigs.k8s.io/kustomize/kustomize/v5@${KUSTOMIZE_VERSION} ;\
	rm -rf $$KUSTOMIZE_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

.PHONY: actions action-help
actions: ## Run all GitHub Action checks that run on a pull request creation
	@echo "Running all GitHub Action checks for pull request events..."
	@act -l | grep -v ^Stage | grep pull_request | grep -v image_security_scan | awk '{print $$2}' | while read WF; do \
		echo "Running workflow: $${WF}"; \
		act pull_request --no-cache-server --platform ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest --job "$${WF}"; \
	done

action-help: ## Echo instructions to run one specific workflow locally
	@echo "GitHub Workflows can be run locally with the following command:"
	@echo "act pull_request --no-cache-server --platform ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest --job <jobid>"
	@echo ""
	@echo "Where '<jobid>' is a Job ID returned by the command:"
	@echo "act -l"
	@echo ""
	@echo "NOTE: if act is not installed, it can be downloaded from https://github.com/nektos/act"