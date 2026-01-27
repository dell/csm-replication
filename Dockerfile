# Copyright © 2025-2026 Dell Inc. or its subsidiaries. All Rights Reserved.
#
# Dell Technologies, Dell and other trademarks are trademarks of Dell Inc.
# or its subsidiaries. Other trademarks may be trademarks of their respective 
# owners.

# some arguments that must be supplied
ARG BASEIMAGE
ARG GOIMAGE

# Build the manager binary
FROM $GOIMAGE AS builder
ARG IMAGE
ARG VERSION

RUN mkdir -p /go/src/csm-replication
COPY ./ /go/src/csm-replication

WORKDIR /go/src/csm-replication

# Build specific image.
RUN if [ "$IMAGE" = "dell-replication-controller" ]; then \
    make build-controller-manager IMAGE_VERSION=$VERSION; \
elif [ "$IMAGE" = "dell-csi-replicator" ]; then \
    make build-sidecar-manager IMAGE_VERSION=$VERSION; \
elif [ "$IMAGE" = "dell-csi-migrator" ]; then \
    make build-sidecar-migrator IMAGE_VERSION=$VERSION; \
elif [ "$IMAGE" = "dell-csi-node-rescanner" ]; then \
    make build-sidecar-node-rescanner IMAGE_VERSION=$VERSION; \
else \
    echo "IMAGE supplied is $IMAGE. Not supported."; \
fi

# Base image
FROM $BASEIMAGE AS container-base
ARG VERSION
LABEL vendor="Dell Technologies" \
      maintainer="Dell Technologies" \
      release="1.16.0" \
      license="Apache-2.0"

FROM container-base AS controller
ARG VERSION
COPY /licenses /licenses
COPY --from=builder /go/src/csm-replication/bin/dell-replication-controller /
ENTRYPOINT ["/dell-replication-controller"]
LABEL version=$VERSION \
    name="dell-replication-controller" \
    description="CSI Replication controller" \
    summary="Controller which replicates the resources across (or within) Kubernetes clusters"

FROM container-base AS replicator
ARG VERSION
COPY /licenses /licenses
COPY --from=builder /go/src/csm-replication/bin/dell-csi-replicator /
ENTRYPOINT ["/dell-csi-replicator"]
LABEL version=$VERSION \
    name="dell-csi-replicator" \
    description="CSI Replicator sidecar" \
    summary="Sidecar used for managing the replication processes"

FROM container-base AS migrator
ARG VERSION
COPY /licenses /licenses
COPY --from=builder /go/src/csm-replication/bin/dell-csi-migrator /
ENTRYPOINT ["/dell-csi-migrator"]
LABEL version=$VERSION \
    name="dell-csi-migrator" \
    description="CSI Migrator sidecar" \
    summary="Sidecar used for managing the replication processes" 

FROM container-base AS rescanner
ARG VERSION
COPY /licenses /licenses
COPY --from=builder /go/src/csm-replication/bin/dell-csi-node-rescanner /
ENTRYPOINT ["/dell-csi-node-rescanner"]
LABEL version=$VERSION \
    name="dell-csi-node-rescanner" \
    description="CSI Node Rescanner sidecar" \
    summary="Sidecar used for managing the replication processes"
