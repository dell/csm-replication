package connection

import (
	"context"
	"errors"
	"os"
	"testing"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/go-logr/logr"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	apiExtensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func initScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(repv1.AddToScheme(scheme))
	utilruntime.Must(s1.AddToScheme(scheme))
	utilruntime.Must(storageV1.AddToScheme(scheme))
	utilruntime.Must(apiExtensionsv1.AddToScheme(scheme))
	return scheme
}

func TestRemoteK8sConnHandler_GetConnection(t *testing.T) {
	clusterID := "test-cluster"

	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{}

	// Create a new rest.Config object
	restConfig := &rest.Config{
		Host: "https://example.com",
	}

	// Create a new logger
	log := logr.Discard()

	// Add test-cluster config
	k8sConnHandler.AddOrUpdateConfig(clusterID, restConfig, log)

	// Get the connection for test-cluster
	connection, err := k8sConnHandler.GetConnection(clusterID)
	assert.NoError(t, err)
	assert.NotNil(t, connection)
}

func TestRemoteK8sConnHandler_GetConnectionNotFound(t *testing.T) {
	clusterID := "test-cluster"

	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{}

	// Get the connection for test-cluster
	_, err := k8sConnHandler.GetConnection(clusterID)
	assert.Error(t, err)
}

func TestRemoteK8sConnHandler_Verify(t *testing.T) {
	clusterID := "test-cluster"

	crd := &apiExtensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dellcsireplicationgroups.replication.storage.dell.com",
		},
		Spec: apiExtensionsv1.CustomResourceDefinitionSpec{
			Group: repv1.GroupVersion.Group,
			Names: apiExtensionsv1.CustomResourceDefinitionNames{
				Kind:     "DellCSIReplicationGroup",
				ListKind: "DellCSIReplicationGroupList",
			},
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()
	controllerClient := &RemoteK8sControllerClient{
		ClusterID: clusterID,
		Client:    client,
	}

	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{
		configs: make(map[string]*rest.Config),
		cachedClients: map[string]*RemoteK8sControllerClient{
			clusterID: controllerClient,
		},
	}

	// Create a new rest.Config object
	restConfig := &rest.Config{
		Host: "https://example.com",
	}

	// Create a new logger
	log := logr.Discard()

	// Add test-cluster config
	k8sConnHandler.AddOrUpdateConfig(clusterID, restConfig, log)

	// Verify the configured connections
	err := k8sConnHandler.Verify(context.Background())
	assert.NoError(t, err)
}

func TestRemoteK8sConnHandler_VerifyEmptyCRD(t *testing.T) {
	clusterID := "test-cluster"

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	controllerClient := &RemoteK8sControllerClient{
		ClusterID: clusterID,
		Client:    client,
	}

	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{
		configs: make(map[string]*rest.Config),
		cachedClients: map[string]*RemoteK8sControllerClient{
			clusterID: controllerClient,
		},
	}

	// Create a new rest.Config object
	restConfig := &rest.Config{
		Host: "https://example.com",
	}

	// Create a new logger
	log := logr.Discard()

	// Add test-cluster config
	k8sConnHandler.AddOrUpdateConfig(clusterID, restConfig, log)

	// Verify the configured connections
	err := k8sConnHandler.Verify(context.Background())
	assert.Error(t, err)
}

func TestRemoteK8sConnHandler_VerifyUnableToListCRD(t *testing.T) {
	clusterID := "test-cluster"

	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{}

	// Create a new rest.Config object
	restConfig := &rest.Config{
		Host: "https://example.com",
	}

	// Create a new logger
	log := logr.Discard()

	// Add test-cluster config
	k8sConnHandler.AddOrUpdateConfig(clusterID, restConfig, log)

	// Verify the configured connections
	err := k8sConnHandler.Verify(context.Background())
	assert.Error(t, err)
}

func TestRemoteK8sConnHandler_VerifyNoClusterID(t *testing.T) {
	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{}

	// Verify the configured connections
	err := k8sConnHandler.Verify(context.Background())
	assert.NoError(t, err)
}

func TestRemoteK8sConnHandler_VerifyInvalidCRD(t *testing.T) {
	clusterID := "test-cluster"

	crd := &apiExtensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other-crd",
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()
	controllerClient := &RemoteK8sControllerClient{
		ClusterID: clusterID,
		Client:    client,
	}

	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{
		configs: make(map[string]*rest.Config),
		cachedClients: map[string]*RemoteK8sControllerClient{
			clusterID: controllerClient,
		},
	}

	// Create a new rest.Config object
	restConfig := &rest.Config{
		Host: "https://example.com",
	}

	// Create a new logger
	log := logr.Discard()

	// Add test-cluster config
	k8sConnHandler.AddOrUpdateConfig(clusterID, restConfig, log)

	// Verify the configured connections
	err := k8sConnHandler.Verify(context.Background())
	assert.Error(t, err)
}

func TestRemoteK8sConnHandler_AddOrUpdateConfig(t *testing.T) {
	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{}

	// Create a new rest.Config object
	restConfig := &rest.Config{
		Host: "https://example.com",
	}

	// Create a new logger
	log := logr.Discard()

	// Add test-cluster config
	k8sConnHandler.AddOrUpdateConfig("test-cluster", restConfig, log)

	// Check that the config was added to the map
	config, ok := k8sConnHandler.configs["test-cluster"]
	if !ok {
		t.Error("Expected config to be added to the map, but it was not")
	}

	// Check that the config is the same as the one we added
	if config != restConfig {
		t.Errorf("Expected config to be %v, but it was %v", restConfig, config)
	}

	// Update test-cluster config
	k8sConnHandler.AddOrUpdateConfig("test-cluster", restConfig, log)

	// Check that the config was added to the map
	config, ok = k8sConnHandler.configs["test-cluster"]
	if !ok {
		t.Error("Expected config to be added to the map, but it was not")
	}

	// Check that the config is the same as the one we added
	if config != restConfig {
		t.Errorf("Expected config to be %v, but it was %v", restConfig, config)
	}
}

func TestRemoteK8sConnHandler_CleanCachedClients(t *testing.T) {
	clusterID := "test-cluster"

	// Create a new instance of the struct
	k8sConnHandler := &RemoteK8sConnHandler{}

	// Create a new rest.Config object
	restConfig := &rest.Config{
		Host: "https://example.com",
	}

	// Create a new logger
	log := logr.Discard()

	// Add test-cluster config
	k8sConnHandler.AddOrUpdateConfig(clusterID, restConfig, log)

	// Check that the config was added to the map
	config, ok := k8sConnHandler.configs[clusterID]
	if !ok {
		t.Error("Expected config to be added to the map, but it was not")
	}

	// Check that the config is the same as the one we added
	if config != restConfig {
		t.Errorf("Expected config to be %v, but it was %v", restConfig, config)
	}

	_, err := k8sConnHandler.GetConnection(clusterID)
	assert.NoError(t, err)

	// Update test-cluster config
	k8sConnHandler.AddOrUpdateConfig(clusterID, restConfig, log)

	// Check that the config was added to the map
	config, ok = k8sConnHandler.configs[clusterID]
	if !ok {
		t.Error("Expected config to be added to the map, but it was not")
	}

	// Check that the config is the same as the one we added
	if config != restConfig {
		t.Errorf("Expected config to be %v, but it was %v", restConfig, config)
	}
}

func TestRemoteK8sControllerClient_StorageClass(t *testing.T) {
	sc := &storageV1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sc",
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	err := controllerClient.CreateStorageClass(context.TODO(), sc)
	assert.NoError(t, err)

	result, err := controllerClient.GetStorageClass(context.TODO(), sc.Name)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	resultList, err := controllerClient.ListStorageClass(context.TODO())
	assert.NoError(t, err)
	assert.NotNil(t, resultList)
}

func TestRemoteK8sControllerClient_PersistentVolume(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	err := controllerClient.CreatePersistentVolume(context.TODO(), pv)
	assert.NoError(t, err)

	result, err := controllerClient.GetPersistentVolume(context.TODO(), pv.Name)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	result.Spec.StorageClassName = "test-sc"
	err = controllerClient.UpdatePersistentVolume(context.TODO(), result)
	assert.NoError(t, err)
}

func TestRemoteK8sControllerClient_ReplicationGroup(t *testing.T) {
	rg := &repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-rg",
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	err := controllerClient.CreateReplicationGroup(context.TODO(), rg)
	assert.NoError(t, err)

	result, err := controllerClient.GetReplicationGroup(context.TODO(), rg.Name)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	rg.Spec.Action = "failover_local"
	err = controllerClient.UpdateReplicationGroup(context.TODO(), rg)
	assert.NoError(t, err)

	resultList, err := controllerClient.ListReplicationGroup(context.TODO())
	assert.NoError(t, err)
	assert.NotNil(t, resultList)
}

func TestRemoteK8sControllerClient_GetPersistentVolumeClaim(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-persistent-volume-claim",
			Namespace: "test-ns",
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	result, err := controllerClient.GetPersistentVolumeClaim(context.TODO(), pvc.Namespace, pvc.Name)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRemoteK8sControllerClient_UpdatePresistentVolumeClaim(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-persistent-volume-claim",
			Namespace: "default",
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-persistent-volume-claim",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "test-volume",
		},
	}

	err := controllerClient.UpdatePersistentVolumeClaim(context.TODO(), pvc)
	assert.NoError(t, err)
}

func TestRemoteK8sControllerClient_CreateSnapshotContent(t *testing.T) {
	snapshotContent := &s1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot-content",
			Namespace: "default",
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	err := controllerClient.CreateSnapshotContent(context.TODO(), snapshotContent)
	assert.NoError(t, err)
}

func TestRemoteK8sControllerClient_CreateSnapshotObject(t *testing.T) {
	snapshot := &s1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
	}

	scheme := initScheme()

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	err := controllerClient.CreateSnapshotObject(context.TODO(), snapshot)
	assert.NoError(t, err)
}

func TestRemoteK8sControllerClient_GetSnapshotClass(t *testing.T) {
	snapshotClass := &s1.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-snapshot-class",
		},
	}

	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapshotClass).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	snClass, err := controllerClient.GetSnapshotClass(context.TODO(), snapshotClass.Name)
	assert.NoError(t, err)
	assert.NotNil(t, snClass)
}

func TestRemoteK8sControllerClient_CreateNamespace(t *testing.T) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
		},
	}

	scheme := initScheme()

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	err := controllerClient.CreateNamespace(context.TODO(), namespace)
	assert.NoError(t, err)
}

func TestRemoteK8sControllerClient_GetNamespace(t *testing.T) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
		},
	}

	scheme := initScheme()

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	ns, err := controllerClient.GetNamespace(context.TODO(), namespace.Name)
	assert.NoError(t, err)
	assert.NotNil(t, ns)
}

func TestRemoteK8sConnHandler_GetControllerClient(t *testing.T) {
	// Test case: Successful creation
	restConfig := &rest.Config{
		Host: "localhost",
	}

	scheme := initScheme()

	client, err := GetControllerClient(restConfig, scheme)
	assert.NoError(t, err)
	assert.NotNil(t, client)

	// Test case: Nil restConfig
	client, err = GetControllerClient(nil, scheme)

	// Through CI, assume file does not exist
	dirname, dirErr := os.UserHomeDir()
	assert.NoError(t, dirErr)
	if _, err2 := os.Stat(dirname + "/.kube/config"); errors.Is(err2, os.ErrNotExist) {
		assert.Error(t, err)
		assert.Nil(t, client)
	} else {
		assert.NoError(t, err)
		assert.NotNil(t, client)
	}
}

// Since a scheme does not contain the necessary objects and no objects have been created, retrieval fails.
func TestRemoteK8sConnHandler_InvalidRetrievals(t *testing.T) {
	genericName := "invalid"

	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	controllerClient := &RemoteK8sControllerClient{
		Client: client,
	}

	_, err := controllerClient.GetNamespace(context.TODO(), genericName)
	assert.Error(t, err)

	_, err = controllerClient.GetCustomResourceDefinitions(context.TODO(), genericName)
	assert.Error(t, err)

	_, err = controllerClient.GetReplicationGroup(context.TODO(), genericName)
	assert.Error(t, err)

	_, err = controllerClient.GetPersistentVolume(context.TODO(), genericName)
	assert.Error(t, err)

	_, err = controllerClient.GetStorageClass(context.TODO(), genericName)
	assert.Error(t, err)
}
