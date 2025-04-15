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
	v1 "github.com/dell/csm-replication/api/v1"
	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/cmd/mocks"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
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
		objects                         []runtime.Object
		getListReplicationGroups        func(cluster k8s.ClusterInterface, ctx context.Context) (*v1.DellCSIReplicationGroupList, error)
		getVerifyInputForSnapshotAction func(mc GetClustersInterface, input string, rg string) (res string, tgt string)
		getRGAndClusterFromRGID         func(configFolder string, rgID string, filter string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error)
		getUpdateReplicationGroup       func(cluster k8s.ClusterInterface, ctx context.Context, rg *v1.DellCSIReplicationGroup) error
	}{
		{
			name: "Successful snapshot creation",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			expectedError: false,
			//expectedOutputContains: "Snapshot created successfully",
			objects: []runtime.Object{
				&repv1.DellCSIReplicationGroupList{
					Items: []v1.DellCSIReplicationGroup{
						{ObjectMeta: metav1.ObjectMeta{Name: "rg"}},
					},
				},
			},
			getListReplicationGroups: func(cluster k8s.ClusterInterface, ctx context.Context) (*v1.DellCSIReplicationGroupList, error) {
				return &v1.DellCSIReplicationGroupList{
					Items: []v1.DellCSIReplicationGroup{
						{ObjectMeta: metav1.ObjectMeta{Name: "rg"}},
					},
				}, nil
			},
			getVerifyInputForSnapshotAction: func(mc GetClustersInterface, input string, rg string) (res string, tgt string) {
				return "", ""
			},
			getRGAndClusterFromRGID: func(configFolder string, rgID string, filter string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster-1"}, &v1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{Name: "rg"},
					Status: v1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: v1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *v1.DellCSIReplicationGroup) error {
				return nil
			},
		},
		// {
		// 	name: "Error getting clusters folder path",
		// 	getClustersFolderPath: func(path string) (string, error) {
		// 		return "", errors.New("error")
		// 	},
		// 	expectedError:          true,
		// 	expectedOutputContains: "error getting clusters folder path",
		// 	objects: []runtime.Object{
		// 		&v1.DellCSIReplicationGroupList{
		// 			Items: []v1.DellCSIReplicationGroup{
		// 				{ObjectMeta: metav1.ObjectMeta{Name: "rg"}},
		// 			},
		// 		},
		// 	},
		// 	getListReplicationGroups: func(cluster k8s.ClusterInterface, ctx context.Context) (*v1.DellCSIReplicationGroupList, error) {
		// 		return &v1.DellCSIReplicationGroupList{
		// 			Items: []v1.DellCSIReplicationGroup{
		// 				{ObjectMeta: metav1.ObjectMeta{Name: "rg"}},
		// 			},
		// 		}, nil
		// 	},
		// 	getVerifyInputForSnapshotAction: func(mc GetClustersInterface, input string, rg string) (res string, tgt string) {
		// 		return "", ""
		// 	},
		// 	getRGAndClusterFromRGID: func(configFolder string, rgID string, filter string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error) {
		// 		return nil, nil, fmt.Errorf("error fetching RG info")
		// 	},
		// 	getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *v1.DellCSIReplicationGroup) error {
		// 		return nil
		// 	},
		// },
	}

	for _, tt := range tests {
		suite.T().Run(tt.name, func(t *testing.T) {
			// Save original function references
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			originalGetListReplicationGroupsFunction := getListReplicationGroupsFunction
			originalGetVerifyInputForSnapshotActionFunction := getVerifyInputForSnapshotActionFunction
			originalGetRGAndClusterFromRGIDFunction := getRGAndClusterFromRGIDFunction
			originalGetUpdateReplicationGroupFunction := getUpdateReplicationGroupFunction

			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
				getListReplicationGroupsFunction = originalGetListReplicationGroupsFunction
				getVerifyInputForSnapshotActionFunction = originalGetVerifyInputForSnapshotActionFunction
				getRGAndClusterFromRGIDFunction = originalGetRGAndClusterFromRGIDFunction
				getUpdateReplicationGroupFunction = originalGetUpdateReplicationGroupFunction
			}()

			// Override with test-specific functions
			getClustersFolderPathFunction = tt.getClustersFolderPath
			getListReplicationGroupsFunction = tt.getListReplicationGroups
			getVerifyInputForSnapshotActionFunction = tt.getVerifyInputForSnapshotAction
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
			fake, _ := fake_client.NewFakeClient(tt.objects, nil)
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
			//r, w, _ := os.Pipe()
			//os.Stdout = w

			defer func() {
				os.Stdout = rescueStdout
			}()

			snapshotCmd.Run(nil, nil)
			//w.Close()
			// out, _ := io.ReadAll(r)
			// os.Stdout = rescueStdout

			// Assertions
			// if tt.expectedError {
			// 	//assert.Contains(t, string(out), tt.expectedOutputContains)
			// } else {
			// 	assert.Nil(t, err)
			// }
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
		getRGAndClusterFromRGID   func(string, string, string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error)
		getUpdateReplicationGroup func(k8s.ClusterInterface, context.Context, *v1.DellCSIReplicationGroup) error
		expectedError             string
		getWaitForStateToUpdate   func(string, k8s.ClusterInterface, v1.ReplicationLinkState) bool
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
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &v1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "rg1",
						Annotations: map[string]string{},
					},
					Status: v1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: v1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *v1.DellCSIReplicationGroup) error {
				return nil
			},
			expectedError: "",
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
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &v1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "rg1",
						Annotations: map[string]string{},
					},
					Status: v1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: v1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *v1.DellCSIReplicationGroup) error {
				return nil
			},
			getWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState v1.ReplicationLinkState) bool {
				return true
			},
			expectedError: "",
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
			getRGAndClusterFromRGID: func(configFolder, rgName, src string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster1"}, &v1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "rg1",
						Annotations: map[string]string{},
					},
					Status: v1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: v1.ReplicationLinkState{
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			getUpdateReplicationGroup: func(cluster k8s.ClusterInterface, ctx context.Context, rg *v1.DellCSIReplicationGroup) error {
				return nil
			},
			getWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState v1.ReplicationLinkState) bool {
				return false
			},
			expectedError: "",
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

			// Optional: Add assertions here if you capture logs or output
			if tt.expectedError != "" {
				assert.Contains(t, tt.expectedError, "error fetching RG info")
			}
		})
	}
}
