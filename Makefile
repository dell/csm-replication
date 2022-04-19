# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Take tools from GOBIN
CONTROLLER_GEN=$(GOBIN)/controller-gen
KUSTOMIZE=$(GOBIN)/kustomize

IMG ?= "NOIMG"

#Generate semver.mk
gen-semver:
	(cd core; rm -f core_generated.go; go generate)
	go run core/semver/semver.go -f mk > semver.mk

# Run tests
test: generate fmt vet static-crd gen-semver
	go test ./... -coverprofile cover.out

# Build manager binary for csi-replicator
sidecar-manager: pre
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/dell-csi-replicator cmd/csi-replicator/main.go
sidecar-migrator: pre
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/dell-csi-migrator cmd/csi-migrator/main.go
# Build manager binary for replication-controller
controller-manager: pre
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/dell-replication-controller cmd/replication-controller/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run-sidecar: pre static-crd
	go run cmd/csi-replicator/main.go

run-controller: pre static-crd
	go run cmd/replication-controller/main.go

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
	$(CONTROLLER_GEN) crd:trivialVersions=false paths="./..." output:crd:artifacts:config=config/crd/bases

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
tools:
	go list -f '{{range .Imports}}{{.}} {{end}}' pkg/tools/tools.go | xargs go install

# Generate code
generate: tools
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Pre-requisite for the build/run targets
pre: gen-semver fmt vet tools generate

# Build the container image
image-sidecar: gen-semver
	make -f image.mk sidecar

image-migrator: gen-semver
	make -f image.mk migrator-sidecar

image-migrator-push: gen-semver
	make -f image.mk sidecar-migrator-push

# Build the container image
image-controller: gen-semver
	make -f image.mk controller

image-sidecar-push: gen-semver
	make -f image.mk sidecar-push

image-controller-push: gen-semver
	make -f image.mk controller-push

images: gen-semver
	make -f image.mk images

images-push: gen-semver
	make -f image.mk images-push

image-sidecar-dev: sidecar-manager
	make -f image.mk sidecar-dev

image-controller-dev:	controller-manager
	make -f image.mk controller-dev

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
# Set the kubernetes config file path to REMOTE_KUBE_CONFIG_PATH environment variable
# Example: export REMOTE_KUBE_CONFIG_PATH=C:\config-files\remote-cluster-config.yaml
run-controller-tests:
	go test test/e2e-framework/controller_integration_test.go -v

#To generate coverage for the csi-replicator
gen-coverage-csi-replicator:
	go test ./controllers/csi-replicator/ ./pkg/connection/ -v -race -coverpkg=./controllers/csi-replicator/ -coverprofile cover.out

#To generate coverage for the replication-controller
gen-coverage-replication-controller:
	go test ./controllers/replication-controller/ -v -race -coverpkg=./controllers/replication-controller/ -coverprofile cover.out

#To generate coverage for all the controllers
gen-coverage:
	( cd controllers; go clean -cache; go test -race -v -cover ./... -coverprofile cover.out )
