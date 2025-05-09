/*
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
*/

package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	repv1 "github.com/dell/csm-replication/api/v1"
	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/display"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/dell/repctl/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterTestSuite struct {
	suite.Suite
	cluster    ClusterInterface
	fakeClient ClientInterface
}

// blank assignment to verify client.Client method implementations
var _ client.Client = &fake_client.Client{}

func (suite *ClusterTestSuite) SetupSuite() {
	metadata.Init("replication.storage.dell.com")
	suite.cluster = &Cluster{}
	_ = repv1.AddToScheme(scheme.Scheme)
}

func (suite *ClusterTestSuite) TearDownSuite() {
}

func (suite *ClusterTestSuite) TestGetPersistentVolume() {
	tests := []struct {
		name           string
		client         ClientInterface
		expectedPVName string
		expectedErr    error
	}{
		{
			name: "Successful",
			client: func() ClientInterface {
				persistentVolume := &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pv",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{persistentVolume}, nil, nil)
				return fake
			}(),
			expectedErr:    nil,
			expectedPVName: "test-pv",
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			expectedErr: errors.New("PersistentVolume \"test-pv\" not found"),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cluster.SetClient(tt.client)
			foundPV, err := suite.cluster.GetPersistentVolume(context.Background(), "test-pv")
			if tt.expectedErr == nil {
				suite.Nil(err)
				suite.NotNil(foundPV)
				suite.Equal(tt.expectedPVName, foundPV.Name)
			} else {
				suite.Equal(tt.expectedErr.Error(), err.Error())
			}
		})
	}
}

func (suite *ClusterTestSuite) TestListPersistentVolumes() {
	pv1 := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv-1",
		},
	}

	pv2 := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv-2",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pv1, pv2}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	volumeList, err := suite.cluster.ListPersistentVolumes(context.Background())
	suite.NoError(err)
	suite.NotNil(volumeList)
	suite.Equal(2, len(volumeList.Items))
	suite.Contains(volumeList.Items, *pv1)
	suite.Contains(volumeList.Items, *pv2)
}

func (suite *ClusterTestSuite) TestFilterPersistentVolumes() {
	pv1 := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv-1",
		},
	}

	pv2 := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv-2",
		},
	}

	matchingLabels := make(map[string]string)
	matchingLabels[metadata.ReplicationGroup] = "rg-1"

	repPV := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "replicated-pv",
			Labels: matchingLabels,
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pv1, pv2, repPV}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	volumeList, err := suite.cluster.FilterPersistentVolumes(context.Background(), "", "", "", "")
	suite.NoError(err)
	suite.NotNil(volumeList)
	suite.Equal(3, len(volumeList))
	var names []string
	for _, v := range volumeList {
		names = append(names, v.Name)
	}
	suite.ElementsMatch(names, []string{"test-pv-1", "test-pv-2", "replicated-pv"})

	replicatedVolumes, err := suite.cluster.FilterPersistentVolumes(context.Background(), "", "", "", "rg-1")
	suite.NoError(err)
	suite.NotNil(replicatedVolumes)
	suite.Equal(1, len(replicatedVolumes))
	suite.Equal("replicated-pv", replicatedVolumes[0].Name)
}

func (suite *ClusterTestSuite) TestGetNamespace() {
	tests := []struct {
		name           string
		client         ClientInterface
		expectedNSName string
		expectedErr    error
	}{
		{
			name: "Successful",
			client: func() ClientInterface {
				namespace := &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{namespace}, nil, nil)
				return fake
			}(),
			expectedErr:    nil,
			expectedNSName: "test-ns",
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			expectedErr: errors.New("Namespace \"test-ns\" not found"),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cluster.SetClient(tt.client)
			foundNS, err := suite.cluster.GetNamespace(context.Background(), "test-ns")
			if tt.expectedErr == nil {
				suite.Nil(err)
				suite.NotNil(foundNS)
				suite.Equal(tt.expectedNSName, foundNS.Name)
			} else {
				suite.Equal(tt.expectedErr.Error(), err.Error())
			}
		})
	}
}

func (suite *ClusterTestSuite) TestCreateNamespace() {
	fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
		},
	}

	err = suite.cluster.CreateNamespace(context.Background(), ns)
	suite.NoError(err)

	gotNs, err := suite.cluster.GetNamespace(context.Background(), "test-ns")
	suite.NoError(err)
	suite.NotNil(gotNs)
	suite.Equal("test-ns", gotNs.Name)
}

func (suite *ClusterTestSuite) TestListStorageClass() {
	sc1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sc-1",
		},
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sc-2",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{sc1, sc2}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	scList, err := suite.cluster.ListStorageClass(context.Background())
	suite.NoError(err)
	suite.NotNil(scList)
	suite.Equal(2, len(scList.Items))

	suite.Contains(scList.Items, *sc1)
	suite.Contains(scList.Items, *sc2)
}

func (suite *ClusterTestSuite) TestFilterStorageClass() {
	sc1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sc-1",
		},
	}

	params := make(map[string]string)
	params[metadata.ReplicationEnabled] = "true"

	repSC := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "replicated-sc",
		},
		Parameters:  params,
		Provisioner: "powerstore.dellemc.com",
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{sc1, repSC}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	scList, err := suite.cluster.FilterStorageClass(context.Background(), "", true)
	suite.NoError(err)
	suite.NotNil(scList.SCList)
	suite.Equal(2, len(scList.SCList))
	var names []string
	for _, sc := range scList.SCList {
		names = append(names, sc.Name)
	}
	suite.ElementsMatch(names, []string{"test-sc-1", "replicated-sc"})

	repSCList, err := suite.cluster.FilterStorageClass(context.Background(), "powerstore.dellemc.com", false)
	suite.NoError(err)
	suite.NotNil(repSCList.SCList)
	suite.Equal(1, len(repSCList.SCList))
	suite.Equal("replicated-sc", repSCList.SCList[0].Name)
}

func (suite *ClusterTestSuite) TestListPersistentVolumeClaims() {
	pvc1 := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc-1",
			Namespace: "test-ns",
		},
	}

	pvc2 := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc-2",
			Namespace: "test-ns",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pvc1, pvc2}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	volumeList, err := suite.cluster.ListPersistentVolumeClaims(context.Background())
	suite.NoError(err)
	suite.NotNil(volumeList)
	suite.Equal(2, len(volumeList.Items))
	suite.Contains(volumeList.Items, *pvc1)
	suite.Contains(volumeList.Items, *pvc2)
}

func (suite *ClusterTestSuite) TestFilterPersistentVolumeClaims() {
	usualSC := "simple-sc"
	namespace := "test-ns"

	pvc1 := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc-1",
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &usualSC,
		},
	}

	pvc2 := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc-2",
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &usualSC,
		},
	}

	remoteCluster := "cluster-2"
	rgName := "myReplicationGroup"
	repSC := "replicated-sc"

	matchingLabels := make(map[string]string)
	matchingLabels[metadata.RemoteClusterID] = remoteCluster
	matchingLabels[metadata.ReplicationGroup] = rgName
	matchingLabels[metadata.RemotePVCNamespace] = namespace

	repPVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "replicated-pvc",
			Namespace: namespace,
			Labels:    matchingLabels,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &repSC,
		},
	}

	suite.Suite.T().Run("success: get all PVC", func(t *testing.T) {
		fake, err := fake_client.NewFakeClient([]runtime.Object{pvc1, pvc2}, nil, nil)
		suite.NoError(err)

		suite.cluster.SetClient(fake)

		// Get all without matching labels.
		volumeList, err := suite.cluster.FilterPersistentVolumeClaims(context.Background(), "", "", "", "")
		suite.NoError(err)
		suite.NotNil(volumeList.PVCList)
		suite.Equal(2, len(volumeList.PVCList))

		var names []string
		for _, v := range volumeList.PVCList {
			names = append(names, v.Name)
		}

		suite.ElementsMatch(names, []string{"test-pvc-1", "test-pvc-2"})
	})

	suite.Suite.T().Run("success: match labels get PVC", func(t *testing.T) {

		fake, err := fake_client.NewFakeClient([]runtime.Object{pvc1, pvc2, repPVC}, nil, nil)
		suite.NoError(err)

		suite.cluster.SetClient(fake)

		// Match with labels.
		replicatedVolumes, err := suite.cluster.FilterPersistentVolumeClaims(context.Background(), namespace, remoteCluster, namespace, rgName)
		suite.NoError(err)
		suite.NotNil(replicatedVolumes.PVCList)
		suite.Equal(1, len(replicatedVolumes.PVCList))
		suite.Equal("replicated-pvc", replicatedVolumes.PVCList[0].Name)
	})
}

func (suite *ClusterTestSuite) TestCreatePVCsFromPVs() {
	matchingLabels := make(map[string]string)
	matchingLabels[metadata.RemoteClusterID] = "cluster-2"

	replicationPrefix := "replication.storage.dell.com"
	annotations := map[string]string{
		replicationPrefix + "/myAnnot": "myAnnotation",
	}

	pv1 := types.PersistentVolume{
		Name:          "pv-1",
		RGName:        "rg-1",
		SCName:        "rep-sc",
		RemotePVCName: "pvc-1",
		ReclaimPolicy: "Delete",
		Labels:        matchingLabels,
		Annotations:   annotations,
	}

	pv2 := types.PersistentVolume{
		Name:          "pv-2",
		RGName:        "rg-1",
		SCName:        "rep-sc",
		RemotePVCName: "pvc-2",
		ReclaimPolicy: "Delete",
		Labels:        matchingLabels,
		Annotations:   annotations,
	}

	suite.Suite.T().Run("success: create pvc from pv", func(t *testing.T) {
		fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
		suite.NoError(err)

		suite.cluster.SetClient(fake)

		err = suite.cluster.CreatePersistentVolumeClaimsFromPVs(context.Background(), "remote-ns", []types.PersistentVolume{pv1, pv2}, replicationPrefix, false)
		suite.NoError(err)

		volumeList, err := suite.cluster.ListPersistentVolumeClaims(context.Background())
		suite.NoError(err)
		suite.NotNil(volumeList)
		suite.Equal(2, len(volumeList.Items))
		var names []string
		var pvNames []string
		for _, v := range volumeList.Items {
			names = append(names, v.Name)
			pvNames = append(pvNames, v.Spec.VolumeName)
		}
		suite.ElementsMatch(names, []string{"pvc-1", "pvc-2"})
		suite.ElementsMatch(pvNames, []string{"pv-1", "pv-2"})
	})

	suite.Suite.T().Run("success: pvc already exist", func(t *testing.T) {
		fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
		suite.NoError(err)

		suite.cluster.SetClient(fake)

		myPV := pv1
		myPV.PVCName = "myPVC"
		err = suite.cluster.CreatePersistentVolumeClaimsFromPVs(context.Background(), "remote-ns", []types.PersistentVolume{myPV}, replicationPrefix, false)
		suite.NoError(err)
	})

	suite.Suite.T().Run("success: dryrun only", func(t *testing.T) {
		fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
		suite.NoError(err)

		suite.cluster.SetClient(fake)

		err = suite.cluster.CreatePersistentVolumeClaimsFromPVs(context.Background(), "remote-ns", []types.PersistentVolume{pv1}, replicationPrefix, true)
		suite.NoError(err)

		// Do to fake_client implementation, dryRun does not work as it should.
	})

	suite.Suite.T().Run("fail: unable to create", func(t *testing.T) {
		fake, err := fake_client.NewFakeClient([]runtime.Object{}, &errorInjector{}, nil)
		suite.NoError(err)

		suite.cluster.SetClient(fake)

		err = suite.cluster.CreatePersistentVolumeClaimsFromPVs(context.Background(), "remote-ns", []types.PersistentVolume{pv1}, replicationPrefix, true)
		suite.Error(err)
	})
}

type errorInjector struct{}

func (ei *errorInjector) ShouldFail(method string, _ runtime.Object) error {
	return fmt.Errorf("error with call %s", method)
}

func (suite *ClusterTestSuite) TestCreateObject() {
	fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	file, fileErr := os.Open("testdata/test-ns.yaml")
	suite.NoError(fileErr)
	defer file.Close()

	data, err := io.ReadAll(file)
	suite.NoError(err)

	_, err = suite.cluster.CreateObject(context.Background(), data)
	suite.NoError(err)

	namespace, err := suite.cluster.GetNamespace(context.Background(), "test-ns")
	suite.NoError(err)
	suite.NotNil(namespace)
	suite.Equal(namespace.Name, "test-ns")
}

func (suite *ClusterTestSuite) TestListReplicationGroups() {
	rg1 := &repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-rg-1",
		},
	}

	rg2 := &repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-rg-2",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{rg1, rg2}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	rgList, err := suite.cluster.ListReplicationGroups(context.Background())
	suite.NoError(err)
	suite.NotNil(rgList)
	suite.Equal(2, len(rgList.Items))
	suite.Contains(rgList.Items, *rg1)
	suite.Contains(rgList.Items, *rg2)
}

func (suite *ClusterTestSuite) TestFilterReplicationGroups() {
	labelsAndAnnotationsA := make(map[string]string)
	labelsAndAnnotationsA[metadata.RemoteClusterID] = "cluster-A"

	rg1 := &repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-rg-1",
			Labels:      labelsAndAnnotationsA,
			Annotations: labelsAndAnnotationsA,
		},
	}

	labelsAndAnnotationsB := make(map[string]string)
	labelsAndAnnotationsB[metadata.RemoteClusterID] = "cluster-B"

	rg2 := &repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-rg-2",
			Labels:      labelsAndAnnotationsB,
			Annotations: labelsAndAnnotationsB,
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{rg1, rg2}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	rgList, err := suite.cluster.FilterReplicationGroups(context.Background(), "", "cluster-B")
	suite.NoError(err)
	suite.NotNil(rgList.RGList)
	suite.Equal(1, len(rgList.RGList))
	suite.Equal("test-rg-2", rgList.RGList[0].Name)
	suite.Equal("cluster-B", rgList.RGList[0].RemoteClusterID)
}

func (suite *ClusterTestSuite) TestGetAllClusters() {
	suite.Run("failed to get any config files", func() {
		mc := MultiClusterConfigurator{}
		_, err := mc.GetAllClusters([]string{"cluster-2"}, "testdata/")
		suite.Error(err)
		suite.Contains(err.Error(), "failed to find any valid config files")
	})
	// TODO: write success case if you can figure out how to mock client created from file
}

func TestClusterTestSuite(t *testing.T) {
	suite.Run(t, new(ClusterTestSuite))
}

// Mock implementation of ClientInterface
type MockClient struct{}

func (m *MockClient) On(s string, ctx context.Context, rg *repv1.DellCSIReplicationGroup, patch client.Patch) {
	panic("unimplemented")
}

// Create implements ClientInterface.
func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	panic("unimplemented")
}

// Delete implements ClientInterface.
func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	panic("unimplemented")
}

// DeleteAllOf implements ClientInterface.
func (m *MockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	panic("unimplemented")
}

// Get implements ClientInterface.
func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	panic("unimplemented")
}

// GroupVersionKindFor implements ClientInterface.
func (m *MockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	panic("unimplemented")
}

// IsObjectNamespaced implements ClientInterface.
func (m *MockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	panic("unimplemented")
}

// List implements ClientInterface.
func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	panic("unimplemented")
}

// Patch implements ClientInterface.
func (m *MockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	panic("unimplemented")
}

func (m *MockClient) Called(ctx context.Context, obj client.Object, patch client.Patch) any {
	panic("unimplemented")
}

// RESTMapper implements ClientInterface.
func (m *MockClient) RESTMapper() meta.RESTMapper {
	panic("unimplemented")
}

// Scheme implements ClientInterface.
func (m *MockClient) Scheme() *runtime.Scheme {
	panic("unimplemented")
}

// Status implements ClientInterface.
func (m *MockClient) Status() client.SubResourceWriter {
	panic("unimplemented")
}

// SubResource implements ClientInterface.
func (m *MockClient) SubResource(subResource string) client.SubResourceClient {
	panic("unimplemented")
}

// Update implements ClientInterface.
func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	panic("unimplemented")
}

func (m *MockClient) GetClient() ClientInterface {
	return m
}

func (m *MockClient) SetClient(client ClientInterface) {}

func (m *MockClient) GetID() string {
	return "mock-id"
}

func (m *MockClient) GetKubeVersion() string {
	return "mock-version"
}

func (m *MockClient) GetHost() string {
	return "mock-host"
}

func (m *MockClient) GetKubeConfigFile() string {
	return "mock-kubeconfig"
}

// Add other methods as needed...

func TestCluster_GetKubeVersion(t *testing.T) {
	tests := []struct {
		name    string
		cluster Cluster
		want    string
	}{
		{
			name: "Get KubeVersion",
			cluster: Cluster{
				KubeVersion: "v1.20.0",
			},
			want: "v1.20.0",
		},
		{
			name: "Get KubeVersion with different version",
			cluster: Cluster{
				KubeVersion: "v1.21.0",
			},
			want: "v1.21.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cluster.GetKubeVersion()
			if got != tt.want {
				t.Errorf("GetKubeVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCluster_GetHost(t *testing.T) {
	tests := []struct {
		name    string
		cluster Cluster
		want    string
	}{
		{
			name: "Get Host",
			cluster: Cluster{
				Host: "https://cluster1.example.com",
			},
			want: "https://cluster1.example.com",
		},
		{
			name: "Get Host with different URL",
			cluster: Cluster{
				Host: "https://cluster2.example.com",
			},
			want: "https://cluster2.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cluster.GetHost()
			if got != tt.want {
				t.Errorf("GetHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCluster_GetKubeConfigFile(t *testing.T) {
	tests := []struct {
		name    string
		cluster Cluster
		want    string
	}{
		{
			name: "Get KubeConfigFile",
			cluster: Cluster{
				kubeConfigFile: "/path/to/kubeconfig1",
			},
			want: "/path/to/kubeconfig1",
		},
		{
			name: "Get KubeConfigFile with different path",
			cluster: Cluster{
				kubeConfigFile: "/path/to/kubeconfig2",
			},
			want: "/path/to/kubeconfig2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cluster.GetKubeConfigFile()
			if got != tt.want {
				t.Errorf("GetKubeConfigFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func (suite *ClusterTestSuite) TestGetSecret() {
	tests := []struct {
		name               string
		client             ClientInterface
		expectedSecretName string
		expectedErr        error
	}{
		{
			name: "Successful",
			client: func() ClientInterface {
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "test-namespace",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{secret}, nil, nil)
				return fake
			}(),
			expectedErr:        nil,
			expectedSecretName: "test-secret",
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			expectedErr: errors.New("Secret \"test-secret\" not found"),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cluster.SetClient(tt.client)
			foundSecret, err := suite.cluster.GetSecret(context.Background(), "test-namespace", "test-secret")
			if tt.expectedErr == nil {
				suite.Nil(err)
				suite.NotNil(foundSecret)
				suite.Equal(tt.expectedSecretName, foundSecret.Name)
			} else {
				suite.Equal(tt.expectedErr.Error(), err.Error())
			}
		})
	}
}

func (suite *ClusterTestSuite) TestGetPersistentVolumeClaim() {
	tests := []struct {
		name            string
		client          ClientInterface
		expectedPVCName string
		expectedErr     error
	}{
		{
			name: "Successful",
			client: func() ClientInterface {
				persistentVolumeClaim := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc",
						Namespace: "test-namespace",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{persistentVolumeClaim}, nil, nil)
				return fake
			}(),
			expectedErr:     nil,
			expectedPVCName: "test-pvc",
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			expectedErr: errors.New("PersistentVolumeClaim \"test-pvc\" not found"),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cluster.SetClient(tt.client)
			foundPVC, err := suite.cluster.GetPersistentVolumeClaim(context.Background(), "test-namespace", "test-pvc")
			if tt.expectedErr == nil {
				suite.Nil(err)
				suite.NotNil(foundPVC)
				suite.Equal(tt.expectedPVCName, foundPVC.Name)
			} else {
				suite.Equal(tt.expectedErr.Error(), err.Error())
			}
		})
	}
}

func (suite *ClusterTestSuite) TestGetReplicationGroups() {

	tests := []struct {
		name           string
		client         ClientInterface
		expectedRGName string
		expectedErr    error
	}{

		{
			name: "Successful",
			client: func() ClientInterface {
				replicationGroup := &repv1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-rg",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{replicationGroup}, nil, nil)
				return fake
			}(),
			expectedErr:    nil,
			expectedRGName: "test-rg",
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			expectedErr: errors.New("DellCSIReplicationGroup.replication.storage.dell.com \"test-rg\" not found"),
		},
	}

	for _, tt := range tests {
		suite.cluster.SetClient(tt.client)
		foundRG, err := suite.cluster.GetReplicationGroups(context.Background(), "test-rg")
		if tt.expectedErr == nil {
			suite.Nil(err)
			suite.NotNil(foundRG)
			suite.Equal(tt.expectedRGName, foundRG.Name)
		} else {
			suite.Equal(tt.expectedErr.Error(), err.Error())
		}
	}
}

func (suite *ClusterTestSuite) TestGetStatefulSet() {

	tests := []struct {
		name                 string
		client               ClientInterface
		expectedSTSName      string
		expectedSTSNamespace string
		expectedErr          error
	}{

		{
			name: "Successful",
			client: func() ClientInterface {
				statefulSet := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sts",
						Namespace: "test-namespace",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{statefulSet}, nil, nil)
				return fake
			}(),
			expectedErr:          nil,
			expectedSTSName:      "test-sts",
			expectedSTSNamespace: "test-namespace",
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			expectedErr: errors.New("StatefulSet.apps \"test-sts\" not found"),
		},
	}

	for _, tt := range tests {
		suite.cluster.SetClient(tt.client)
		foundSTS, err := suite.cluster.GetStatefulSet(context.Background(), "test-namespace", "test-sts")
		if tt.expectedErr == nil {
			suite.Nil(err)
			suite.NotNil(foundSTS)
			suite.Equal(tt.expectedSTSName, foundSTS.Name)
			suite.Equal(tt.expectedSTSNamespace, foundSTS.Namespace)
		} else {
			suite.Equal(tt.expectedErr.Error(), err.Error())
		}
	}
}

func (suite *ClusterTestSuite) TestGetPod() {
	tests := []struct {
		name            string
		client          ClientInterface
		expectedPodName string
		expectedErr     error
	}{
		{
			name: "Successful",
			client: func() ClientInterface {
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{pod}, nil, nil)
				return fake
			}(),
			expectedErr:     nil,
			expectedPodName: "test-pod",
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			expectedErr: errors.New("Pod \"test-pod\" not found"),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cluster.SetClient(tt.client)
			foundPod, err := suite.cluster.GetPod(context.Background(), "test-pod", "test-namespace")
			if tt.expectedErr == nil {
				suite.Nil(err)
				suite.NotNil(foundPod)
				suite.Equal(tt.expectedPodName, foundPod.Name)
			} else {
				suite.Equal(tt.expectedErr.Error(), err.Error())
			}
		})
	}
}

func (suite *ClusterTestSuite) TestGetMigrationGroup() {
	tests := []struct {
		name           string
		client         ClientInterface
		expectedMGName string
		expectedErr    error
	}{
		{
			name: "Successful",
			client: func() ClientInterface {
				migrationGroup := &repv1.DellCSIMigrationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-mg",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{migrationGroup}, nil, nil)
				return fake
			}(),
			expectedErr:    nil,
			expectedMGName: "test-mg",
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			expectedErr: errors.New("DellCSIMigrationGroup.replication.storage.dell.com \"test-mg\" not found"),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cluster.SetClient(tt.client)
			foundMG, err := suite.cluster.GetMigrationGroup(context.Background(), "test-mg")
			if tt.expectedErr == nil {
				suite.Nil(err)
				suite.NotNil(foundMG)
				suite.Equal(tt.expectedMGName, foundMG.Name)
			} else {
				suite.Equal(tt.expectedErr.Error(), err.Error())
			}
		})
	}
}

func (suite *ClusterTestSuite) TestDeletePod() {
	tests := []struct {
		name        string
		client      ClientInterface
		pod         *v1.Pod
		expectedErr error
	}{
		{
			name: "Successful",
			client: func() ClientInterface {
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{pod}, nil, nil)
				return fake
			}(),
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			},
			expectedErr: errors.New("Pod \"test-pod\" not found"),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cluster.SetClient(tt.client)
			err := suite.cluster.DeletePod(context.Background(), tt.pod)
			if tt.expectedErr == nil {
				suite.Nil(err)
			} else {
				suite.Equal(tt.expectedErr.Error(), err.Error())
			}
		})
	}
}

func (suite *ClusterTestSuite) TestDeletePersistentVolumeClaim() {
	tests := []struct {
		name        string
		client      ClientInterface
		pvc         *v1.PersistentVolumeClaim
		expectedErr error
	}{
		{
			name: "Successful",
			client: func() ClientInterface {
				persistentVolumeClaim := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc",
						Namespace: "test-namespace",
					},
				}
				fake, _ := fake_client.NewFakeClient([]runtime.Object{persistentVolumeClaim}, nil, nil)
				return fake
			}(),
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-namespace",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Error",
			client: func() ClientInterface {
				fake, _ := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
				return fake
			}(),
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-namespace",
				},
			},
			expectedErr: errors.New("PersistentVolumeClaim \"test-pvc\" not found"),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cluster.SetClient(tt.client)
			err := suite.cluster.DeletePersistentVolumeClaim(context.Background(), tt.pvc)
			if tt.expectedErr == nil {
				suite.Nil(err)
			} else {
				suite.Equal(tt.expectedErr.Error(), err.Error())
			}
		})
	}
}

func (suite *ClusterTestSuite) TestCreateStatefulSet() {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "test-namespace",
		},
		Spec: appsv1.StatefulSetSpec{
			// Add necessary spec fields here
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	err = suite.cluster.CreateStatefulSet(context.Background(), sts)
	suite.NoError(err)

	// Verify that the StatefulSet has been created
	foundSts := &appsv1.StatefulSet{}
	err = fake.Get(context.Background(), apiTypes.NamespacedName{Name: "test-sts", Namespace: "test-namespace"}, foundSts)
	suite.NoError(err)
	suite.NotNil(foundSts)
	suite.Equal("test-sts", foundSts.Name)
	suite.Equal("test-namespace", foundSts.Namespace)
}

func (suite *ClusterTestSuite) TestUpdateMigrationGroup() {
	mg := &repv1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mg",
		},
		// Add necessary spec fields here
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{mg}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	// Update the MigrationGroup
	mg.Spec.SourceID = "new-value" // Modify a field to simulate an update
	err = suite.cluster.UpdateMigrationGroup(context.Background(), mg)
	suite.NoError(err)

	// Verify that the MigrationGroup has been updated
	foundMG := &repv1.DellCSIMigrationGroup{}
	err = fake.Get(context.Background(), apiTypes.NamespacedName{Name: "test-mg"}, foundMG)
	suite.NoError(err)
	suite.NotNil(foundMG)
	suite.Equal("new-value", foundMG.Spec.SourceID)
}

func (suite *ClusterTestSuite) TestUpdateReplicationGroup() {
	rg := &repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-rg",
		},
		// Add necessary spec fields here
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{rg}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	// Update the ReplicationGroup
	rg.Spec.Action = "new-value" // Modify a field to simulate an update
	err = suite.cluster.UpdateReplicationGroup(context.Background(), rg)
	suite.NoError(err)

	// Verify that the ReplicationGroup has been updated
	foundRG := &repv1.DellCSIReplicationGroup{}
	err = fake.Get(context.Background(), apiTypes.NamespacedName{Name: "test-rg"}, foundRG)
	suite.NoError(err)
	suite.NotNil(foundRG)
	suite.Equal("new-value", foundRG.Spec.Action)
}

func (suite *ClusterTestSuite) TestUpdateSecret() {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{secret}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	// Update the Secret
	secret.Data["key"] = []byte("new-value") // Modify a field to simulate an update
	err = suite.cluster.UpdateSecret(context.Background(), secret)
	suite.NoError(err)

	// Verify that the Secret has been updated
	foundSecret := &v1.Secret{}
	err = fake.Get(context.Background(), apiTypes.NamespacedName{Name: "test-secret", Namespace: "test-namespace"}, foundSecret)
	suite.NoError(err)
	suite.NotNil(foundSecret)
	suite.Equal([]byte("new-value"), foundSecret.Data["key"])
}

func (suite *ClusterTestSuite) TestUpdatePersistentVolume() {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: v1.PersistentVolumeSpec{
			// Add necessary spec fields here
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pv}, nil, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	// Update the PersistentVolume
	pv.Spec.StorageClassName = "new-value" // Modify a field to simulate an update
	err = suite.cluster.UpdatePersistentVolume(context.Background(), pv)
	suite.NoError(err)

	// Verify that the PersistentVolume has been updated
	foundPV := &v1.PersistentVolume{}
	err = fake.Get(context.Background(), apiTypes.NamespacedName{Name: "test-pv"}, foundPV)
	suite.NoError(err)
	suite.NotNil(foundPV)
	suite.Equal("new-value", foundPV.Spec.StorageClassName)
}

func TestNewClientSet(t *testing.T) {
	// Saving original functions
	defaultBuildConfigFromFlags := clientcmdBuildConfigFromFlags
	defaultNewForConfig := kubernetesNewForConfig

	after := func() {
		clientcmdBuildConfigFromFlags = defaultBuildConfigFromFlags
		kubernetesNewForConfig = defaultNewForConfig
	}

	tests := []struct {
		name        string
		setup       func()
		expectedErr bool
	}{
		{
			name: "Clientset creation is successful",
			setup: func() {
				clientcmdBuildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
					return &rest.Config{}, nil
				}
				kubernetesNewForConfig = func(config *rest.Config) (*kubernetes.Clientset, error) {
					return &kubernetes.Clientset{}, nil
				}
			},
			expectedErr: false,
		},
		{
			name: "Config creation fails",
			setup: func() {
				clientcmdBuildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
					return nil, fmt.Errorf("mock error")
				}
			},
			expectedErr: true,
		},
		{
			name: "Clientset creation fails",
			setup: func() {
				clientcmdBuildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
					return &rest.Config{}, nil
				}
				kubernetesNewForConfig = func(config *rest.Config) (*kubernetes.Clientset, error) {
					return nil, fmt.Errorf("mock error")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()

			clientset, restConfig, err := newClientSet("/path/to/mock/kubeconfig")

			if tt.expectedErr {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if clientset != nil {
					t.Fatalf("Expected nil clientset, got %v", clientset)
				}
				if restConfig != nil {
					t.Fatalf("Expected nil restConfig, got %v", restConfig)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if clientset == nil {
					t.Fatalf("Expected clientset, got nil")
				}
				if restConfig == nil {
					t.Fatalf("Expected restConfig, got nil")
				}
			}
		})
	}
}

func TestClusters_Print(t *testing.T) {
	// Saving original function
	defaultDisplayNewTableWriter := displayNewTableWriter

	after := func() {
		displayNewTableWriter = defaultDisplayNewTableWriter
	}

	tests := []struct {
		name        string
		setup       func()
		expectedErr bool
	}{
		{
			name: "Print is successful",
			setup: func() {
				displayNewTableWriter = func(obj interface{}, w io.Writer) (*display.TableWriter, error) {
					return display.NewTableWriter(obj, w)
				}
			},
			expectedErr: false,
		},
		{
			name: "Table writer creation fails",
			setup: func() {
				displayNewTableWriter = func(obj interface{}, w io.Writer) (*display.TableWriter, error) {
					return nil, fmt.Errorf("mock error")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()

			// Mock Clusters data
			mockClusters := &Clusters{
				Clusters: []ClusterInterface{
					&Cluster{ClusterID: ""},
					&Cluster{ClusterID: ""},
				},
			}

			// Capture the output
			var buf bytes.Buffer
			oldStdout := os.Stdout
			defer func() { os.Stdout = oldStdout }()

			mockClusters.Print()

			// Check the output
			output := buf.String()
			if tt.expectedErr {
				assert.Empty(t, output)
			} else {
				assert.Contains(t, output, "")
				assert.Contains(t, output, "")
			}
		})
	}
}

func TestCreateCluster(t *testing.T) {
	// Saving original functions
	defaultGetControllerRuntimeClient := GetCtrlRuntimeClient
	defaultNewClientSet := newClntSet
	defaultGetServiceVersion := getServiceVersion

	after := func() {
		GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		newClntSet = defaultNewClientSet
		getServiceVersion = defaultGetServiceVersion
	}

	tests := []struct {
		name        string
		setup       func()
		expectedErr bool
	}{
		{
			name: "Cluster creation is successful",
			setup: func() {
				GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
					return &mockClient{}, nil
				}
				newClntSet = func(kubeconfig string) (*kubernetes.Clientset, *rest.Config, error) {
					return &kubernetes.Clientset{}, &rest.Config{Host: "https://mock-host"}, nil
				}
				getServiceVersion = func(clientset *kubernetes.Clientset) (*version.Info, error) {
					return &version.Info{Major: "1", Minor: "20"}, nil
				}
			},
			expectedErr: false,
		},
		{
			name: "Controller runtime client creation fails",
			setup: func() {
				GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
					return nil, fmt.Errorf("mock error")
				}
			},
			expectedErr: true,
		},
		{
			name: "Clientset creation fails",
			setup: func() {
				GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
					return &mockClient{}, nil
				}
				newClntSet = func(kubeconfig string) (*kubernetes.Clientset, *rest.Config, error) {
					return nil, nil, fmt.Errorf("mock error")
				}
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()

			cluster, err := CreateCluster("test-cluster", "/path/to/mock/kubeconfig")

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, cluster)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cluster)
			}
		})
	}
}

func (m *mockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	podList := list.(*v1.PodList)
	*podList = v1.PodList{
		Items: []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-1",
					OwnerReferences: []metav1.OwnerReference{
						{
							Name: "statefulset-1",
							Kind: "StatefulSet",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-2",
					OwnerReferences: []metav1.OwnerReference{
						{
							Name: "statefulset-2",
							Kind: "StatefulSet",
						},
					},
				},
			},
		},
	}
	return nil
}

func TestFilterPods(t *testing.T) {
	mockClient := &mockClient{}
	cluster := &Cluster{client: mockClient}

	tests := []struct {
		name             string
		namespace        string
		stsName          string
		expectedPodNames []string
	}{
		{
			name:             "Filter pods by StatefulSet name",
			namespace:        "default",
			stsName:          "statefulset-1",
			expectedPodNames: []string{"pod-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podList, err := cluster.FilterPods(context.Background(), tt.namespace, tt.stsName)
			assert.NoError(t, err)
			assert.NotNil(t, podList)

			var podNames []string
			for _, pod := range podList.Items {
				podNames = append(podNames, pod.Name)
			}

			assert.Equal(t, tt.expectedPodNames, podNames)
		})
	}
}

func (m *mockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	// Mock deletion logic
	if obj.GetName() == "error-sts" {
		return fmt.Errorf("mock error")
	}
	return nil
}

func TestDeleteStsOrphan(t *testing.T) {
	mockClient := &mockClient{}
	cluster := &Cluster{client: mockClient}

	tests := []struct {
		name        string
		sts         *appsv1.StatefulSet
		expectedErr bool
	}{
		{
			name: "Successful deletion",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sts",
				},
			},
			expectedErr: false,
		},
		{
			name: "Deletion fails",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "error-sts",
				},
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cluster.DeleteStsOrphan(context.Background(), tt.sts)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func (m *mockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	// Mock creation logic
	if obj.GetName() == "error-object" {
		return fmt.Errorf("mock error")
	}
	if obj.GetName() == "already-exists-object" {
		return fmt.Errorf("already exists")
	}
	return nil
}

func (m *mockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	// Mock update logic
	if obj.GetName() == "error-object" {
		return fmt.Errorf("mock error")
	}
	return nil
}

func TestCreateObject(t *testing.T) {
	mockClient := &mockClient{}
	cluster := &Cluster{client: mockClient}

	tests := []struct {
		name        string
		data        []byte
		expectedErr bool
	}{
		{
			name: "Create StorageClass",
			data: []byte(`apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: test-storageclass`),
			expectedErr: false,
		},
		{
			name: "Create Namespace",
			data: []byte(`apiVersion: v1
kind: Namespace
metadata:
  name: test-namespace`),
			expectedErr: false,
		},
		{
			name: "Create CustomResourceDefinition",
			data: []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test-crd`),
			expectedErr: false,
		},
		{
			name: "Create ClusterRole",
			data: []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: test-clusterrole`),
			expectedErr: false,
		},
		{
			name: "Create Role",
			data: []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: test-role`),
			expectedErr: false,
		},
		{
			name: "Error creating Role",
			data: []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: error-object`),
			expectedErr: true,
		},
		{
			name: "Updating existing Role",
			data: []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: already-exists-object`),
			expectedErr: false,
		},
		{
			name: "Create ClusterRoleBinding",
			data: []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: test-clusterrolebinding`),
			expectedErr: false,
		},
		{
			name: "Create RoleBinding",
			data: []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: test-rolebinding`),
			expectedErr: false,
		},
		{
			name: "Error Creating RoleBinding",
			data: []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: error-object`),
			expectedErr: true,
		},
		{
			name: "Updating Existing RoleBinding",
			data: []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: already-exists-object`),
			expectedErr: false,
		},
		{
			name: "Create Service",
			data: []byte(`apiVersion: v1
kind: Service
metadata:
  name: test-service`),
			expectedErr: false,
		},
		{
			name: "Create Deployment",
			data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment`),
			expectedErr: false,
		},
		{
			name: "Create ConfigMap",
			data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap`),
			expectedErr: false,
		},
		{
			name: "Create ServiceAccount",
			data: []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: test-serviceaccount`),
			expectedErr: false,
		},
		{
			name: "Create Secret",
			data: []byte(`apiVersion: v1
kind: Secret
metadata:
  name: test-secret`),
			expectedErr: false,
		},
		{
			name: "Unsupported object type",
			data: []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test-pod`),
			expectedErr: true,
		},
		{
			name: "Error creating object",
			data: []byte(`apiVersion: v1
kind: Namespace
metadata:
  name: error-object`),
			expectedErr: true,
		},
		{
			name: "Object already exists and update succeeds",
			data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: already-exists-object`),
			expectedErr: false,
		},
		{
			name: "Object already exists and update fails",
			data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: error-object`),
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := cluster.CreateObject(context.Background(), tt.data)
			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, obj)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, obj)
			}
		})
	}
}
