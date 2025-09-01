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

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetRG(t *testing.T) {
	tests := []struct {
		name       string
		group      repv1.DellCSIReplicationGroup
		expectedRG RG
	}{
		{
			name: "Replication group with valid parameters",
			group: repv1.DellCSIReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rg1",
					Annotations: map[string]string{
						"remoteClusterID":        "",
						"remoteReplicationGroup": "",
					},
				},
				Spec: repv1.DellCSIReplicationGroupSpec{
					ProtectionGroupID: "pg1",
					DriverName:        "driver1",
				},
				Status: repv1.DellCSIReplicationGroupStatus{
					State: "Active",
					ReplicationLinkState: repv1.ReplicationLinkState{
						IsSource: true,
						State:    "Linked",
					},
				},
			},
			expectedRG: RG{
				Name:              "rg1",
				State:             "Active",
				ProtectionGroupID: "pg1",
				DriverName:        "driver1",
				RemoteClusterID:   "",
				RemoteRGName:      "",
				IsSource:          true,
				LinkState:         "Linked",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualRG := GetRG(tt.group)
			assert.Equal(t, tt.expectedRG, actualRG)
		})
	}
}

func TestRGList_Print(t *testing.T) {
	rgl := &RGList{
		RGList: []RG{
			{
				Name:              "test-rg",
				State:             "Active",
				ProtectionGroupID: "pg1",
				RemoteClusterID:   "cluster-A",
				DriverName:        "test.dellemc.com",
				RemoteRGName:      "remote-rg1",
				IsSource:          true,
				LinkState:         "Linked",
			},
		},
	}
	rgl.Print()
}
