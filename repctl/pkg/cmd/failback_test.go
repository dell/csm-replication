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
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	v1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/mocks"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
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
	tests := []struct {
		name                       string
		configFolder               string
		rgName                     string
		discard, verbose, wait     bool
		mockGetRGAndCluster        func(string, string, string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error)
		mockUpdateReplicationGroup func(k8s.ClusterInterface, context.Context, *v1.DellCSIReplicationGroup) error
		mockWaitForStateToUpdate   func(string, k8s.ClusterInterface, v1.ReplicationLinkState) bool
		expectedErrorContains      string
	}{
		{
			name:         "success: planned failback without wait",
			configFolder: "config",
			rgName:       "rg-id",
			discard:      false,
			verbose:      true,
			wait:         false,
			mockGetRGAndCluster: func(_, _, _ string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error) {
				return &k8s.Cluster{ClusterID: "cluster-1"}, &v1.DellCSIReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg-id",
					},
					Spec: v1.DellCSIReplicationGroupSpec{},
					Status: v1.DellCSIReplicationGroupStatus{
						ReplicationLinkState: v1.ReplicationLinkState{
							IsSource:             true,
							LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
						},
					},
				}, nil
			},
			mockUpdateReplicationGroup: func(_ k8s.ClusterInterface, _ context.Context, _ *v1.DellCSIReplicationGroup) error {
				return nil
			},
		},
		// {
		// 	name:         "success: planned failback with wait and getWaitForStateToUpdate is false",
		// 	configFolder: "config",
		// 	rgName:       "rg-id",
		// 	discard:      false,
		// 	verbose:      true,
		// 	wait:         true,
		// 	mockGetRGAndCluster: func(_, _, _ string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error) {
		// 		return &k8s.Cluster{ClusterID: "cluster-1"}, &v1.DellCSIReplicationGroup{
		// 			ObjectMeta: metav1.ObjectMeta{
		// 				Name: "rg-id",
		// 			},
		// 			Spec: v1.DellCSIReplicationGroupSpec{},
		// 			Status: v1.DellCSIReplicationGroupStatus{
		// 				ReplicationLinkState: v1.ReplicationLinkState{
		// 					IsSource:             true,
		// 					LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
		// 				},
		// 			},
		// 		}, nil
		// 	},
		// 	mockUpdateReplicationGroup: func(_ k8s.ClusterInterface, _ context.Context, _ *v1.DellCSIReplicationGroup) error {
		// 		return nil
		// 	},
		// 	mockWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState v1.ReplicationLinkState) bool {
		// 		return false
		// 	},
		// },
		// {
		// 	name:         "success: planned failback with wait and getWaitForStateToUpdate is true",
		// 	configFolder: "config",
		// 	rgName:       "rg-id",
		// 	discard:      false,
		// 	verbose:      true,
		// 	wait:         true,
		// 	mockGetRGAndCluster: func(_, _, _ string) (k8s.ClusterInterface, *v1.DellCSIReplicationGroup, error) {
		// 		return &k8s.Cluster{ClusterID: "cluster-1"}, &v1.DellCSIReplicationGroup{
		// 			ObjectMeta: metav1.ObjectMeta{
		// 				Name: "rg-id",
		// 			},
		// 			Spec: v1.DellCSIReplicationGroupSpec{},
		// 			Status: v1.DellCSIReplicationGroupStatus{
		// 				ReplicationLinkState: v1.ReplicationLinkState{
		// 					IsSource:             true,
		// 					LastSuccessfulUpdate: &metav1.Time{Time: time.Now()},
		// 				},
		// 			},
		// 		}, nil
		// 	},
		// 	mockUpdateReplicationGroup: func(_ k8s.ClusterInterface, _ context.Context, _ *v1.DellCSIReplicationGroup) error {
		// 		return nil
		// 	},
		// 	mockWaitForStateToUpdate: func(rgName string, cluster k8s.ClusterInterface, rLinkState v1.ReplicationLinkState) bool {
		// 		return true
		// 	},
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Backup and inject mocks
			origGetRGAndCluster := getRGAndClusterFromRGIDFunction
			origUpdateRG := getUpdateReplicationGroupFunction
			origWait := getWaitForStateToUpdateFunction

			getRGAndClusterFromRGIDFunction = tt.mockGetRGAndCluster
			getUpdateReplicationGroupFunction = tt.mockUpdateReplicationGroup
			getWaitForStateToUpdateFunction = tt.mockWaitForStateToUpdate

			defer func() {
				getRGAndClusterFromRGIDFunction = origGetRGAndCluster
				getUpdateReplicationGroupFunction = origUpdateRG
				getWaitForStateToUpdateFunction = origWait
			}()

			failbackToRG(tt.configFolder, tt.rgName, tt.discard, tt.verbose, tt.wait)

		})
	}
}
