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

package constants

// Constants
const (
	DefaultConfigFileName     = "config"
	DefaultConfigDir          = "deploy"
	EnvWatchNameSpace         = "X_CSI_REPLICATION_WATCH_NAMESPACE"
	EnvConfigFileName         = "X_CSI_REPLICATION_CONFIG_FILE_NAME"
	EnvConfigDirName          = "X_CSI_REPLICATION_CONFIG_DIR"
	EnvInClusterConfig        = "X_CSI_REPLICATION_IN_CLUSTER"
	EnvUseConfFileFormat      = "X_CSI_USE_CONF_FILE_FORMAT"
	DefaultNameSpace          = "dell-replication-controller"
	DefaultDomain             = "replication.storage.dell.com"
	DefaultMigrationDomain    = "migration.storage.dell.com"
	DellReplicationController = "dell-replication-controller"
	// DellCSIReplicator - Name of the sidecar controller manager
	DellCSIReplicator = "dell-csi-replicator"
	// DellCSIMigrator - Name of the sidecar controller manager
	DellCSIMigrator = "dell-csi-migrator"
	Monitoring      = "rg-monitoring"
	// DellCSINodeReScanner - Name of the node sidecar manager
	DellCSINodeReScanner = "dell-csi-node-rescanner"
	EnvNodeName          = "X_CSI_NODE_NAME"
)
