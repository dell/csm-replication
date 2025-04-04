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

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetSC(t *testing.T) {
	tests := []struct {
		name       string
		sc         v1.StorageClass
		clusterID  string
		repEnabled bool
		expectedSC StorageClass
	}{
		{
			name: "Replication enabled with valid parameters",
			sc: v1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc1",
				},
				Provisioner: "driver1",
				Parameters: map[string]string{
					"remoteClusterID": "",
					"remoteSCName":    "",
				},
			},
			clusterID:  "localCluster1",
			repEnabled: true,
			expectedSC: StorageClass{
				Name:            "sc1",
				Provisioner:     "driver1",
				RemoteSCName:    "",
				LocalClusterID:  "localCluster1",
				RemoteClusterID: "",
				Valid:           "false",
				Parameters:      map[string]string{"remoteClusterID": "", "remoteSCName": ""},
			},
		},
		{
			name: "Replication disabled",
			sc: v1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc2",
				},
				Provisioner: "driver2",
				Parameters: map[string]string{
					"remoteClusterID": "cluster-2",
					"remoteSCName":    "remote-sc1",
				},
			},
			clusterID:  "localCluster3",
			repEnabled: false,
			expectedSC: StorageClass{
				Name:            "sc2",
				Provisioner:     "driver2",
				RemoteSCName:    "N/A",
				LocalClusterID:  "N/A",
				RemoteClusterID: "N/A",
				Valid:           "N/A",
				Parameters:      map[string]string{"remoteClusterID": "cluster-2", "remoteSCName": "remote-sc1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSC := GetSC(tt.sc, tt.clusterID, tt.repEnabled)
			assert.Equal(t, tt.expectedSC, actualSC)
		})
	}
}

func TestSCList_Print(t *testing.T) {
	scl := &SCList{
		SCList: []StorageClass{
			{
				Name:            "test-sc",
				Provisioner:     "test.dellemc.com",
				RemoteSCName:    "test-remote-sc",
				LocalClusterID:  "cluster-A",
				RemoteClusterID: "cluster-B",
				Valid:           "true",
			},
		},
	}

	scl.Print()
}
