/*
 Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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
)

const (
	driver                 = "driverName"
	remoteClusterID        = "remoteClusterID"
	remotePVCNamespace     = "remotePVCNamespace"
	remotePVC              = "remotePVC"
	remotePV               = "remotePV"
	replicationGroup       = "replicationGroupName"
	remoteReplicationGroup = "/remoteReplicationGroupName"
	resourceRequest        = "resourceRequest"
	replicationEnabled     = "isReplicationEnabled"
	remoteSCName           = "remoteStorageClassName"
)

// Exports
var (
	Driver                 string
	RemoteClusterID        string
	RemotePVCNamespace     string
	RemotePVC              string
	RemotePV               string
	ReplicationGroup       string
	RemoteReplicationGroup string
	ResourceRequest        string
	ReplicationEnabled     string
	RemoteSCName           string
)

// Init - initialize keys
func Init(prefix string) {
	Driver = path.Join(prefix, driver)
	RemoteClusterID = path.Join(prefix, remoteClusterID)
	RemotePVCNamespace = path.Join(prefix, remotePVCNamespace)
	RemotePVC = path.Join(prefix, remotePVC)
	RemotePV = path.Join(prefix, remotePV)
	ReplicationGroup = path.Join(prefix, replicationGroup)
	ResourceRequest = path.Join(prefix, resourceRequest)
	ReplicationEnabled = path.Join(prefix, replicationEnabled)
	RemoteSCName = path.Join(prefix, remoteSCName)
	RemoteReplicationGroup = path.Join(prefix, remoteReplicationGroup)
}
