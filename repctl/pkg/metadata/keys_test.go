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

package metadata

import (
	"path"
	"testing"
)

func TestInit(t *testing.T) {
	prefix := "testPrefix"

	Init(prefix)

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"Driver", Driver, path.Join(prefix, driver)},
		{"RemoteClusterID", RemoteClusterID, path.Join(prefix, remoteClusterID)},
		{"RemotePVCNamespace", RemotePVCNamespace, path.Join(prefix, remotePVCNamespace)},
		{"RemotePVC", RemotePVC, path.Join(prefix, remotePVC)},
		{"RemotePV", RemotePV, path.Join(prefix, remotePV)},
		{"ReplicationGroup", ReplicationGroup, path.Join(prefix, replicationGroup)},
		{"ResourceRequest", ResourceRequest, path.Join(prefix, resourceRequest)},
		{"ReplicationEnabled", ReplicationEnabled, path.Join(prefix, replicationEnabled)},
		{"RemoteSCName", RemoteSCName, path.Join(prefix, remoteSCName)},
		{"RemoteReplicationGroup", RemoteReplicationGroup, path.Join(prefix, remoteReplicationGroup)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}
