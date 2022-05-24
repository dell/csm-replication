/*
 Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package cmd

import (
	"testing"

	"github.com/dell/repctl/mocks"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CreateTestSuite struct {
	suite.Suite
}

func (suite *CreateTestSuite) TestCreatePVCs() {
	prefix := "replication.storage.dell.com"
	dryRun := true

	suite.Run("no pvs provided", func() {
		pvList := []string{}
		rgName := "test-rg"

		pvs := []types.PersistentVolume{{
			Name:   "test-pv-1",
			RGName: rgName,
			SCName: "test-sc-1",
		}}

		mockCluster := new(mocks.ClusterInterface)
		mockCluster.On("GetID").Return("cluster-1")
		mockCluster.On("FilterPersistentVolumes", mock.Anything, "", "", "", rgName).Return(pvs, nil)
		mockCluster.
			On("CreatePersistentVolumeClaimsFromPVs", mock.Anything, "test-ns", pvs, prefix, dryRun).
			Return(nil)

		err := createPVCs(pvList, mockCluster, rgName, "test-ns", prefix, dryRun)
		suite.NoError(err)
	})

	suite.Run("from provided pvs", func() {
		pv1 := types.PersistentVolume{
			Name:   "test-pv-1",
			SCName: "test-sc-1",
		}
		pv2 := types.PersistentVolume{
			Name:   "test-pv-1",
			SCName: "test-sc-1",
		}

		pvList := []string{"test-pv-1"}

		pvs := []types.PersistentVolume{pv1, pv2}

		mockCluster := new(mocks.ClusterInterface)
		mockCluster.On("GetID").Return("cluster-1")
		mockCluster.On("FilterPersistentVolumes", mock.Anything, "", "", "", "").Return(pvs, nil)
		mockCluster.
			On("CreatePersistentVolumeClaimsFromPVs", mock.Anything, "test-ns", pvs, prefix, dryRun).
			Return(nil)

		err := createPVCs(pvList, mockCluster, "", "test-ns", prefix, dryRun)
		suite.NoError(err)
	})
}

func (suite *CreateTestSuite) TestCreateSCs() {
	mockClusterA := new(mocks.ClusterInterface)
	mockClusterA.On("GetID").Return("cluster-A")
	mockClusterA.On("GetHost").Return("192.168.0.1")
	mockClusterA.On("GetKubeConfigFile").Return("testdata/config")
	mockClusterA.On("CreateObject", mock.Anything, mock.Anything).Return(&v1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "powerstore-replication",
		},
		Provisioner: "csi-powerstore.dellemc.com",
	}, nil)

	mockClusterB := new(mocks.ClusterInterface)
	mockClusterB.On("GetID").Return("cluster-B")
	mockClusterB.On("GetHost").Return("192.168.0.2")
	mockClusterB.On("GetKubeConfigFile").Return("testdata/config")
	mockClusterB.On("CreateObject", mock.Anything, mock.Anything).Return(&v1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "powerstore-replication",
		},
		Provisioner: "csi-powerstore.dellemc.com",
	}, nil)

	clusters := &k8s.Clusters{
		Clusters: []k8s.ClusterInterface{mockClusterA, mockClusterB},
	}

	config := ScConfig{
		Name:            "powerstore-replication",
		Driver:          "powerstore",
		ReclaimPolicy:   "Retain",
		SourceClusterID: "cluster-A",
		TargetClusterID: "cluster-B",
		Parameters: GlobalParameters{
			ArrayID: Mirrored{
				Source: "192.0.0.1",
				Target: "192.0.0.2",
			},
			RemoteSystem: Mirrored{
				Source: "WX-0001",
				Target: "WX-0002",
			},
			Rpo: "Five_Minutes",
		},
	}

	err := createSCs(config, clusters, false)
	suite.NoError(err)

}

func TestCreateTestSuite(t *testing.T) {
	suite.Run(t, new(CreateTestSuite))
}

func TestSplitFuncAtWithYAMLSeparator(t *testing.T) {
	f := splitFuncAt(yamlSeparator)
	testCases := []struct {
		input  string
		atEOF  bool
		expect string
		adv    int
	}{
		{"foo", true, "foo", 3},
		{"fo", false, "", 0},

		{"---", true, "---", 3},
		{"---\n", true, "---\n", 4},
		{"---\n", false, "", 0},

		{"\n---\n", false, "", 5},
		{"\n---\n", true, "", 5},

		{"abc\n---\ndef", true, "abc", 8},
		{"def", true, "def", 3},
		{"", true, "", 0},
	}
	for i, testCase := range testCases {
		adv, token, err := f([]byte(testCase.input), testCase.atEOF)
		if err != nil {
			t.Errorf("%d: unexpected error: %v", i, err)
			continue
		}
		if adv != testCase.adv {
			t.Errorf("%d: advance did not match: %d %d", i, testCase.adv, adv)
		}
		if testCase.expect != string(token) {
			t.Errorf("%d: token did not match: %q %q", i, testCase.expect, string(token))
		}
	}
}
