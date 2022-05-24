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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dell/repctl/mocks"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const folderPath = ".repctl/testdata/"

type ClusterTestSuite struct {
	suite.Suite
	testDataFolder string
}

func (suite *ClusterTestSuite) SetupSuite() {
	curUser, err := os.UserHomeDir()
	suite.NoError(err)

	curUser = filepath.Join(curUser, folderPath)
	curUserPath, err := filepath.Abs(curUser)
	suite.NoError(err)

	suite.testDataFolder = curUserPath
}

func (suite *ClusterTestSuite) TearDownSuite() {
	err := os.RemoveAll(suite.testDataFolder)
	suite.NoError(err)
}

func (suite *ClusterTestSuite) TestAddAndRemoveCluster() {
	path := filepath.Join(folderPath, "clusters")

	err := addCluster([]string{"testdata/config"}, []string{"test-cluster"}, path, false)
	suite.NoError(err)
	suite.FileExists(filepath.Join(suite.testDataFolder, "clusters", "test-cluster"))

	err = removeCluster("test-cluster", path)
	suite.NoError(err)
	suite.NoFileExists(filepath.Join(suite.testDataFolder, "clusters", "test-cluster"))
}

func (suite *ClusterTestSuite) TestInjectCluster() {
	path := filepath.Join(folderPath, "clusters")

	mockClient := new(mocks.ClientInterface)
	mockClient.On("Create", mock.Anything, mock.Anything).Return(nil)
	mockClient.On("Update", mock.Anything, mock.Anything).Return(nil)

	clusterIDs := []string{"test-cluster"}

	mockClusterA := new(mocks.ClusterInterface)
	mockClusterA.On("GetID").Return("cluster-A")
	mockClusterA.On("GetHost").Return("192.168.0.1")
	mockClusterA.On("GetKubeConfigFile").Return("testdata/config")
	mockClusterA.
		On("GetNamespace", mock.Anything, "dell-replication-controller").
		Return(nil, errors.New("namespace not found")).Once()
	mockClusterA.
		On("CreateNamespace", mock.Anything, mock.Anything).Return(nil).Once()
	mockClusterA.On("GetClient").Return(mockClient)

	mockClusterB := new(mocks.ClusterInterface)
	mockClusterB.On("GetID").Return("cluster-B")
	mockClusterB.On("GetHost").Return("192.168.0.2")
	mockClusterB.On("GetKubeConfigFile").Return("testdata/config")
	mockClusterB.
		On("GetNamespace", mock.Anything, "dell-replication-controller").
		Return(nil, errors.New("namespace not found")).Once()
	mockClusterB.
		On("CreateNamespace", mock.Anything, mock.Anything).Return(nil).Once()
	mockClusterB.On("GetClient").Return(mockClient)

	clusters := &k8s.Clusters{
		Clusters: []k8s.ClusterInterface{mockClusterA, mockClusterB},
	}

	mcMock := new(mocks.MultiClusterConfiguratorInterface)
	mcMock.On("GetAllClusters", clusterIDs, filepath.Join(suite.testDataFolder, "clusters")).Return(clusters, nil)

	err := injectCluster(mcMock, clusterIDs, path)
	suite.NoError(err)
}

func TestClusterTestSuite(t *testing.T) {
	suite.Run(t, new(ClusterTestSuite))
}
