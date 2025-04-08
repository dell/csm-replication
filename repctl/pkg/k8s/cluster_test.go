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
	"context"
	"io"
	"os"
	"testing"

	repv1 "github.com/dell/csm-replication/api/v1"
	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/dell/repctl/pkg/types"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
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
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pv}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	volume, err := suite.cluster.GetPersistentVolume(context.Background(), "test-pv")
	suite.NoError(err)
	suite.NotNil(volume)
	suite.Equal("test-pv", volume.Name)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{pv1, pv2}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{pv1, pv2, repPV}, nil)
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
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{ns}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	gotNs, err := suite.cluster.GetNamespace(context.Background(), "test-ns")
	suite.NoError(err)
	suite.NotNil(gotNs)
	suite.Equal("test-ns", gotNs.Name)
}

func (suite *ClusterTestSuite) TestCreateNamespace() {
	fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{sc1, sc2}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{sc1, repSC}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{pvc1, pvc2}, nil)
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
	pvc1 := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc-1",
			Namespace: "test-ns",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &usualSC,
		},
	}

	pvc2 := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc-2",
			Namespace: "test-ns",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &usualSC,
		},
	}

	repSC := "replicated-sc"
	matchingLabels := make(map[string]string)
	matchingLabels[metadata.RemoteClusterID] = "cluster-2"

	repPVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "replicated-pvc",
			Namespace: "test-ns",
			Labels:    matchingLabels,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &repSC,
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pvc1, pvc2, repPVC}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	volumeList, err := suite.cluster.FilterPersistentVolumeClaims(context.Background(), "", "", "", "")
	suite.NoError(err)
	suite.NotNil(volumeList.PVCList)
	suite.Equal(3, len(volumeList.PVCList))
	var names []string
	for _, v := range volumeList.PVCList {
		names = append(names, v.Name)
	}
	suite.ElementsMatch(names, []string{"test-pvc-1", "test-pvc-2", "replicated-pvc"})

	replicatedVolumes, err := suite.cluster.FilterPersistentVolumeClaims(context.Background(), "test-ns", "cluster-2", "", "")
	suite.NoError(err)
	suite.NotNil(replicatedVolumes.PVCList)
	suite.Equal(1, len(replicatedVolumes.PVCList))
	suite.Equal("replicated-pvc", replicatedVolumes.PVCList[0].Name)
}

func (suite *ClusterTestSuite) TestCreatePVCsFromPVs() {
	matchingLabels := make(map[string]string)
	matchingLabels[metadata.RemoteClusterID] = "cluster-2"

	pv1 := types.PersistentVolume{
		Name:          "pv-1",
		RGName:        "rg-1",
		SCName:        "rep-sc",
		RemotePVCName: "pvc-1",
		ReclaimPolicy: "Delete",
		Labels:        matchingLabels,
	}

	pv2 := types.PersistentVolume{
		Name:          "pv-2",
		RGName:        "rg-1",
		SCName:        "rep-sc",
		RemotePVCName: "pvc-2",
		ReclaimPolicy: "Delete",
		Labels:        matchingLabels,
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	err = suite.cluster.CreatePersistentVolumeClaimsFromPVs(context.Background(), "remote-ns", []types.PersistentVolume{pv1, pv2}, "replication.storage.dell.com", false)
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
}

func (suite *ClusterTestSuite) TestCreateObject() {
	fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{rg1, rg2}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{rg1, rg2}, nil)
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
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{secret}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	foundSecret, err := suite.cluster.GetSecret(context.Background(), "test-namespace", "test-secret")
	suite.NoError(err)
	suite.NotNil(foundSecret)
	suite.Equal("test-secret", foundSecret.Name)
	suite.Equal("test-namespace", foundSecret.Namespace)
}

func (suite *ClusterTestSuite) TestGetPersistentVolumeClaim() {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "test-namespace",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pvc}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	foundPVC, err := suite.cluster.GetPersistentVolumeClaim(context.Background(), "test-namespace", "test-pvc")
	suite.NoError(err)
	suite.NotNil(foundPVC)
	suite.Equal("test-pvc", foundPVC.Name)
	suite.Equal("test-namespace", foundPVC.Namespace)
}

func (suite *ClusterTestSuite) TestGetReplicationGroups() {
	replicationGroup := &repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-rg",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{replicationGroup}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	foundRG, err := suite.cluster.GetReplicationGroups(context.Background(), "test-rg")
	suite.NoError(err)
	suite.NotNil(foundRG)
	suite.Equal("test-rg", foundRG.Name)
}

func (suite *ClusterTestSuite) TestGetStatefulSet() {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "test-namespace",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{statefulSet}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	foundSTS, err := suite.cluster.GetStatefulSet(context.Background(), "test-namespace", "test-sts")
	suite.NoError(err)
	suite.NotNil(foundSTS)
	suite.Equal("test-sts", foundSTS.Name)
	suite.Equal("test-namespace", foundSTS.Namespace)
}

func (suite *ClusterTestSuite) TestGetPod() {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pod}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	foundPod, err := suite.cluster.GetPod(context.Background(), "test-pod", "test-namespace")
	suite.NoError(err)
	suite.NotNil(foundPod)
	suite.Equal("test-pod", foundPod.Name)
	suite.Equal("test-namespace", foundPod.Namespace)
}

func (suite *ClusterTestSuite) TestGetMigrationGroup() {
	mg := &repv1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mg",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{mg}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	foundMG, err := suite.cluster.GetMigrationGroup(context.Background(), "test-mg")
	suite.NoError(err)
	suite.NotNil(foundMG)
	suite.Equal("test-mg", foundMG.Name)
}

func (suite *ClusterTestSuite) TestDeletePod() {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pod}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	err = suite.cluster.DeletePod(context.Background(), pod)
	suite.NoError(err)

	// Verify that the Pod has been deleted
	foundPod := &v1.Pod{}
	err = fake.Get(context.Background(), apiTypes.NamespacedName{Name: "test-pod", Namespace: "test-namespace"}, foundPod)
	suite.Error(err) // Expect an error since the Pod should be deleted
}

func (suite *ClusterTestSuite) TestDeletePersistentVolumeClaim() {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "test-namespace",
		},
	}

	fake, err := fake_client.NewFakeClient([]runtime.Object{pvc}, nil)
	suite.NoError(err)

	suite.cluster.SetClient(fake)

	err = suite.cluster.DeletePersistentVolumeClaim(context.Background(), pvc)
	suite.NoError(err)

	// Verify that the PersistentVolumeClaim has been deleted
	foundPVC := &v1.PersistentVolumeClaim{}
	err = fake.Get(context.Background(), apiTypes.NamespacedName{Name: "test-pvc", Namespace: "test-namespace"}, foundPVC)
	suite.Error(err) // Expect an error since the PersistentVolumeClaim should be deleted
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{mg}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{rg}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{secret}, nil)
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

	fake, err := fake_client.NewFakeClient([]runtime.Object{pv}, nil)
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
