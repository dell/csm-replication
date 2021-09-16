/*
 Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package k8s_test

import (
	"context"
	"github.com/dell/csm-replication/api/v1alpha1"
	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/dell/repctl/pkg/types"
	"github.com/stretchr/testify/suite"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type ClusterTestSuite struct {
	suite.Suite
	cluster    k8s.ClusterInterface
	fakeClient k8s.ClientInterface
}

// blank assignment to verify client.Client method implementations
var _ client.Client = &fake_client.Client{}

func (suite *ClusterTestSuite) SetupSuite() {
	metadata.Init("replication.storage.dell.com")
	suite.cluster = &k8s.Cluster{}
	_ = v1alpha1.AddToScheme(scheme.Scheme)
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

	data, err := ioutil.ReadAll(file)
	suite.NoError(err)

	_, err = suite.cluster.CreateObject(context.Background(), data)
	suite.NoError(err)

	namespace, err := suite.cluster.GetNamespace(context.Background(), "test-ns")
	suite.NoError(err)
	suite.NotNil(namespace)
	suite.Equal(namespace.Name, "test-ns")
}

func (suite *ClusterTestSuite) TestListReplicationGroups() {
	rg1 := &v1alpha1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-rg-1",
		},
	}

	rg2 := &v1alpha1.DellCSIReplicationGroup{
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

	rg1 := &v1alpha1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-rg-1",
			Labels:      labelsAndAnnotationsA,
			Annotations: labelsAndAnnotationsA,
		},
	}

	labelsAndAnnotationsB := make(map[string]string)
	labelsAndAnnotationsB[metadata.RemoteClusterID] = "cluster-B"

	rg2 := &v1alpha1.DellCSIReplicationGroup{
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
	suite.Equal("cluster-B", rgList.RGList[0].RemoteClusterId)
}

func (suite *ClusterTestSuite) TestGetAllClusters() {
	suite.Run("failed to get any config files", func() {
		mc := k8s.MultiClusterConfigurator{}
		_, err := mc.GetAllClusters([]string{"cluster-1"}, "testdata/")
		suite.Error(err)
		suite.Contains(err.Error(), "failed to find any valid config files")
	})
	// TODO: write success case if you can figure out how to mock client created from file
}

func TestClusterTestSuite(t *testing.T) {
	suite.Run(t, new(ClusterTestSuite))
}
