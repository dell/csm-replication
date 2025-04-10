/*
 Copyright © 2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"io"
	"os"
	"path/filepath"
	"testing"

	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/cmd/mocks"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

type ListTestSuite struct {
	suite.Suite
	testDataFolder string
}

func (suite *ListTestSuite) SetupSuite() {
	curUser, err := os.UserHomeDir()
	suite.NoError(err)

	curUser = filepath.Join(curUser, folderPath)
	curUserPath, err := filepath.Abs(curUser)
	suite.NoError(err)

	suite.testDataFolder = curUserPath
}

func (suite *ListTestSuite) TestGetListCommand() {
	cmd := GetListCommand()

	// Test the command usage
	suite.Equal("get", cmd.Use)
	suite.Equal("lists different resources in clusters with configured replication", cmd.Short)

	// Test the persistent flags
	allFlag := cmd.PersistentFlags().Lookup("all")
	suite.NotNil(allFlag)
	suite.Equal("show all objects (overrides other filters)", allFlag.Usage)

	rnFlag := cmd.PersistentFlags().Lookup("rn")
	suite.NotNil(rnFlag)
	suite.Equal("remote namespace", rnFlag.Usage)

	rcFlag := cmd.PersistentFlags().Lookup("rc")
	suite.NotNil(rcFlag)
	suite.Equal("remote cluster id", rcFlag.Usage)

	subCommands := cmd.Commands()
	suite.NotEmpty(subCommands)
	for _, subCmd := range subCommands {
		suite.NotNil(subCmd)
	}

	cmd.Run(nil, []string{"test"})
}

func TestListTestSuite(t *testing.T) {
	suite.Run(t, new(ListTestSuite))
}

func TestGetListPersistentVolumesCommand(t *testing.T) {
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		expectedOutputContains string
	}{
		{
			name: "Successful",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			expectedOutputContains: "test-pv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			persistentVolume := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv",
				},
			}

			fake, _ := fake_client.NewFakeClient([]runtime.Object{persistentVolume}, nil)

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "",
					},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			listPVCmd := getListPersistentVolumesCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			listPVCmd.Run(nil, nil)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Contains(t, string(out), tt.expectedOutputContains)
		})
	}
}

func TestGetListStorageClassesCommand(t *testing.T) {
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		expectedOutputContains string
	}{
		{
			name: "Successful",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			expectedOutputContains: "test-sc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			viper.Set("all", "true")
			getClustersFolderPathFunction = tt.getClustersFolderPath

			storageClass := &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					"isReplicationEnabled": "true",
				},
			}

			fake, _ := fake_client.NewFakeClient([]runtime.Object{storageClass}, nil)

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "",
					},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			listPVCmd := getListStorageClassesCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			listPVCmd.Run(nil, nil)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Contains(t, string(out), tt.expectedOutputContains)
		})
	}
}

func TestGetListPersistentVolumeClaimsCommand(t *testing.T) {
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		expectedOutputContains string
	}{
		{
			name: "Successful",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			expectedOutputContains: "test-pvc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			storageClassName := "test-sc"

			persistentVolumeClaim := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pvc",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClassName,
				},
			}

			fake, _ := fake_client.NewFakeClient([]runtime.Object{persistentVolumeClaim}, nil)

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "",
					},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			listPVCmd := getListPersistentVolumeClaimsCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			listPVCmd.Run(nil, nil)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Contains(t, string(out), tt.expectedOutputContains)
		})
	}
}

func TestGetListClusterGlobalCommand(t *testing.T) {
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		expectedOutputContains string
	}{
		{
			name: "Successful",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			expectedOutputContains: "cluster-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "cluster-id",
					},
				},
			}

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			cmd := getListClusterGlobalCommand(getClustersMock)

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

			assert.Contains(t, string(out), tt.expectedOutputContains)
		})
	}
}
