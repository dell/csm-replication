# some arguments that must be supplied
ARG BASEIMAGE
ARG GOIMAGE
# Build the manager binary
FROM $GOIMAGE AS builder

RUN mkdir -p /workspace/dell-csi-replicator
WORKDIR /workspace/dell-csi-replicator

# Copy the Go Modules manifests
COPY go.mod go.sum ./
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY api api/
COPY controllers controllers/
COPY core core/
COPY pkg pkg/
COPY test/e2e-framework test/e2e-framework/
COPY cmd/replication-controller/main.go controller.go
COPY cmd/csi-replicator/main.go sidecar.go
COPY cmd/csi-migrator/main.go migrator.go
COPY cmd/csi-node-rescanner/main.go node-rescanner.go

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o dell-replication-controller controller.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o dell-csi-replicator sidecar.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o dell-csi-migrator migrator.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o dell-csi-node-rescanner node-rescanner.go

# Base image
FROM $BASEIMAGE AS container-base
LABEL vendor="Dell Technologies" \
      maintainer="Dell Technologies" \
      release="1.13.0" \
      version="1.11.0" \
      license="Apache-2.0"

# Controller
FROM container-base AS controller
COPY licenses licenses/
WORKDIR /
COPY --from=builder /workspace/dell-csi-replicator/dell-replication-controller ./
ENTRYPOINT ["/dell-replication-controller"]
LABEL name="dell-replication-controller" \
    description="CSI Replication controller" \
    summary="Controller which replicates the resources across (or within) Kubernetes clusters"

# Sidecar
FROM container-base AS sidecar
COPY licenses licenses/
WORKDIR /
COPY --from=builder /workspace/dell-csi-replicator/dell-csi-replicator ./
ENTRYPOINT ["/dell-csi-replicator"]
LABEL name="dell-csi-replicator" \
    description="CSI Replicator sidecar" \
    summary="Sidecar used for managing the replication processes"

# Sidecar migrator
FROM container-base AS migrator
COPY licenses licenses/
WORKDIR /
COPY --from=builder /workspace/dell-csi-replicator/dell-csi-migrator ./
ENTRYPOINT ["/dell-csi-migrator"]
LABEL name="dell-csi-migrator" \
    description="CSI Migrator sidecar" \
    summary="Sidecar used for managing the replication processes"

# Sidecar node-rescanner
FROM container-base AS node-rescanner
COPY licenses licenses/
WORKDIR /
COPY --from=builder /workspace/dell-csi-replicator/dell-csi-node-rescanner ./
ENTRYPOINT ["/dell-csi-node-rescanner"]
LABEL name="dell-csi-node-rescanner" \
    description="CSI Node Rescanner sidecar" \
    summary="Sidecar used for managing the replication processes"
