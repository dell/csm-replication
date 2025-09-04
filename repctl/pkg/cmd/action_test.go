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

func TestGetExecCommand(t *testing.T) {
	t.Run("fail: invalid action", func(t *testing.T) {
		viper.Set("action", "cluster2")
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

		cmd := GetExecCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("fail: attempting to execute action on a non-source rg", func(t *testing.T) {
		viper.Set("action", "resume")
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
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *rg)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetExecCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("fail: get replication error", func(t *testing.T) {
		viper.Set("action", "resume")
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
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(fmt.Errorf("error"))

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetExecCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})

	t.Run("success: execute/update rg", func(t *testing.T) {
		viper.Set("action", "resume")
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
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil).SetArg(2, *rg)
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

		defaultGetControllerRuntimeClient := k8s.GetCtrlRuntimeClient
		defer func() {
			k8s.GetCtrlRuntimeClient = defaultGetControllerRuntimeClient
		}()
		k8s.GetCtrlRuntimeClient = func(kubeconfig string) (client.Client, error) {
			return mockClient, nil
		}

		cmd := GetExecCommand()
		assert.NotNil(t, cmd)

		cmd.Run(nil, nil)
	})
}

func TestGetSupportedMaintenanceAction(t *testing.T) {
	tests := []struct {
		action   string
		expected string
		err      error
	}{
		{"resume", config.ActionResume, nil},
		{"suspend", config.ActionSuspend, nil},
		{"sync", config.ActionSync, nil},
		{"establish", config.ActionEstablish, nil}
		{"invalid", "", fmt.Errorf("not a supported action")},
	}

	for _, test := range tests {
		result, err := getSupportedMaintenanceAction(test.action)
		if result != test.expected || (err != nil && err.Error() != test.err.Error()) {
			t.Errorf("For action %s, expected %s and error %v, but got %s and error %v", test.action, test.expected, test.err, result, err)
		}
	}
}
