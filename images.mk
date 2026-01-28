# Copyright © 2026 Dell Inc. or its subsidiaries. All Rights Reserved.
#
# Dell Technologies, Dell and other trademarks are trademarks of Dell Inc.
# or its subsidiaries. Other trademarks may be trademarks of their respective 
# owners.

include overrides.mk
include helper.mk

CONTROLLER_IMAGE_NAME=dell-replication-controller
REPLICATOR_IMAGE_NAME=dell-csi-replicator
CONTROLLER_REPLICATOR_VERSION=1.14.0

MIGRATOR_IMAGE_NAME=dell-csi-migrator
MIGRATOR_VERSION=1.10.0

RESCANNER_IMAGE_NAME=dell-csi-node-rescanner
RESCANNER_VERSION=1.9.0

.PHONY: eval

eval:
	$(eval include csm-common.mk)
	$(eval include semver.mk)

image-build-controller: gen-semver vendor download-csm-common eval
	$(BUILDER) build --pull -t "$(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_NAME):$(IMAGE_TAG)" --target controller --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE) --build-arg IMAGE=$(CONTROLLER_IMAGE_NAME) --build-arg VERSION=$(CONTROLLER_REPLICATOR_VERSION) .

image-push-controller:
	$(BUILDER) push "$(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_NAME):$(IMAGE_TAG)"

image-build-replicator: gen-semver vendor download-csm-common eval
	$(eval include csm-common.mk)
	$(BUILDER) build --pull -t "$(IMAGE_REGISTRY)/$(REPLICATOR_IMAGE_NAME):$(IMAGE_TAG)" --target replicator --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE) --build-arg IMAGE=$(REPLICATOR_IMAGE_NAME) --build-arg VERSION=$(CONTROLLER_REPLICATOR_VERSION) .

image-push-replicator:
	$(BUILDER) push "$(IMAGE_REGISTRY)/$(REPLICATOR_IMAGE_NAME):$(IMAGE_TAG)"

image-build-migrator: gen-semver vendor download-csm-common eval
	$(eval include csm-common.mk)
	$(BUILDER) build --pull -t "$(IMAGE_REGISTRY)/$(MIGRATOR_IMAGE_NAME):$(IMAGE_TAG)" --target migrator --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE) --build-arg IMAGE=$(MIGRATOR_IMAGE_NAME) --build-arg VERSION=$(MIGRATOR_VERSION) .

image-push-migrator:
	$(BUILDER) push "$(IMAGE_REGISTRY)/$(MIGRATOR_IMAGE_NAME):$(IMAGE_TAG)"

image-build-rescanner: gen-semver vendor download-csm-common eval
	$(eval include csm-common.mk)
	$(BUILDER) build --pull -t "$(IMAGE_REGISTRY)/$(RESCANNER_IMAGE_NAME):$(IMAGE_TAG)" --target rescanner --build-arg BASEIMAGE=$(CSM_BASEIMAGE) --build-arg GOIMAGE=$(DEFAULT_GOIMAGE) --build-arg IMAGE=$(RESCANNER_IMAGE_NAME) --build-arg VERSION=$(RESCANNER_VERSION) .

image-push-rescanner:
	$(BUILDER) push "$(IMAGE_REGISTRY)/$(RESCANNER_IMAGE_NAME):$(IMAGE_TAG)"

images: image-build-controller image-build-replicator image-build-migrator image-build-rescanner
push: image-push-controller image-push-replicator image-push-migrator image-push-rescanner
