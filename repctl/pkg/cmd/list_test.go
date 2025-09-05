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
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/cmd/mocks"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	repv1 "github.com/dell/csm-replication/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
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
	_ = repv1.AddToScheme(scheme.Scheme)

	metadata.Init("replication.storage.dell.com")
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

func TestGetListClusterGlobalCommandExitClustersFolderPath(t *testing.T) {

	if os.Getenv("INVOKE_ERROR_EXIT") == "1" {
		originalGetClustersFolderPathFunction := getClustersFolderPathFunction
		defer func() {
			getClustersFolderPathFunction = originalGetClustersFolderPathFunction
		}()
		getClustersFolderPathFunction = func(path string) (string, error) {
			return "", errors.New("error getting clusters folder path")
		}
		cmd := getListClusterGlobalCommand(nil)
		cmd.Run(nil, nil)
		return
	}

	// call the test again with INVOKE_ERROR_EXIT=1 so the function is invoked and we can check the return code
	cmd := exec.Command(os.Args[0], "-test.run=TestGetListClusterGlobalCommandExitClustersFolderPath") // #nosec G204
	cmd.Env = append(os.Environ(), "INVOKE_ERROR_EXIT=1")

	stdout, err := cmd.StderrPipe()
	if err != nil {
		t.Error(err)
		return
	}

	if err := cmd.Start(); err != nil {
		t.Error(err)
	}

	buf := make([]byte, 1024)
	n, err := stdout.Read(buf)
	if err != nil {
		t.Error(err)
	}

	err = cmd.Wait()
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Error(err)
	}

	// Trim the warning message from the actual output
	actualMessage := string(buf[:n])

	// check the output is the message we logged in errorExit
	assert.Contains(t, actualMessage, "cluster list: error getting clusters folder path: error getting clusters folder path")
}

func TestGetListClusterGlobalCommandExitGetAllClusters(t *testing.T) {

	if os.Getenv("INVOKE_ERROR_EXIT") == "1" {
		originalGetClustersFolderPathFunction := getClustersFolderPathFunction
		defer func() {
			getClustersFolderPathFunction = originalGetClustersFolderPathFunction
		}()
		getClustersFolderPathFunction = func(path string) (string, error) {
			return "folder", nil
		}
		getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
		getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("error calling GetAllClusters"))
		cmd := getListClusterGlobalCommand(getClustersMock)
		cmd.Run(nil, nil)
		return
	}

	// call the test again with INVOKE_ERROR_EXIT=1 so the function is invoked and we can check the return code
	cmd := exec.Command(os.Args[0], "-test.run=TestGetListClusterGlobalCommandExitGetAllClusters") // #nosec G204
	cmd.Env = append(os.Environ(), "INVOKE_ERROR_EXIT=1")

	stdout, err := cmd.StderrPipe()
	if err != nil {
		t.Error(err)
		return
	}

	if err := cmd.Start(); err != nil {
		t.Error(err)
	}

	buf := make([]byte, 1024)
	n, err := stdout.Read(buf)
	if err != nil {
		t.Error(err)
	}

	err = cmd.Wait()
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Error(err)
	}

	// Trim the warning message from the actual output
	actualMessage := string(buf[:n])

	// check the output is the message we logged in errorExit
	assert.Contains(t, actualMessage, "cluster list: error in initializing cluster info: error calling GetAllClusters")
}

func TestListTestSuite(t *testing.T) {
	suite.Run(t, new(ListTestSuite))
}

func (suite *ListTestSuite) TestGetListPersistentVolumesCommand() {
	tests := []struct {
		name                      string
		getClustersFolderPath     func(string) (string, error)
		objects                   []runtime.Object
		filter                    bool
		driverFilter              string
		expectedOutputContains    []string
		expectedOutputNotContains []string
	}{
		{
			name: "Successful with no filter",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			objects: []runtime.Object{
				&v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pv",
					},
				},
			},
			filter:                 false,
			expectedOutputContains: []string{"test-pv"},
		},
		{
			name: "Successful with filter",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			objects: []runtime.Object{
				&v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pv-1",
						Labels: map[string]string{
							"replication.storage.dell.com/driverName": "driver-1",
						},
					},
				},
				&v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pv-2",
						Labels: map[string]string{
							"replication.storage.dell.com/driverName": "driver-2",
						},
					},
				},
			},
			filter:                    true,
			driverFilter:              "driver-1",
			expectedOutputContains:    []string{"test-pv-1"},
			expectedOutputNotContains: []string{"test-pv-2"},
		},
	}

	for _, tt := range tests {
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			viper.Set("driver", tt.driverFilter)

			if tt.filter {
				viper.Set("all", "false")
			} else {
				viper.Set("all", "true")
			}
			defer viper.Reset()

			fake, _ := fake_client.NewFakeClient(tt.objects, nil, nil)

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

			cmd := getListPersistentVolumesCommand(getClustersMock)

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

			for _, notExpected := range tt.expectedOutputNotContains {
				assert.NotContains(t, string(out), notExpected)
			}
		})
	}
}

func (suite *ListTestSuite) TestGetListStorageClassesCommand() {
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
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			viper.Set("all", "true")
			defer viper.Reset()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			storageClass := &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					"isReplicationEnabled": "true",
				},
			}

			fake, _ := fake_client.NewFakeClient([]runtime.Object{storageClass}, nil, nil)

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

func (suite *ListTestSuite) TestGetListPersistentVolumeClaimsCommand() {
	storageClassName := "test-sc"

	tests := []struct {
		name                      string
		getClustersFolderPath     func(string) (string, error)
		objects                   []runtime.Object
		filter                    bool
		namespaceFilter           string
		expectedOutputContains    []string
		expectedOutputNotContains []string
	}{
		{
			name: "Successful with filter",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			objects: []runtime.Object{
				&v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc-1",
						Namespace: "test-ns-1",
					},
					Spec: v1.PersistentVolumeClaimSpec{
						StorageClassName: &storageClassName,
					},
				},
				&v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc-2",
						Namespace: "test-ns-2",
					},
					Spec: v1.PersistentVolumeClaimSpec{
						StorageClassName: &storageClassName,
					},
				},
			},
			filter:                    true,
			namespaceFilter:           "test-ns-1",
			expectedOutputContains:    []string{"test-pvc-1"},
			expectedOutputNotContains: []string{"test-pvc-2"},
		},
		{
			name: "Successful with no filter",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			objects: []runtime.Object{
				&v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc-1",
						Namespace: "test-ns-1",
					},
					Spec: v1.PersistentVolumeClaimSpec{
						StorageClassName: &storageClassName,
					},
				},
				&v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc-2",
						Namespace: "test-ns-2",
					},
					Spec: v1.PersistentVolumeClaimSpec{
						StorageClassName: &storageClassName,
					},
				},
			},
			filter:                 false,
			expectedOutputContains: []string{"test-pvc-1", "test-pvc-2"},
		},
	}

	for _, tt := range tests {
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			viper.Set("namespace", tt.namespaceFilter)

			if tt.filter {
				viper.Set("all", "false")
			} else {
				viper.Set("all", "true")
			}
			defer viper.Reset()

			fake, _ := fake_client.NewFakeClient(tt.objects, nil, nil)

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			cmd := getListPersistentVolumeClaimsCommand(getClustersMock)

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

			for _, notExpected := range tt.expectedOutputNotContains {
				assert.NotContains(t, string(out), notExpected)
			}
		})
	}
}

func (suite *ListTestSuite) TestGetListClusterGlobalCommand() {
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
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
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

func (suite *ListTestSuite) TestGetListReplicationGroupsCommand() {
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
			expectedOutputContains: "test-rg",
		},
	}

	for _, tt := range tests {
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			rg := &repv1.DellCSIReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-rg",
				},
			}

			fake, err := fake_client.NewFakeClient([]runtime.Object{rg}, nil, nil)
			assert.NoError(t, err)

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

			cmd := getListReplicationGroupsCommand(getClustersMock)

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
