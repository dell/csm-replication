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
	"errors"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/mocks"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetFailbackCommand(t *testing.T) {
	t.Run("fail: unexpected input", func(t *testing.T) {
		viper.Set("src", "cluster2")
		viper.Set("discard", false)
		viper.Set("failback-wait", true)
		viper.Set(config.ReplicationGroup, "rg-id")
		viper.Set(config.Verbose, true)
		defer func() {
			viper.Reset()
		}()

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
						Name: "rg-id",
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *repList)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetFailbackCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})
}

func TestFailbackToRG(t *testing.T) {
	origGetRGAndCluster := getRGAndClusterFromRGIDFunction
	origUpdateRG := getUpdateReplicationGroupFunction
	origWait := getWaitForStateToUpdateFunction
	originalFatalfLog := fatalfLog
	originalFatalLog := fatalLog
	exitCode := 0

	after := func() {
		getRGAndClusterFromRGIDFunction = origGetRGAndCluster
		getUpdateReplicationGroupFunction = origUpdateRG
		getWaitForStateToUpdateFunction = origWait
		fatalfLog = originalFatalfLog
		fatalLog = originalFatalLog
		exitCode = 0
	}

	type args struct {
		configFolder string
		rgName       string
		discard      bool
		verbose      bool
		wait         bool
	}

	tests := []struct {
		name             string
		args             args
		setup            func()
		expectedExitCode int
	}{
		{
			name: "success: planned failback without wait",
			args: args{
				configFolder: "config",
				rgName:       "rg-id",
				discard:      true,
				verbose:      true,
				wait:         false,
			},
			setup: func() {
				getRGAndClusterFromRGIDFunction = func(_, _, _ string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
					return &k8s.Cluster{ClusterID: "cluster-1"}, &repv1.DellCSIReplicationGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "rg-id",
						},
						Spec: repv1.DellCSIReplicationGroupSpec{},
						Status: repv1.DellCSIReplicationGroupStatus{
							ReplicationLinkState: repv1.ReplicationLinkState{
								IsSource:             true,
								LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
							},
						},
					}, nil
				}
				getUpdateReplicationGroupFunction = func(_ k8s.ClusterInterface, _ context.Context, _ *repv1.DellCSIReplicationGroup) error {
					return nil
				}
			},
			expectedExitCode: 0,
		},
		{
			name: "Timeout: timed out with action: failover",
			args: args{
				configFolder: "config",
				rgName:       "rg-id",
				discard:      true,
				verbose:      true,
				wait:         true,
			},
			setup: func() {
				getRGAndClusterFromRGIDFunction = func(_, _, _ string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
					return &k8s.Cluster{ClusterID: "cluster-1"}, &repv1.DellCSIReplicationGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "rg-id",
						},
						Spec: repv1.DellCSIReplicationGroupSpec{},
						Status: repv1.DellCSIReplicationGroupStatus{
							ReplicationLinkState: repv1.ReplicationLinkState{
								IsSource:             true,
								LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
							},
						},
					}, nil
				}
				getUpdateReplicationGroupFunction = func(_ k8s.ClusterInterface, _ context.Context, _ *repv1.DellCSIReplicationGroup) error {
					return nil
				}
				getWaitForStateToUpdateFunction = func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
					return false
				}
			},
			expectedExitCode: 0,
		},
		{
			name: "failed: getRGAndClusterFromRGIDFunction failed",
			args: args{
				configFolder: "config",
				rgName:       "rg-id",
				discard:      false,
				verbose:      false,
				wait:         true,
			},
			setup: func() {
				getRGAndClusterFromRGIDFunction = func(_, _, _ string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
					return &k8s.Cluster{ClusterID: "cluster-1"}, &repv1.DellCSIReplicationGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "rg-id",
						},
						Spec: repv1.DellCSIReplicationGroupSpec{},
						Status: repv1.DellCSIReplicationGroupStatus{
							ReplicationLinkState: repv1.ReplicationLinkState{
								IsSource:             true,
								LastSuccessfulUpdate: nil,
							},
						},
					}, errors.New("failback to RG: error fetching source RG info")
				}
				fatalfLog = func(_ string, _ ...interface{}) {
					exitCode = 1
				}
				fatalLog = func(_ ...interface{}) {
					exitCode = 1
				}
				getUpdateReplicationGroupFunction = func(_ k8s.ClusterInterface, _ context.Context, _ *repv1.DellCSIReplicationGroup) error {
					return errors.New("failback: error executing UpdateAction")
				}
				getWaitForStateToUpdateFunction = func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
					return true
				}
			},
			expectedExitCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			defer after()

			failbackToRG(tt.args.configFolder, tt.args.rgName, tt.args.discard, tt.args.verbose, tt.args.wait)

			assert.Equal(t, tt.expectedExitCode, exitCode, "Expected exit code %d, got %d", tt.expectedExitCode, exitCode)
		})
	}
}

func Test_failbackToCluster(t *testing.T) {
	originalGetMultiClusterAllClusters := getMultiClusterAllClusters
	originalWaitForStateToUpdateFunc := waitForStateToUpdateFunc
	originalFatalfLog := fatalfLog
	originalFatalLog := fatalLog
	exitCode := 0

	after := func() {
		getMultiClusterAllClusters = originalGetMultiClusterAllClusters
		waitForStateToUpdateFunc = originalWaitForStateToUpdateFunc
		fatalfLog = originalFatalfLog
		fatalLog = originalFatalLog
		exitCode = 0
	}

	type args struct {
		configFolder       string
		inputSourceCluster string
		rgName             string
		discard            bool
		verbose            bool
		wait               bool
	}
	tests := []struct {
		name             string
		args             args
		setup            func()
		expectedExitCode int
	}{
		{
			name: "Successful function call",
			args: args{
				configFolder:       "",
				inputSourceCluster: "",
				rgName:             "",
				discard:            false,
				verbose:            true,
				wait:               false,
			},
			setup: func() {
				mockClusterA := new(mocks.ClusterInterface)
				mockClusterA.On("GetID").Return("test-name")
				mockClusterA.On("GetReplicationGroups", mock.Anything, mock.Anything).Return(
					&repv1.DellCSIReplicationGroup{
						Status: repv1.DellCSIReplicationGroupStatus{
							ReplicationLinkState: repv1.ReplicationLinkState{
								IsSource: true,
								LastSuccessfulUpdate: &metav1.Time{
									Time: time.Now(),
								},
							},
						},
					},
					nil,
				)
				mockClusterA.On("UpdateReplicationGroup", mock.Anything, mock.Anything).Return(nil)

				getMultiClusterAllClusters = func(mc *k8s.MultiClusterConfigurator, clusterIDs []string, configDir string) (*k8s.Clusters, error) {
					return &k8s.Clusters{
						Clusters: []k8s.ClusterInterface{
							mockClusterA,
						},
					}, nil
				}
				fatalfLog = func(_ string, _ ...interface{}) {
					exitCode = 1
				}
				fatalLog = func(_ ...interface{}) {
					exitCode = 1
				}
			},
			expectedExitCode: 0,
		},
		{
			name: "Covering all failed paths",
			args: args{
				configFolder:       "",
				inputSourceCluster: "",
				rgName:             "",
				discard:            true,
				verbose:            true,
				wait:               true,
			},
			setup: func() {
				mockClusterA := new(mocks.ClusterInterface)
				mockClusterA.On("GetID").Return("test-name")
				mockClusterA.On("GetReplicationGroups", mock.Anything, mock.Anything).Return(
					&repv1.DellCSIReplicationGroup{
						Status: repv1.DellCSIReplicationGroupStatus{
							ReplicationLinkState: repv1.ReplicationLinkState{
								IsSource:             false,
								LastSuccessfulUpdate: nil,
							},
						},
					},
					errors.New("failback: error in fecthing RG info"),
				)
				mockClusterA.On("UpdateReplicationGroup", mock.Anything, mock.Anything).Return(
					errors.New("failback: error executing UpdateAction"),
				)

				getMultiClusterAllClusters = func(mc *k8s.MultiClusterConfigurator, clusterIDs []string, configDir string) (*k8s.Clusters, error) {
					return &k8s.Clusters{
						Clusters: []k8s.ClusterInterface{
							mockClusterA,
						},
					}, errors.New("failback: error in initializing cluster info")
				}

				waitForStateToUpdateFunc = func(_ string, _ k8s.ClusterInterface, _ repv1.ReplicationLinkState) bool {
					return true
				}

				fatalfLog = func(_ string, _ ...interface{}) {
					exitCode = 1
				}
				fatalLog = func(_ ...interface{}) {
					exitCode = 1
				}
			},
			expectedExitCode: 1,
		},
		{
			name: "Covering all failed paths - action timed out",
			args: args{
				configFolder:       "",
				inputSourceCluster: "",
				rgName:             "",
				discard:            true,
				verbose:            true,
				wait:               true,
			},
			setup: func() {
				mockClusterA := new(mocks.ClusterInterface)
				mockClusterA.On("GetID").Return("test-name")
				mockClusterA.On("GetReplicationGroups", mock.Anything, mock.Anything).Return(
					&repv1.DellCSIReplicationGroup{
						Status: repv1.DellCSIReplicationGroupStatus{
							ReplicationLinkState: repv1.ReplicationLinkState{
								IsSource:             false,
								LastSuccessfulUpdate: nil,
							},
						},
					},
					errors.New("failback: error in fecthing RG info"),
				)
				mockClusterA.On("UpdateReplicationGroup", mock.Anything, mock.Anything).Return(
					errors.New("failback: error executing UpdateAction"),
				)

				getMultiClusterAllClusters = func(mc *k8s.MultiClusterConfigurator, clusterIDs []string, configDir string) (*k8s.Clusters, error) {
					return &k8s.Clusters{
						Clusters: []k8s.ClusterInterface{
							mockClusterA,
						},
					}, errors.New("failback: error in initializing cluster info")
				}

				waitForStateToUpdateFunc = func(_ string, _ k8s.ClusterInterface, _ repv1.ReplicationLinkState) bool {
					return false
				}

				fatalfLog = func(_ string, _ ...interface{}) {
					exitCode = 1
				}
				fatalLog = func(_ ...interface{}) {
					exitCode = 1
				}
			},
			expectedExitCode: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()

			failbackToCluster(tt.args.configFolder, tt.args.inputSourceCluster, tt.args.rgName, tt.args.discard, tt.args.verbose, tt.args.wait)

			assert.Equal(t, tt.expectedExitCode, exitCode, "Expected exit code %d, got %d", tt.expectedExitCode, exitCode)
		})
	}
}
