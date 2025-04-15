/*
 Copyright © 2021-2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/mocks"
	cmdMocks "github.com/dell/repctl/pkg/cmd/mocks"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func (suite *ClusterTestSuite) TestGetClusterCommand() {
	cmd := GetClusterCommand()

	// Test the command usage
	suite.Equal("cluster", cmd.Use)
	suite.Equal("allows to manipulate cluster configs", cmd.Short)

	subCommands := cmd.Commands()
	suite.NotEmpty(subCommands)
	for _, subCmd := range subCommands {
		suite.NotNil(subCmd)
	}

	cmd.Run(nil, []string{"test"})
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

func (suite *ClusterTestSuite) TestAddClusterCommand() {

	tests := []struct {
		name                  string
		getClustersFolderPath func(string) (string, error)
		expectedOutputEquals  string
	}{
		{
			name: "Successful",
			getClustersFolderPath: func(path string) (string, error) {
				filePath, err := os.Getwd()
				if err != nil {
					return "", err
				}
				filePath += "/testdata"

				return filePath, nil
			},
			expectedOutputEquals: "",
		},
	}

	for _, tt := range tests {
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			defer viper.Reset()

			getClustersMock := cmdMocks.NewMockGetClustersInterface(gomock.NewController(t))

			filePath, err := os.Getwd()
			assert.Nil(t, err)
			filePath += "/testdata/config"

			viper.Set("files", filePath)
			viper.Set("add-name", "test-cluster-1")

			cmd := getAddClusterCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			cmd.Run(nil, nil)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Equal(t, string(out), tt.expectedOutputEquals)
		})
	}
}

func (suite *ClusterTestSuite) TestRemoveClusterCommand() {

	tests := []struct {
		name                  string
		getClustersFolderPath func(string) (string, error)
		expectedOutputEquals  string
	}{
		{
			name: "Successful",
			getClustersFolderPath: func(path string) (string, error) {
				filePath, err := os.Getwd()
				if err != nil {
					return "", err
				}
				filePath += "/testdata"

				return filePath, nil
			},
			expectedOutputEquals: "",
		},
	}

	for _, tt := range tests {
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			defer viper.Reset()

			filePath, err := os.Getwd()
			assert.Nil(t, err)
			filePath += "/testdata/remove-cluster-sample"

			_, err = os.Create(filePath)
			assert.Nil(t, err)

			viper.Set("remove-name", "remove-cluster-sample")

			cmd := getRemoveClusterCommand()

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			cmd.Run(nil, nil)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Equal(t, string(out), tt.expectedOutputEquals)
		})
	}
}

func (suite *ListTestSuite) TestGetListClustersCommand() {
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		clusters               *k8s.Clusters
		expectedOutputContains []string
	}{
		{
			name: "Successful",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			clusters: &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "cluster-1",
					},
					&k8s.Cluster{
						ClusterID: "cluster-2",
					},
				},
			},
			expectedOutputContains: []string{"cluster-1", "cluster-2"},
		},
	}

	for _, tt := range tests {
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()
			getClustersFolderPathFunction = tt.getClustersFolderPath

			getClustersMock := cmdMocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(tt.clusters, nil)

			cmd := getListClusterCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			cmd.Run(nil, nil)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			for _, expected := range tt.expectedOutputContains {
				assert.Contains(t, string(out), expected)
			}
		})
	}
}

func TestGenerateConfigsFromSA(t *testing.T) {
	tests := []struct {
		name    string
		objects []runtime.Object
	}{
		{
			name: "Successful",
			objects: []runtime.Object{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dell-replication-controller",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			RunCommand = func(_ *exec.Cmd) error {
				return nil
			}

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "cluster-1",
					},
					&k8s.Cluster{
						ClusterID: "cluster-2",
					},
				},
			}
			fake, _ := fake_client.NewFakeClient(tt.objects, nil, nil)

			getClustersMock := cmdMocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			mockClusters.Clusters[0].SetClient(fake)
			mockClusters.Clusters[1].SetClient(fake)

			_, err := generateConfigsFromSA(getClustersMock, []string{"cluster-1", "cluster-2"})
			assert.Nil(t, err)
		})
	}
}
