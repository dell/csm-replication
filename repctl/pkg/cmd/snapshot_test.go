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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/cmd/mocks"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	repMock "github.com/dell/repctl/mocks"
)

type SnapshotTestSuite struct {
	suite.Suite
	testDataFolder string
}

func (suite *SnapshotTestSuite) SetupSuite() {
	curUser, err := os.UserHomeDir()
	suite.NoError(err)

	curUser = filepath.Join(curUser, folderPath)
	curUserPath, err := filepath.Abs(curUser)
	suite.NoError(err)

	suite.testDataFolder = curUserPath
	_ = repv1.AddToScheme(scheme.Scheme)

	metadata.Init("replication.storage.dell.com")
}

func TestSnapshotTestSuite(t *testing.T) {
	suite.Run(t, new(SnapshotTestSuite))
}

func (suite *SnapshotTestSuite) TestGetSnapshotCommand() {
	tests := []struct {
		name                  string
		getClustersFolderPath func(string) (string, error)
		expectedError         bool
		//expectedOutputContains          string
		objects                   []runtime.Object
		getListReplicationGroups  func(cluster k8s.ClusterInterface, ctx context.Context) (*repv1.DellCSIReplicationGroupList, error)
		getRGAndClusterFromRGID   func(configFolder string, rgID string, filter string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error)
		getUpdateReplicationGroup func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error
	}{
		{
			name: "Successful snapshot creation",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			expectedError: false,
			objects: []runtime.Object{
				&repv1.DellCSIReplicationGroupList{
					Items: []repv1.DellCSIReplicationGroup{
						{ObjectMeta: metav1.ObjectMeta{Name: "rg"}},
					},
				},
			},
			getListReplicationGroups: func(cluster k8s.ClusterInterface, ctx context.Context) (*repv1.DellCSIReplicationGroupList, error) {
				return &repv1.DellCSIReplicationGroupList{
					Items: []repv1.DellCSIReplicationGroup{
						{ObjectMeta: metav1.ObjectMeta{Name: "rg"}},
					},
				}, nil
			},
			getRGAndClusterFromRGID: func(configFolder string, rgID string, filter string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster-1"}, &repv1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{Name: "rg"},
					Status: repv1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: repv1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
				return nil
			},
		},
	}

	for _, tt := range tests {
		suite.T().Run(tt.name, func(t *testing.T) {
			// Save original function references
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			originalGetListReplicationGroupsFunction := getListReplicationGroupsFunction
			originalGetRGAndClusterFromRGIDFunction := getRGAndClusterFromRGIDFunction
			originalGetUpdateReplicationGroupFunction := getUpdateReplicationGroupFunction

			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
				getListReplicationGroupsFunction = originalGetListReplicationGroupsFunction
				getRGAndClusterFromRGIDFunction = originalGetRGAndClusterFromRGIDFunction
				getUpdateReplicationGroupFunction = originalGetUpdateReplicationGroupFunction
			}()

			// Override with test-specific functions
			getClustersFolderPathFunction = tt.getClustersFolderPath
			getListReplicationGroupsFunction = tt.getListReplicationGroups
			getRGAndClusterFromRGIDFunction = tt.getRGAndClusterFromRGID
			getUpdateReplicationGroupFunction = tt.getUpdateReplicationGroup

			// Set Viper flags
			viper.Set("sn-namespace", "test-namespace")
			viper.Set("sn-class", "test-class")
			viper.Set("snapshot-wait", false)
			viper.Set(config.ReplicationGroup, "rg")
			viper.Set(config.ReplicationPrefix, "test-prefix")
			viper.Set(config.Verbose, false)

			// Fake client and mock clusters
			fake, _ := fake_client.NewFakeClient(tt.objects, nil, nil)
			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "cluster-1",
					},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().
				GetAllClusters(gomock.Any(), gomock.Any()).
				Times(1).
				Return(mockClusters, nil)

			// Setup the command
			snapshotCmd := GetSnapshotCommand(getClustersMock)
			snapshotCmd.Flags().Set("at", "cluster-1")
			snapshotCmd.Flags().Set("sn-namespace", "test-namespace")
			snapshotCmd.Flags().Set("sn-class", "test-class")
			snapshotCmd.Flags().Set("wait", fmt.Sprintf("%v", true))

			// Capture output
			rescueStdout := os.Stdout

			defer func() {
				os.Stdout = rescueStdout
			}()

			snapshotCmd.Run(nil, nil)
		})
	}
}

func (suite *SnapshotTestSuite) TestCreateSnapshot() {
	tests := []struct {
		name                      string
		configFolder              string
		rgName                    string
		prefix                    string
		snNamespace               string
		snClass                   string
		verbose                   bool
		wait                      bool
		getRGAndClusterFromRGID   func(string, string, string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error)
		getUpdateReplicationGroup func(k8s.ClusterInterface, context.Context, *repv1.DellCSIReplicationGroup) error
		getWaitForStateToUpdate   func(string, k8s.ClusterInterface, repv1.ReplicationLinkState) bool
	}{
		{
			name:         "successful snapshot creation",
			configFolder: "config",
			rgName:       "rg1",
			prefix:       "replication.storage.dell.com",
			snNamespace:  "namespace",
			snClass:      "class",
			verbose:      true,
			wait:         false,
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &repv1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg1",
					},
					Status: repv1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: repv1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
				return nil
			},
		},
		{
			name:         "successful snapshot creation with wait and getWaitForStateToUpdate is true",
			configFolder: "config",
			rgName:       "rg1",
			prefix:       "replication.storage.dell.com",
			snNamespace:  "namespace",
			snClass:      "class",
			verbose:      true,
			wait:         true,
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &repv1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg1",
					},
					Status: repv1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: repv1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
				return nil
			},
			getWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
				return true
			},
		},
		{
			name:         "snapshot creation with wait but getWaitForStateToUpdate is false",
			configFolder: "config",
			rgName:       "rg1",
			prefix:       "replication.storage.dell.com",
			snNamespace:  "namespace",
			snClass:      "class",
			verbose:      true,
			wait:         true,
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &repv1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg1",
					},
					Status: repv1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: repv1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
				return nil
			},
			getWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
				return false
			},
		},
		{
			name:         "snapshot failed to get RG and cluster from rgID",
			configFolder: "config",
			rgName:       "rg1",
			prefix:       "replication.storage.dell.com",
			snNamespace:  "namespace",
			snClass:      "class",
			verbose:      true,
			wait:         false,
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
				return nil, nil, fmt.Errorf("failed to get RG and cluster from rgID")
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
				return nil
			},
			getWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
				return true
			},
		},

		{
			name:         "snapshot failed: link state is nil",
			configFolder: "config",
			rgName:       "rg1",
			prefix:       "replication.storage.dell.com",
			snNamespace:  "namespace",
			snClass:      "class",
			verbose:      true,
			wait:         false,
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &repv1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg1",
					},
					Status: repv1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: repv1.ReplicationLinkState{},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
				return nil
			},
			getWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
				return true
			},
		},
		{
			name:         "snapshot failed: snapshotclass is missing",
			configFolder: "config",
			rgName:       "rg1",
			prefix:       "replication.storage.dell.com",
			snNamespace:  "namespace",
			snClass:      "",
			verbose:      true,
			wait:         false,
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &repv1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg1",
					},
					Status: repv1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: repv1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
				return nil
			},
			getWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
				return true
			},
		},
		{
			name:         "snapshot failed: unable to update replication group",
			configFolder: "config",
			rgName:       "rg1",
			prefix:       "replication.storage.dell.com",
			snNamespace:  "namespace",
			snClass:      "myClass",
			verbose:      true,
			wait:         false,
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &repv1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg1",
					},
					Status: repv1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: repv1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
				return fmt.Errorf("error")
			},
			getWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
				return true
			},
		},
	}

	for _, tt := range tests {
		suite.T().Run(tt.name, func(t *testing.T) {
			// Save original function references
			originalGetRGAndClusterFromRGIDFunction := getRGAndClusterFromRGIDFunction
			originalGetUpdateReplicationGroupFunction := getUpdateReplicationGroupFunction
			originalGetWaitForStateToUpdateFunction := getWaitForStateToUpdateFunction

			defer func() {
				getRGAndClusterFromRGIDFunction = originalGetRGAndClusterFromRGIDFunction
				getUpdateReplicationGroupFunction = originalGetUpdateReplicationGroupFunction
				getWaitForStateToUpdateFunction = originalGetWaitForStateToUpdateFunction
			}()

			// Inject test-specific behavior
			getRGAndClusterFromRGIDFunction = tt.getRGAndClusterFromRGID
			getUpdateReplicationGroupFunction = tt.getUpdateReplicationGroup
			getWaitForStateToUpdateFunction = tt.getWaitForStateToUpdate

			// Call the function under test
			createSnapshot(
				tt.configFolder,
				tt.rgName,
				tt.prefix,
				tt.snNamespace,
				tt.snClass,
				tt.verbose,
				tt.wait,
			)
		})
	}
}

func (suite *SnapshotTestSuite) TestVerifyInputForSnapshotAction() {
	rgName := "myRG"
	suite.T().Run("fail: unable to get replication groups", func(t *testing.T) {
		originalClusterPath := clusterPath
		defer func() {
			clusterPath = originalClusterPath
		}()

		clusterPath = "testdata"

		originalGetClustersFolderPathFunction := getClustersFolderPathFunction
		defer func() {
			getClustersFolderPathFunction = originalGetClustersFolderPathFunction
		}()

		getClustersFolderPathFunction = func(path string) (string, error) {
			return path, nil
		}

		res, tgt := verifyInputForSnapshotAction(&k8s.MultiClusterConfigurator{}, "", rgName)
		assert.Empty(t, res)
		assert.Empty(t, tgt)
	})

	suite.T().Run("success: rgList input matches", func(t *testing.T) {
		originalClusterPath := clusterPath
		defer func() {
			clusterPath = originalClusterPath
		}()

		clusterPath = "testdata"

		originalGetClustersFolderPathFunction := getClustersFolderPathFunction
		defer func() {
			getClustersFolderPathFunction = originalGetClustersFolderPathFunction
		}()

		getClustersFolderPathFunction = func(path string) (string, error) {
			return path, nil
		}

		repList := &repv1.DellCSIReplicationGroupList{
			Items: []repv1.DellCSIReplicationGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "myRG",
					},
				},
			},
		}

		mockClient := repMock.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *repList)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		res, tgt := verifyInputForSnapshotAction(&k8s.MultiClusterConfigurator{}, "", rgName)
		assert.Equal(t, res, "rg")
		assert.Equal(t, tgt, "myRG")
	})
}
