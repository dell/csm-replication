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
	"fmt"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/mocks"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetSwapCommand(t *testing.T) {
	t.Run("fail: unexpected input", func(t *testing.T) {
		viper.Set("toTgt", "cluster2")
		viper.Set("swap-wait", false)
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

		rg := &repv1.DellCSIReplicationGroup{
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
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *repList)
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *rg)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetSwapCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: swap at cluster", func(t *testing.T) {
		viper.Set("toTgt", "cluster-1")
		viper.Set("swap-wait", false)
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

		rgList := &repv1.DellCSIReplicationGroupList{
			Items: []repv1.DellCSIReplicationGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg-id",
					},
				},
			},
		}

		rg := &repv1.DellCSIReplicationGroup{
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
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *rgList)
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *rg)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetSwapCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: swap at cluster - wait", func(t *testing.T) {
		viper.Set("toTgt", "cluster-1")
		viper.Set("swap-wait", true)
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
		originalGetWaitForStateToUpdateFunction := getWaitForStateToUpdateFunction
		defer func() {
			getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			getWaitForStateToUpdateFunction = originalGetWaitForStateToUpdateFunction
		}()

		getClustersFolderPathFunction = func(path string) (string, error) {
			return path, nil
		}
		getWaitForStateToUpdateFunction = func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
			return true
		}

		rgList := &repv1.DellCSIReplicationGroupList{
			Items: []repv1.DellCSIReplicationGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg-id",
					},
				},
			},
		}

		rg := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rg-id",
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: true,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *rgList)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		maxWaitTimeout = 100 * time.Millisecond
		waitRetryDelay = 50 * time.Millisecond

		setAndReturnRG := func() repv1.DellCSIReplicationGroup {
			rg.Status.ReplicationLinkState.LastSuccessfulUpdate = &metav1.Time{
				Time: time.Now().Add(maxWaitTimeout),
			}

			return *rg
		}
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, setAndReturnRG())

		cmd := GetSwapCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("fail: swap at cluster - wait timeout", func(t *testing.T) {
		viper.Set("toTgt", "cluster-1")
		viper.Set("swap-wait", true)
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
		originalGetWaitForStateToUpdateFunction := getWaitForStateToUpdateFunction
		defer func() {
			getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			getWaitForStateToUpdateFunction = originalGetWaitForStateToUpdateFunction
		}()

		getClustersFolderPathFunction = func(path string) (string, error) {
			return path, nil
		}
		getWaitForStateToUpdateFunction = func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
			return false
		}

		rgList := &repv1.DellCSIReplicationGroupList{
			Items: []repv1.DellCSIReplicationGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg-id",
					},
				},
			},
		}

		rg := &repv1.DellCSIReplicationGroup{
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
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *rgList)
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *rg)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		maxWaitTimeout = 5 * time.Millisecond
		waitRetryDelay = 50 * time.Millisecond

		cmd := GetSwapCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("fail: swap at rg - unable to list rg", func(t *testing.T) {
		viper.Set("toTgt", "myReplicationGroup")
		viper.Set("swap-wait", false)
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

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(fmt.Errorf("unable to list replication groups"))

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetSwapCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: swap at rg", func(t *testing.T) {
		viper.Set("toTgt", "")
		viper.Set("swap-wait", false)
		viper.Set(config.ReplicationGroup, "myReplicationGroup")
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

		rgList := &repv1.DellCSIReplicationGroupList{
			Items: []repv1.DellCSIReplicationGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "myReplicationGroup",
					},
				},
			},
		}

		rg := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myReplicationGroup",
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: true,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *rgList)
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *rg)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetSwapCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: swap at rg - wait", func(t *testing.T) {
		viper.Set("toTgt", "")
		viper.Set("swap-wait", true)
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
		originalGetWaitForStateToUpdateFunction := getWaitForStateToUpdateFunction
		defer func() {
			getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			getWaitForStateToUpdateFunction = originalGetWaitForStateToUpdateFunction
		}()

		getClustersFolderPathFunction = func(path string) (string, error) {
			return path, nil
		}
		getWaitForStateToUpdateFunction = func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
			return true
		}

		rgList := &repv1.DellCSIReplicationGroupList{
			Items: []repv1.DellCSIReplicationGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg-id",
					},
				},
			},
		}

		rg := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rg-id",
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: true,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *rgList)
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil).SetArg(2, *rg)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		maxWaitTimeout = 100 * time.Millisecond
		waitRetryDelay = 50 * time.Millisecond

		setAndReturnRG := func() repv1.DellCSIReplicationGroup {
			rg.Status.ReplicationLinkState.LastSuccessfulUpdate = &metav1.Time{
				Time: time.Now().Add(maxWaitTimeout),
			}

			return *rg
		}
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, setAndReturnRG())

		cmd := GetSwapCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("fail: swap at rg - wait timeout", func(t *testing.T) {
		viper.Set("toTgt", "")
		viper.Set("swap-wait", true)
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
		originalGetWaitForStateToUpdateFunction := getWaitForStateToUpdateFunction
		defer func() {
			getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			getWaitForStateToUpdateFunction = originalGetWaitForStateToUpdateFunction
		}()

		getClustersFolderPathFunction = func(path string) (string, error) {
			return path, nil
		}
		getWaitForStateToUpdateFunction = func(rgName string, cluster k8s.ClusterInterface, rLinkState repv1.ReplicationLinkState) bool {
			return false
		}

		rgList := &repv1.DellCSIReplicationGroupList{
			Items: []repv1.DellCSIReplicationGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg-id",
					},
				},
			},
		}

		rg := &repv1.DellCSIReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rg-id",
			},
			Spec: repv1.DellCSIReplicationGroupSpec{
				RemoteClusterID: "cluster-2",
			},
			Status: repv1.DellCSIReplicationGroupStatus{
				ReplicationLinkState: repv1.ReplicationLinkState{
					IsSource: true,
					State:    "Synchronized",
					LastSuccessfulUpdate: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		}

		mockClient := mocks.NewMockClientInterface(gomock.NewController(t))
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(1, *rgList)
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *rg)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		maxWaitTimeout = 100 * time.Millisecond
		waitRetryDelay = 50 * time.Millisecond

		cmd := GetSwapCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})
}
