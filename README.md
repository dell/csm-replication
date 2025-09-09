<!--
 Copyright © 2021-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
      http://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->

# :lock: **Important Notice**
Starting with the release of **Container Storage Modules v1.16.0**, this repository will no longer be maintained as an open source project. Future development will continue under a closed source model. This change reflects our commitment to delivering even greater value to our customers by enabling faster innovation and more deeply integrated features with the Dell storage portfolio.<br>
For existing customers using Dell’s Container Storage Modules, you will continue to receive:
* **Ongoing Support & Community Engagement**<br>
       You will continue to receive high-quality support through Dell Support and our community channels. Your experience of engaging with the Dell community remains unchanged.
* **Streamlined Deployment & Updates**<br>
        Deployment and update processes will remain consistent, ensuring a smooth and familiar experience.
* **Access to Documentation & Resources**<br>
       All documentation and related materials will remain publicly accessible, providing transparency and technical guidance.
* **Continued Access to Current Open Source Version**<br>
       The current open-source version will remain available under its existing license for those who rely on it.

Moving to a closed source model allows Dell’s development team to accelerate feature delivery and enhance integration across our Enterprise Kubernetes Storage solutions ultimately providing a more seamless and robust experience.<br>
We deeply appreciate the contributions of the open source community and remain committed to supporting our customers through this transition.<br>

For questions or access requests, please contact the maintainers via [Dell Support](https://www.dell.com/support/kbdoc/en-in/000188046/container-storage-interface-csi-drivers-and-container-storage-modules-csm-how-to-get-support).

# Dell Container Storage Modules (CSM) for Replication

[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0%20adopted-ff69b4.svg)](https://github.com/dell/csm/blob/main/docs/CODE_OF_CONDUCT.md)
[![License](https://img.shields.io/github/license/dell/csm-replication)](LICENSE)
[![Docker Pulls](https://img.shields.io/docker/pulls/dellemc/dell-csi-replicator)](https://hub.docker.com/r/dellemc/dell-csi-replicator)
[![Go version](https://img.shields.io/github/go-mod/go-version/dell/csm-replication)](go.mod)
[![GitHub release (latest by date including pre-releases)](https://img.shields.io/github/v/release/dell/csm-replication?include_prereleases&label=latest&style=flat-square)](https://github.com/dell/csm-replication/releases/latest)

Dell CSM for Replication is part of the [CSM (Container Storage Modules)](https://github.com/dell/csm) open-source suite of Kubernetes storage enablers for Dell products.

This project aims at extending native Kubernetes functionality to support _Disaster Recovery_ workflows by utilizing storage array based replication.

CSM Replication project includes the following components:
* CSM Replication CRDs - [Custom Resource definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
* CSM Replication sidecar container for CSI drivers
* CSM Replication Controller - Multi-Cluster Controller
* repctl - helper utility for setup and queries

_*CSM Replication*_ sidecar is installed as part of the CSI driver controller pod and utilizes custom Dell CSI extensions to communicate
with Dell CSI drivers. It includes a set of Kubernetes controllers which keep watch on requests related to Persistent Volumes (PVs)
 and Persistent Volume Claims(PVCs). It uses `dell-csi-extensions` APIs to perform replication related actions.
You can read more about the extensions [here](https://github.com/dell/dell-csi-extensions).

_*CSM Replication Controller*_ is a set of Kubernetes controllers which are _multi-cluster_ aware and can communicate with various Kubernetes API servers
to reconcile state. These controllers are responsible for the syncing of objects and the associated metadata.

This project has been created using [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder), and the various kubernetes controllers
have been implemented using [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime). Additionally, this project uses [kustomize](https://github.com/kubernetes-sigs/kustomize) for generating various `yaml` manifests for Kubernetes objects.

For documentation, please visit [Container Storage Modules documentation](https://dell.github.io/csm-docs/).

## Table of Contents

* [Code of Conduct](https://github.com/dell/csm/blob/main/docs/CODE_OF_CONDUCT.md)
* [Maintainer Guide](https://github.com/dell/csm/blob/main/docs/MAINTAINER_GUIDE.md)
* [Committer Guide](https://github.com/dell/csm/blob/main/docs/COMMITTER_GUIDE.md)
* [Contributing Guide](https://github.com/dell/csm/blob/main/docs/CONTRIBUTING.md)
* [List of Adopters](https://github.com/dell/csm/blob/main/docs/ADOPTERS.md)
* [Dell support](https://www.dell.com/support/incidents-online/en-us/contactus/product/container-storage-modules)
* [Security](https://github.com/dell/csm/blob/main/docs/SECURITY.md)

## Build

This project contains multiple go modules, and you can build multiple binaries & images with the provided Makefile.

### Dependencies

This project relies on the following tools which have to be installed in order to generate certain manifests.

| Tool              | Version      |
| ----------------- | ------------ |
| controller-gen    | v0.15.0      |
| kustomize         | v5.4.3       |

The above tools can also be installed by running the command `make tools`.

### Custom Resource Definitions

Run the command `make manifests` to build the CSM Replication Custom Resource Definitions (CRDs). This command invokes `controller-gen` to generate the CRDs. 
The API code is annotated with `kubebuilder` tags which are used by the `controller-gen` generators. The generated definitions are present in the form
of a _kustomize_ recipe in the `config/crd` folder.

### Binaries
To build all the binaries associated with CSM Replication, run `make build`.

To compile individual binaries run:
- `make build-controller-manager` to build the CSM Replication Controller, _dell-replication-controller_.
- `make build-sidecar-manager` to build the CSM Replication sidecar, _dell-csi-replicator_.
- `make build-sidecar-migrator` to build the CSM Replication sidecar, _dell-csi-migrator_.
- `make build-sidecar-node-rescanner` to build the CSM Replication sidecar, _dell-csi-node-rescanner_.

#### Repctl
To build the repctl binary, run `cd repctl && make build`. 

### Static manifests

In order to simplify installation for users who don't want to use `kustomize`, statically generated manifests are stored in the repository.
* deploy/controller.yaml - manifest for installing the _CSM Replication Controller_.
* deploy/role.yaml - manifest for creating ClusterRole for _CSM Replication Controller_.
* deploy/replicationcrds.all.yaml - manifest for install _CSM Replication CRDs_.

In case there are modifications done which update the CRD, run `make static-crd` and commit changes to the CRD file in the repository.

In case there are modifications done which update any manifests for the installation of the _CSM Replication Controller_, run
`make static-controller` and commit changes to the file `deploy/controller.yaml` in the repository.

### Container Images

There are various Makefile targets available for building and pushing container images
| Target | Description |
| --------- | ----------- |
| image-sidecar | Build `dell-csi-replicator` sidecar image |
| image-sidecar-push | Build `dell-csi-replicator` sidecar image & push to an image repo |
| image-controller | Build `dell-replication-controller` image |
| image-controller-push | Build `dell-replication-controller` image & push to an image repo |
| images | Build both `dell-csi-replicator` & `dell-replication-controller` images |
| images-push | Push both `dell-csi-replicator` & `dell-replication-controller` images |

**Note**: `make images` will also tag the built images with the release tags and push those tags to the image repo.

### RBAC manifests
For building the RBAC rules for the sidecar, run `make sidecar-rbac`.
For building the RBAC rules for the controller, run `make controller-rbac`.

### Custom Resource Definitions

You can run the command `make install` to install the Custom Resource Definitions in your Kubernetes cluster.

### CSM Replication sidecar

The CSM Replication sidecar container can only be installed as a sidecar within the CSI driver controller pod. The kubernetes controllers which
run as part of the sidecar container, communicate with the CSI drivers using [gRPC](https://grpc.io/) over a Unix Domain Socket (UDS). The address for this socket
can be passed as an argument to the sidecar container. 

The [RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/) permissions required by the kubernetes controllers are present in the file
`config/sidecar-rbac/role.yaml` and should be included in the CSI driver's RBAC permissions. 

You can also run the `dell-csi-replicator` process directly in your Kubernetes cluster by running the command `make run-sidecar`. 
Make sure that the kubernetes user has the desired RBAC permissions as described above.

### CSM Replication Controller

You can install the CSM Replication Controller by running the command `make deploy-controller`. The namespace `dell-replication-controller` must exist in the cluster before this installation.

You can also run the `dell-replication-controller` process directly in your Kubernetes cluster by running the command `make run-controller`.
Make sure that the kubernetes user has the desired RBAC permissions.

#### Platform Notes

If you wish to install CSM Replication Controller with repctl on your openSUSE server, when referring to this [section](https://dell.github.io/csm-docs/docs/deployment/helm/modules/installation/replication/install-repctl/), you need to install `glibc-devel-static` devel package before running `make build`.

## Testing

Click [here](/TESTING.md) for details on how to test.
