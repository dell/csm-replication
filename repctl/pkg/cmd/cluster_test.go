/*
 *
 * Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

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

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	"k8s.io/apimachinery/pkg/util/validation"
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
	tests := []struct {
		name                          string
		clusters                      *k8s.Clusters
		createCluster                 k8s.ClusterInterface
		clusterIDs                    []string
		path                          string
		customConfigs                 []string
		getAllClustersError           error
		createClusterError            error
		expectError                   bool
		getClustersFolderPathFunction func(_ string) (string, error)
	}{
		{
			name: "Successful",
			clusters: func() *k8s.Clusters {
				mockClient := new(mocks.ClientInterface)
				mockClient.On("Create", mock.Anything, mock.Anything).Return(nil)
				mockClient.On("Update", mock.Anything, mock.Anything).Return(nil)

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
				return clusters
			}(),
			clusterIDs:  []string{"test-cluster"},
			path:        filepath.Join(folderPath, "clusters"),
			expectError: false,
		},
		{
			name: "Successful with customConfigs",
			clusters: func() *k8s.Clusters {
				mockClient := new(mocks.ClientInterface)
				mockClient.On("Create", mock.Anything, mock.Anything).Return(nil)
				mockClient.On("Update", mock.Anything, mock.Anything).Return(nil)

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
				return clusters
			}(),
			createCluster: func() k8s.ClusterInterface {
				mockClient := new(mocks.ClientInterface)
				mockClient.On("Create", mock.Anything, mock.Anything).Return(nil)
				mockClient.On("Update", mock.Anything, mock.Anything).Return(nil)

				mockClusterA := new(mocks.ClusterInterface)
				mockClusterA.On("GetID").Return("cluster-C")
				mockClusterA.On("GetHost").Return("192.168.0.3")
				mockClusterA.On("GetKubeConfigFile").Return("testdata/config")
				mockClusterA.
					On("GetNamespace", mock.Anything, "dell-replication-controller").
					Return(nil, errors.New("namespace not found")).Once()
				mockClusterA.
					On("CreateNamespace", mock.Anything, mock.Anything).Return(nil).Once()
				mockClusterA.On("GetClient").Return(mockClient)
				return mockClusterA
			}(),
			clusterIDs:    []string{"test-cluster"},
			path:          filepath.Join(folderPath, "clusters"),
			customConfigs: []string{"cluster-c"},
			expectError:   false,
		},
		{
			name: "Successful when creating ConfigMap that already exists",
			clusters: func() *k8s.Clusters {
				mockClient := new(mocks.ClientInterface)

				mockClient.On("Create", mock.Anything, mock.Anything).Return(errors.New("ConfigMap already exists"))
				mockClient.On("Update", mock.Anything, mock.Anything).Return(nil)
				mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil)

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

				clusters := &k8s.Clusters{
					Clusters: []k8s.ClusterInterface{mockClusterA},
				}
				return clusters
			}(),
			clusterIDs:  []string{"test-cluster"},
			path:        filepath.Join(folderPath, "clusters"),
			expectError: false,
		},
		{
			name: "Error due to incorrect path",
			getClustersFolderPathFunction: func(path string) (string, error) {
				return "", errors.New("error")
			},
			expectError: true,
		},
		{
			name:                "Error calling GetAllClusters",
			getAllClustersError: errors.New("error"),
			expectError:         true,
		},
	}

	for _, tt := range tests {
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			mcMock := new(mocks.MultiClusterConfiguratorInterface)
			mcMock.On("GetAllClusters", tt.clusterIDs, mock.Anything).Return(tt.clusters, tt.getAllClustersError)

			oldGetFileName := GetFileName
			defer func() { GetFileName = oldGetFileName }()
			GetFileName = func(_ string) (string, error) {
				return "name", nil
			}

			oldGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() { getClustersFolderPathFunction = oldGetClustersFolderPathFunction }()
			if tt.getClustersFolderPathFunction != nil {
				getClustersFolderPathFunction = tt.getClustersFolderPathFunction
			}

			CreateCluster = func(_ string, _ string) (k8s.ClusterInterface, error) {
				return tt.createCluster, tt.createClusterError
			}

			err := injectCluster(mcMock, tt.clusterIDs, tt.path, tt.customConfigs...)
			if tt.expectError {
				suite.Error(err)
			} else {
				suite.NoError(err)
			}
		})
	}
}

func (suite *ClusterTestSuite) TestGetInjectClustersCommand() {
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
		Return(nil, errors.New("namespace not found"))
	mockClusterA.
		On("CreateNamespace", mock.Anything, mock.Anything).Return(nil)
	mockClusterA.On("GetClient").Return(mockClient)

	mockClusterB := new(mocks.ClusterInterface)
	mockClusterB.On("GetID").Return("cluster-B")
	mockClusterB.On("GetHost").Return("192.168.0.2")
	mockClusterB.On("GetKubeConfigFile").Return("testdata/config")
	mockClusterB.
		On("GetNamespace", mock.Anything, "dell-replication-controller").
		Return(nil, errors.New("namespace not found"))
	mockClusterB.
		On("CreateNamespace", mock.Anything, mock.Anything).Return(nil)
	mockClusterB.On("GetClient").Return(mockClient)

	clusters := &k8s.Clusters{
		Clusters: []k8s.ClusterInterface{mockClusterA, mockClusterB},
	}

	path := filepath.Join(folderPath, "clusters")

	mcMock := new(mocks.MultiClusterConfiguratorInterface)
	mcMock.On("GetAllClusters", clusterIDs, path).Return(clusters, nil)

	originalGetClustersFolderPathFunction := getClustersFolderPathFunction
	defer func() {
		getClustersFolderPathFunction = originalGetClustersFolderPathFunction
	}()
	getClustersFolderPathFunction = func(_ string) (string, error) {
		return path, nil
	}
	RunCommand = func(_ *exec.Cmd) error {
		return nil
	}
	viper.Set("clusters", clusterIDs)
	viper.Set("use-sa", "true")
	defer viper.Reset()
	cmd := getInjectClustersCommand(mcMock)
	cmd.Run(nil, nil)
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

			clusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "cluster-1",
					},
				},
			}
			getClustersMock := cmdMocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(clusters, nil)

			fake, _ := fake_client.NewFakeClient(nil, nil, nil)
			clusters.Clusters[0].SetClient(fake)

			filePath, err := os.Getwd()
			assert.Nil(t, err)
			filePath += "/testdata/config"

			viper.Set("files", filePath)
			viper.Set("add-name", "test-cluster-1")
			viper.Set("auto-inject", "true")
			viper.Set("clusters", "cluster-1")

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
		name                string
		objects             []runtime.Object
		clusters            *k8s.Clusters
		getAllClustersError error
		runCommandError     error
		expectedError       error
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
		},
		{
			name:                "Error calling GetAllClusters",
			getAllClustersError: errors.New("error"),
			expectedError:       errors.New("error in initializing cluster info: error"),
		},
		{
			name: "Success even with error from RunCommand",
			objects: []runtime.Object{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dell-replication-controller",
					},
				},
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
			runCommandError: errors.New("error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RunCommand = func(_ *exec.Cmd) error {
				return tt.runCommandError
			}

			fake, _ := fake_client.NewFakeClient(tt.objects, nil, nil)

			getClustersMock := cmdMocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(tt.clusters, tt.getAllClustersError)

			if tt.clusters != nil {
				tt.clusters.Clusters[0].SetClient(fake)
				tt.clusters.Clusters[1].SetClient(fake)
			}

			_, err := generateConfigsFromSA(getClustersMock, []string{"cluster-1", "cluster-2"})
			if tt.expectedError == nil {
				assert.Nil(t, err)
			} else {
				assert.Equal(t, tt.expectedError.Error(), err.Error())
			}
		})
	}
}

func TestAddKeyFromLiteralToSecret(t *testing.T) {
	tests := []struct {
		secret      *v1.Secret
		keyName     string
		data        []byte
		expectedErr error
	}{
		{
			secret: &v1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{},
			},
			keyName:     "test-key",
			data:        []byte("test-value"),
			expectedErr: nil,
		},
		{
			secret: &v1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{"test-key": []byte("test-value")},
			},
			keyName:     "test-key",
			data:        []byte("test-value"),
			expectedErr: fmt.Errorf("cannot add key %s, another key by that name already exists", "test-key"),
		},
		{
			secret: &v1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{},
			},
			keyName:     "invalid key",
			data:        []byte("test-value"),
			expectedErr: fmt.Errorf("%q is not valid key name for a Secret %s", "invalid key", strings.Join(validation.IsConfigMapKey("invalid key"), ";")),
		},
	}

	for _, tt := range tests {
		err := addKeyFromLiteralToSecret(tt.secret, tt.keyName, tt.data)
		if tt.expectedErr == nil {
			assert.NoError(t, err)
		} else {
			assert.Equal(t, tt.expectedErr.Error(), err.Error())
		}
	}
}

func TestGetFileName(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		expectedErr error
	}{
		{
			input: func() string {
				filePath, err := os.Getwd()
				if err != nil {
					return ""
				}
				filePath += "/testdata/cluster-1"
				return filePath
			}(),
			expected:    "cluster-1",
			expectedErr: nil,
		},
		{
			input:       "missing-file",
			expected:    "",
			expectedErr: fmt.Errorf("stat missing-file: no such file or directory"),
		},
	}

	for _, tt := range tests {
		actual, err := GetFileName(tt.input)
		if tt.expectedErr == nil {
			assert.Nil(t, err)
		} else {
			assert.Equal(t, tt.expectedErr.Error(), err.Error())
		}
		if actual != tt.expected {
			t.Errorf("expected output %q, but got %q", tt.expected, actual)
		}
	}
}
