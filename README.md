<!--
Copyright (c) 2021 Dell Inc., or its subsidiaries. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
-->

# Dell EMC Container Storage Modules (CSM) for Replication

[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0%20adopted-ff69b4.svg)](https://github.com/dell/csm/blob/main/docs/CODE_OF_CONDUCT.md)
[![License](https://img.shields.io/github/license/dell/csm-replication)](LICENSE)
[![Docker Pulls](https://img.shields.io/docker/pulls/dellemc/csm-replication)](https://hub.docker.com/r/dellemc/csm-replication)
[![Go version](https://img.shields.io/github/go-mod/go-version/dell/csm-replication)](go.mod)
[![GitHub release (latest by date including pre-releases)](https://img.shields.io/github/v/release/dell/csm-replication?include_prereleases&label=latest&style=flat-square)](https://github.com/dell/csm-replication/releases/latest)

Dell EMC CSM for Replication is part of the [CSM (Container Storage Modules)](https://github.com/dell/csm) open-source suite of Kubernetes storage enablers for Dell EMC products.

This project aims at extending native Kubernetes functionality to support _Disaster Recovery_ workflows by utilizing storage array based replication.

CSM for Replication includes the following components:
* Dell Replication CRDs - [Custom Resource definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
* Dell CSI Replication - sidecar container for CSI drivers
* Dell Replication Controller - Multi-Cluster Controller
* repctl - helper utility for setups, queries

_*Dell CSI Replication*_ sidecar is installed as part of the CSI driver controller plugin and utilizes custom Dell CSI extensions to communicate
with Dell CSI drivers. It includes a set of Kubernetes controllers which keep watch on requests related to Persistent Volumes (PVs)
 and Persistent Volume Claims(PVCs). It uses `dell-csi-extensions` APIs to perform replication related actions.
You can read more about the extensions [here](https://eos2git.cec.lab.emc.com/DevCon/dell-csi-extensions).


_*Dell Replication Controller*_ is a set of Kubernetes controllers which are _multi-cluster_ aware and can communicate with various Kubernetes API servers
to reconcile state. These controllers are responsible for the syncing of objects and the associated metadata.

This project has been created using [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder), and the various kubernetes controllers
have been implemented using [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime). Additionally, this project uses [kustomize](https://github.com/kubernetes-sigs/kustomize) for generating various `yaml` manifests for Kubernetes objects.

For documentation, please visit [Container Storage Modules documentation](https://dell.github.io/csm-docs/).

## Table of Contents

* [Code of Conduct](https://github.com/dell/csm/blob/main/docs/CODE_OF_CONDUCT.md)
* [Maintainer Guide](https://github.com/dell/csm/blob/main/docs/MAINTAINER_GUIDE.md)
* [Committer Guide](https://github.com/dell/csm/blob/main/docs/COMMITTER_GUIDE.md)
* [Contributing Guide](https://github.com/dell/csm/blob/main/docs/CONTRIBUTING.md)
* [Branching Strategy](https://github.com/dell/csm/blob/main/docs/BRANCHING.md)
* [List of Adopters](https://github.com/dell/csm/blob/main/ADOPTERS.md)
* [Maintainers](https://github.com/dell/csm/blob/main/docs/MAINTAINERS.md)
* [Support](https://github.com/dell/csm/blob/main/docs/SUPPORT.md)
* [Security](https://github.com/dell/csm/blob/main/docs/SECURITY.md)
* [About](#about)

## Build

This project contains multiple go modules, and you can build multiple binaries & images with the provided Makefile.

### Dependencies

This project relies on the following tools which have to be installed in order to generate certain manifests.

| Tool | Version |
| --------- | ----------- |
| controller-gen | v0.7.2 |
| kustomize | v3.10.0 |

The above tools can also be installed by running the command `make tools`.

### Custom Resource Definitions

Run the command `make manifests` to build the Dell CSI Replication CRDs. This command invokes `controller-gen` to generate the CRDs. 
The API code is annotated with `kubebuilder` tags which are used by the`controller-gen` generators. The generated definitions are present in the form
of a _kustomize_ recipe in the `config/crd` folder.

### Binaries

For building  _dell-csi-replicator_ , run `make sidecar-manager`.  
For building the _dell-replication-controller_, run `make controller-manager`.  
For building the repctl binary, run `cd repctl && make build`.

### Static manifests

In order to simplify installation for users who don't want to use `kustomize`, statically generated manifests are stored in the repository.

* deploy/controller.yaml - manifest for installing the _Dell Replication Controller_.
* deploy/replicationcrds.all.yaml - manifest for install _Dell Replication CRDs_.

In case there are modifications done which update the CRD, run `make static-crd` and commit changes to the CRD file in the repository.

In case there are modifications done which update any manifests for the installation of the _Dell Replication Controller_, run
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
For building the RBAC rules for the common controller, run `make controller-rbac`.

## Installation

### Custom Resource Definitions
Run the command `make install` to install the Custom Resource Definitions in your Kubernetes cluster. Alternatively, you can directly install
the CRDs using the static manifest file `deploy/replicationcrds.all.yaml` by running the command -

```
kubectl create -f deploy/replicationcrds.all.yaml
```

**Note**: CRDs must be installed before installing either Dell CSI Replicator or Dell Replication Controller. 

### Dell CSI Replicator (sidecar)

The dell-csi-replicator container can only be installed as a sidecar within the CSI driver controller pod. The kubernetes controllers which
run as part of the sidecar container, communicate with the CSI drivers using [gRPC](https://grpc.io/) over a Unix Domain Socket (UDS). The address for this socket
can be passed as an argument to the sidecar container.

The [RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/) permissions required by the kubernetes controllers are present in the file
`config/sidecar-rbac/role.yaml` and should be included in the CSI driver's RBAC permissions. 

You can also run the `dell-csi-replicator` process directly in your Kubernetes cluster by running the command `make run-sidecar`. 
Make sure that the kubernetes user has the desired RBAC permissions as described above.

### Dell Replication Controller

#### Pre-requisites

Dell Replication Controller is always installed in the namespace `dell-replication-controller`. 
You should also edit the file `deploy/config.yaml` with the required details before proceeding with the installation.

#### Using Install script

Use the script `scripts/install.sh` to install the Dell Replication Controller. This script will do the following:
* Create namespace `dell-replication-controller`
* Create/Update configmap using the contents of `deploy/config.yaml`
* Install/Update the CRDs
* Install/Update the `dell-replication-controller` Deployment

**Note**: In case you want to install a specific version, make sure to update the `deploy/controller.yaml` to point to the desired image tag.

#### Using Makefile target

If you are building your own images using the provided Makefile targets, you can install the Dell Replication Controller
by running the command `make deploy-controller`. The namespace `dell-replication-controller` must exist in the cluster before this installation.

You can also run the `dell-replication-controller` process directly in your Kubernetes cluster by running the command `make run-controller`.
Make sure that the kubernetes user has the desired RBAC permissions.

## Testing

There are many unit tests written in the respective packages in `_test.go` files, Along with the unit tests there are integration tests written separately for both Sidecar and Replication controller. These integration tests cover complete end to end scenario testing starting from automated way of creating the resources PVC, PV and RG and by running the sidecar/replication controllers internally from the tests itself.

The integration test cases can be executed by using the `Makefile` recipe or from any IDE directly. These tests can be ran against any driver without any modification to the code by just updating the driver and the storage class to use in the stubs and in the integration tests config variable.

```
	suite.driver = common.Driver{
		DriverName:   "csi-unity.dellemc.com",
		StorageClass: "replication-unity",
	}
```

To run sidecar integration tests: 

```bash
make run-sidecar-tests
```

To run replication controller tests:

Set the remote cluster config file path into the environment variable and run the tests.

```bash
export REMOTE_KUBE_CONFIG_PATH=C:\config-files\remote-cluster-config.yaml
make run-controller-tests
```

To run unit tests: 
```bash
make gen-coverage
```