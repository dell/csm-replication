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
	"fmt"
	"log"
	"sync"

	"github.com/dell/csm-replication/pkg/common"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	apiExtensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// RemoteK8sConnHandler handler of remote Kubernetes cluster connection
type RemoteK8sConnHandler struct {
	configs       map[string]*rest.Config
	lock          sync.Mutex
	cachedClients map[string]*RemoteK8sControllerClient
}

func (k8sConnHandler *RemoteK8sConnHandler) init() {
	if k8sConnHandler.configs == nil {
		k8sConnHandler.configs = make(map[string]*rest.Config)
	}
	if k8sConnHandler.cachedClients == nil {
		k8sConnHandler.cachedClients = make(map[string]*RemoteK8sControllerClient)
	}
}

// AddOrUpdateConfig adds (or updates) config to the list of managed clusters
func (k8sConnHandler *RemoteK8sConnHandler) AddOrUpdateConfig(clusterID string, config *rest.Config, log logr.Logger) {
	k8sConnHandler.lock.Lock()
	defer k8sConnHandler.lock.Unlock()
	k8sConnHandler.init()
	if _, ok := k8sConnHandler.configs[clusterID]; ok {
		log.V(common.DebugLevel).Info(fmt.Sprintf("Updating REST config for ClusterId: %s", clusterID))
		delete(k8sConnHandler.configs, clusterID)
		// Also delete any cached clients
		if _, ok := k8sConnHandler.cachedClients[clusterID]; ok {
			log.V(common.DebugLevel).Info(fmt.Sprintf("Deleting cached client for ClusterId: %s", clusterID))
			delete(k8sConnHandler.cachedClients, clusterID)
		}
	} else {
		log.V(common.InfoLevel).Info(fmt.Sprintf("Adding REST config for ClusterId: %s\n", clusterID))
	}
	k8sConnHandler.configs[clusterID] = config
}

// GetConnection returns client from the map of managed clusters
func (k8sConnHandler *RemoteK8sConnHandler) GetConnection(clusterID string) (RemoteClusterClient, error) {
	k8sConnHandler.lock.Lock()
	defer k8sConnHandler.lock.Unlock()
	k8sConnHandler.init()
	return k8sConnHandler.getControllerClient(clusterID)
}

func (k8sConnHandler *RemoteK8sConnHandler) getControllerClient(clusterID string) (*RemoteK8sControllerClient, error) {
	// First check if we have cached the client already
	if client, ok := k8sConnHandler.cachedClients[clusterID]; ok {
		log.Printf("Using cached client for ClusterId: %s\n", clusterID)
		return client, nil
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(repv1.AddToScheme(scheme))
	utilruntime.Must(apiExtensionsv1.AddToScheme(scheme))
	utilruntime.Must(s1.AddToScheme(scheme))
	if clientConfig, ok := k8sConnHandler.configs[clusterID]; ok {
		client, err := GetControllerClient(clientConfig, scheme)
		// client, err := ctrlClient.New(clientConfig, ctrlClient.Options{Scheme: scheme})
		if err != nil {
			return nil, err
		}
		remoteK8sClient := RemoteK8sControllerClient{
			ClusterID: clusterID,
			Client:    client,
		}
		k8sConnHandler.cachedClients[clusterID] = &remoteK8sClient
		return &remoteK8sClient, nil
	}
	return nil, fmt.Errorf("clusterID - %s not found", clusterID)
}

// Verify if the connections have been established properly and all the required APIs
// are available on the remote cluster
func (k8sConnHandler *RemoteK8sConnHandler) Verify(ctx context.Context) error {
	k8sConnHandler.lock.Lock()
	defer k8sConnHandler.lock.Unlock()
	k8sConnHandler.init()
	return k8sConnHandler.verifyControllerClients(ctx)
}

func (k8sConnHandler *RemoteK8sConnHandler) verifyControllerClients(ctx context.Context) error {
	for clusterID := range k8sConnHandler.configs {
		client, err := k8sConnHandler.getControllerClient(clusterID)
		if err != nil {
			return fmt.Errorf("failed to create client for clusterId: %s. error - %s", clusterID, err.Error())
		}
		crdList, err := client.ListCustomResourceDefinitions(ctx)
		if err != nil {
			return fmt.Errorf("failed to list CustomResourceDefinitions for ClusterId: %s. error - %s", clusterID, err.Error())
		}
		if len(crdList.Items) == 0 {
			return fmt.Errorf("couldn't find any CRD definitions for ClusterId: %s", clusterID)
		}
		found := false
		for _, crd := range crdList.Items {
			if crd.Name == "dellcsireplicationgroups.replication.storage.dell.com" {
				crd, err := client.GetCustomResourceDefinitions(ctx, crd.Name)
				if err != nil {
					return fmt.Errorf("failed to get CRD definition for DellCSIReplicationGroup for ClusterId: %s. error - %s", clusterID, err.Error())
				}
				if crd.Spec.Group == repv1.GroupVersion.Group && crd.Spec.Names.Kind == "DellCSIReplicationGroup" {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("failed to find DellCSIReplicationGroup in list of CRDs for ClusterId: %s", clusterID)
		}
	}
	return nil
}

// RemoteK8sControllerClient - Represents a single controller split client
type RemoteK8sControllerClient struct {
	ClusterID string
	Client    ctrlClient.Client
}

// GetStorageClass returns storage class object by querying cluster using storage class name
func (c *RemoteK8sControllerClient) GetStorageClass(ctx context.Context, storageClassName string) (*storageV1.StorageClass, error) {
	found := &storageV1.StorageClass{}
	err := c.Client.Get(ctx, types.NamespacedName{Name: storageClassName}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// ListStorageClass returns list of all storage classes objects that are currently in cluster
func (c *RemoteK8sControllerClient) ListStorageClass(ctx context.Context) (*storageV1.StorageClassList, error) {
	scList := &storageV1.StorageClassList{}
	err := c.Client.List(ctx, scList)
	if err != nil {
		return nil, err
	}
	return scList, nil
}

// CreateStorageClass creates storage class object in current cluster
func (c *RemoteK8sControllerClient) CreateStorageClass(ctx context.Context, storageClass *storageV1.StorageClass) error {
	return c.Client.Create(ctx, storageClass)
}

// GetPersistentVolume returns persistent volume object by querying cluster using persistent volume name
func (c *RemoteK8sControllerClient) GetPersistentVolume(ctx context.Context, persistentVolumeName string) (*corev1.PersistentVolume, error) {
	found := &corev1.PersistentVolume{}
	err := c.Client.Get(ctx, types.NamespacedName{Name: persistentVolumeName}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// CreatePersistentVolume creates persistent volume object in current cluster
func (c *RemoteK8sControllerClient) CreatePersistentVolume(ctx context.Context, volume *corev1.PersistentVolume) error {
	return c.Client.Create(ctx, volume)
}

// UpdatePersistentVolume updates persistent volume object in current cluster
func (c *RemoteK8sControllerClient) UpdatePersistentVolume(ctx context.Context, volume *corev1.PersistentVolume) error {
	return c.Client.Update(ctx, volume)
}

// GetReplicationGroup returns replication group object by querying cluster using replication group name
func (c *RemoteK8sControllerClient) GetReplicationGroup(ctx context.Context, replicationGroupName string) (*repv1.DellCSIReplicationGroup, error) {
	found := &repv1.DellCSIReplicationGroup{}
	err := c.Client.Get(ctx, types.NamespacedName{Name: replicationGroupName}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// UpdateReplicationGroup updates replication group object in current cluster
func (c *RemoteK8sControllerClient) UpdateReplicationGroup(ctx context.Context, replicationGroup *repv1.DellCSIReplicationGroup) error {
	return c.Client.Update(ctx, replicationGroup)
}

// CreateReplicationGroup creates replication group object in current cluster
func (c *RemoteK8sControllerClient) CreateReplicationGroup(ctx context.Context, group *repv1.DellCSIReplicationGroup) error {
	return c.Client.Create(ctx, group)
}

// ListReplicationGroup returns list of all replication group objects that are currently in cluster
func (c *RemoteK8sControllerClient) ListReplicationGroup(ctx context.Context) (*repv1.DellCSIReplicationGroupList, error) {
	rgList := &repv1.DellCSIReplicationGroupList{}
	err := c.Client.List(ctx, rgList)
	if err != nil {
		return nil, err
	}
	return rgList, nil
}

// ListCustomResourceDefinitions returns list of custom resource definition objects that are currently in cluster
func (c *RemoteK8sControllerClient) ListCustomResourceDefinitions(ctx context.Context) (*apiExtensionsv1.CustomResourceDefinitionList, error) {
	crdList := &apiExtensionsv1.CustomResourceDefinitionList{}
	err := c.Client.List(ctx, crdList)
	if err != nil {
		return nil, err
	}
	return crdList, nil
}

// GetCustomResourceDefinitions returns custom resource definition object by querying cluster using custom resource definition name
func (c *RemoteK8sControllerClient) GetCustomResourceDefinitions(ctx context.Context, crdName string) (*apiExtensionsv1.CustomResourceDefinition, error) {
	crd := &apiExtensionsv1.CustomResourceDefinition{}
	err := c.Client.Get(ctx, types.NamespacedName{Name: crdName}, crd)
	if err != nil {
		return nil, err
	}
	return crd, nil
}

// GetPersistentVolumeClaim returns persistent volume claim object by querying cluster using persistent volume claim name
func (c *RemoteK8sControllerClient) GetPersistentVolumeClaim(ctx context.Context, namespace, claimName string) (*corev1.PersistentVolumeClaim, error) {
	claim := &corev1.PersistentVolumeClaim{}
	err := c.Client.Get(ctx, types.NamespacedName{Name: claimName, Namespace: namespace}, claim)
	if err != nil {
		return nil, err
	}
	return claim, nil
}

// CreatePersistentVolumeClaim returns persistent volume claim object in current cluster
func (c *RemoteK8sControllerClient) CreatePersistentVolumeClaim(ctx context.Context, claim *corev1.PersistentVolumeClaim) error {
	return c.Client.Create(ctx, claim)
}

// UpdatePersistentVolumeClaim updates persistent volume claim object in current cluster
func (c *RemoteK8sControllerClient) UpdatePersistentVolumeClaim(ctx context.Context, claim *corev1.PersistentVolumeClaim) error {
	return c.Client.Update(ctx, claim)
}

// CreateSnapshotContent creates the snapshot content on the remote cluster
func (c *RemoteK8sControllerClient) CreateSnapshotContent(ctx context.Context, content *s1.VolumeSnapshotContent) error {
	return c.Client.Create(ctx, content)
}

// ListPersistentVolumeClaim gets the list of persistent volume claim objects under the replication group
func (c *RemoteK8sControllerClient) ListPersistentVolumeClaim(ctx context.Context, opts ...ctrlClient.ListOption) (*corev1.PersistentVolumeClaimList, error) {
	list := &corev1.PersistentVolumeClaimList{}

	err := c.Client.List(ctx, list, opts...)
	if err != nil {
		return nil, err
	}

	return list, nil
}

// CreateSnapshotObject creates the snapshot on the remote cluster
func (c *RemoteK8sControllerClient) CreateSnapshotObject(ctx context.Context, content *s1.VolumeSnapshot) error {
	return c.Client.Create(ctx, content)
}

// GetSnapshotClass returns snapshot class object by querying cluster using snapshot class name.
func (c *RemoteK8sControllerClient) GetSnapshotClass(ctx context.Context, snapClassName string) (*s1.VolumeSnapshotClass, error) {
	found := &s1.VolumeSnapshotClass{}
	err := c.Client.Get(ctx, types.NamespacedName{Name: snapClassName}, found)
	if err != nil {
		return nil, err
	}

	return found, nil
}

// CreateSnapshotObject creates the snapshot on the remote cluster
func (c *RemoteK8sControllerClient) CreateSnapshotClass(ctx context.Context, content *s1.VolumeSnapshotClass) error {
	return c.Client.Create(ctx, content)
}

// CreateNamespace creates a desired namespace on the remote cluster.
func (c *RemoteK8sControllerClient) CreateNamespace(ctx context.Context, content *corev1.Namespace) error {
	return c.Client.Create(ctx, content)
}

// GetNamespace returns the desired namespace from the remote cluster.
func (c *RemoteK8sControllerClient) GetNamespace(ctx context.Context, namespace string) (*corev1.Namespace, error) {
	found := &corev1.Namespace{}

	err := c.Client.Get(ctx, types.NamespacedName{Name: namespace}, found)
	if err != nil {
		return nil, err
	}

	return found, nil
}

func (c *RemoteK8sControllerClient) GetSnapshotContent(ctx context.Context, snapshotContentName string) (*s1.VolumeSnapshotContent, error) {
	found := &s1.VolumeSnapshotContent{}

	err := c.Client.Get(ctx, types.NamespacedName{Name: snapshotContentName}, found)
	if err != nil {
		return nil, err
	}

	return found, nil
}

// ListVolumeSnapshotContent gets the list of volume snapshot content objects under the replication group
func (c *RemoteK8sControllerClient) ListSnapshotContent(ctx context.Context, opts ...ctrlClient.ListOption) (*s1.VolumeSnapshotContentList, error) {
	list := &s1.VolumeSnapshotContentList{}

	err := c.Client.List(ctx, list, opts...)
	if err != nil {
		return nil, err
	}

	return list, nil
}

// GetControllerClient - Returns a controller client which reads and writes directly to API server
func GetControllerClient(restConfig *rest.Config, scheme *runtime.Scheme) (ctrlClient.Client, error) {
	// Create a temp client and use it
	var clientConfig *rest.Config
	var err error
	if restConfig == nil {
		clientConfig, err = config.GetConfig()
		if err != nil {
			return nil, err
		}
	} else {
		clientConfig = restConfig
	}
	client, err := ctrlClient.New(clientConfig, ctrlClient.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return client, nil
}
