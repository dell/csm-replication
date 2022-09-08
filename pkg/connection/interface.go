/*
 Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
      http://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package connection

import (
	"context"

	repv1 "github.com/dell/csm-replication/api/v1"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	apiExtensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// RemoteClusterClient interface provides methods for creating, modifying, deleting objects on a remote k8s cluster
type RemoteClusterClient interface {
	GetStorageClass(ctx context.Context, storageClassName string) (*storageV1.StorageClass, error)
	ListStorageClass(ctx context.Context) (*storageV1.StorageClassList, error)
	CreateStorageClass(ctx context.Context, storageClass *storageV1.StorageClass) error
	ListCustomResourceDefinitions(ctx context.Context) (*apiExtensionsv1.CustomResourceDefinitionList, error)
	GetCustomResourceDefinitions(ctx context.Context, crdName string) (*apiExtensionsv1.CustomResourceDefinition, error)
	GetPersistentVolume(ctx context.Context, persistentVolumeName string) (*corev1.PersistentVolume, error)
	CreatePersistentVolume(ctx context.Context, volume *corev1.PersistentVolume) error
	UpdatePersistentVolume(ctx context.Context, volume *corev1.PersistentVolume) error
	GetPersistentVolumeClaim(ctx context.Context, namespace, claimName string) (*corev1.PersistentVolumeClaim, error)
	UpdatePersistentVolumeClaim(ctx context.Context, claim *corev1.PersistentVolumeClaim) error
	GetReplicationGroup(ctx context.Context, replicationGroupName string) (*repv1.DellCSIReplicationGroup, error)
	UpdateReplicationGroup(ctx context.Context, group *repv1.DellCSIReplicationGroup) error
	ListReplicationGroup(ctx context.Context) (*repv1.DellCSIReplicationGroupList, error)
	CreateReplicationGroup(ctx context.Context, group *repv1.DellCSIReplicationGroup) error
	CreateSnapshotContent(ctx context.Context, content *s1.VolumeSnapshotContent) error
}

// ConnHandler - Interface
type ConnHandler interface {
	Verify(ctx context.Context) error
	GetConnection(clusterID string) (RemoteClusterClient, error)
}

// MultiClusterClient interface of client that manages multiple clusters at once
type MultiClusterClient interface {
	GetConnection(clusterID string) (RemoteClusterClient, error)
	GetClusterID() string
}
