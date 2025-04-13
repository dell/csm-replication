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
	"github.com/dell/repctl/mocks"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetFailoverCommand(t *testing.T) {
	t.Run("fail: unexpected input", func(t *testing.T) {
		viper.Set("tgt", "cluster2")
		viper.Set("unplanned", false)
		viper.Set("failover-wait", true)
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

		cmd := GetFailoverCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: using cluster id - planned", func(t *testing.T) {
		viper.Set("tgt", "cluster-1")
		viper.Set("unplanned", false)
		viper.Set("failover-wait", false)
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

		repGroup := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rg-id",
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: false,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *repGroup)
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *repList)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetFailoverCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: using cluster id - unplanned", func(t *testing.T) {
		viper.Set("tgt", "cluster-1")
		viper.Set("unplanned", true)
		viper.Set("failover-wait", false)
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

		repGroup := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rg-id",
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: false,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *repGroup)
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *repList)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetFailoverCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: using target - planned", func(t *testing.T) {
		viper.Set("tgt", "cluster-2")
		viper.Set("unplanned", false)
		viper.Set("failover-wait", false)
		viper.Set(config.ReplicationGroup, "rg-id")
		viper.Set(config.ReplicationPrefix, "replication.storage.dell.com")
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
						Name: "cluster-2",
					},
				},
			},
		}

		repGroup := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rg-id",
				Annotations: map[string]string{
					"replication.storage.dell.com/remoteReplicationGroupName": "rg-id-target",
				},
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: false,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *repGroup)
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *repList)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetFailoverCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: using target - unplanned", func(t *testing.T) {
		viper.Set("tgt", "cluster-2")
		viper.Set("unplanned", true)
		viper.Set("failover-wait", false)
		viper.Set(config.ReplicationGroup, "rg-id")
		viper.Set(config.ReplicationPrefix, "replication.storage.dell.com")
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
						Name: "cluster-2",
					},
				},
			},
		}

		repGroup := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rg-id",
				Annotations: map[string]string{
					"replication.storage.dell.com/remoteReplicationGroupName": "rg-id-target",
				},
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: false,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *repGroup)
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *repList)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetFailoverCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})
}

func TestWaitForStateToUpdate(t *testing.T) {
	t.Run("success: waitForStateToUpdate", func(t *testing.T) {
		cluster := mocks.NewMockClusterInterface(gomock.NewController(t))
		rgName := "rg-" + uuid.New().String()
		rg := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: rgName,
				Annotations: map[string]string{
					"replication.storage.dell.com/remoteReplicationGroupName": "rg-id-target",
				},
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: false,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		cluster.EXPECT().GetReplicationGroups(gomock.Any(), gomock.Any()).Times(1).Return(rg, nil)

		cluster.EXPECT().GetReplicationGroups(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, name string) (*repv1.DellCSIReplicationGroup, error) {
			rg.Status.ReplicationLinkState.LastSuccessfulUpdate = &metav1.Time{
				Time: time.Now().Add(maxWaitTimeout),
			}
			return rg, nil
		})

		maxWaitTimeout = 5 * time.Second
		waitRetryDelay = 1 * time.Second

		result := waitForStateToUpdate(rgName, cluster, rg.Status.ReplicationLinkState)
		assert.True(t, result)
	})

	t.Run("fail: waitForStateToUpdate", func(t *testing.T) {
		cluster := mocks.NewMockClusterInterface(gomock.NewController(t))
		rgName := "rg-" + uuid.New().String()
		rg := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: rgName,
				Annotations: map[string]string{
					"replication.storage.dell.com/remoteReplicationGroupName": "rg-id-target",
				},
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: false,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		cluster.EXPECT().GetReplicationGroups(gomock.Any(), gomock.Any()).AnyTimes().Return(rg, nil)

		maxWaitTimeout = 1 * time.Second
		waitRetryDelay = 250 * time.Millisecond

		result := waitForStateToUpdate(rgName, cluster, rg.Status.ReplicationLinkState)
		assert.False(t, result)
	})
}
