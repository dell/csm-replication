/*
 Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package controllers

import "time"

type actionType string

const (
	// StorageClassReplicationParamEnabledValue value of the replication-enabled flag for which replication is considered to be enabled
	StorageClassReplicationParamEnabledValue = "true"
	// DefaultRetryInterval default interval after which controllers reconcile objects
	DefaultRetryInterval = 2 * time.Second

	storageClassReplicationParam        = "/isReplicationEnabled"
	storageClassRemoteStorageClassParam = "/remoteStorageClassName"
	storageClassRemoteClusterParam      = "/remoteClusterID"
	// StorageClassRemoteClusterParam—finalizer used by the replication sidecar and utils controller for pre delete hook
	replicationFinalizer = "/replicationProtection"
	// rgFinalizer - used by common controller for RG sync
	rgFinalizer = "/replicationSyncProtection"
	// RemoteVolumeAnnotation—annotation on the local PVC for details about the created remote volume
	remoteVolumeAnnotation = "/remoteVolume"
	// RemoteStorageClassAnnotation—annotation on the local PVC for the name of the remote storage class, to be used for remote PV.
	remoteStorageClassAnnotation = "/remoteStorageClassName"
	// PVCProtectionComplete - annotation which implies that the local sidecar has completed its actions for PVC
	pVCProtectionComplete = "/pvc_protection_complete"
	// PVProtectionComplete - annotation which implies that the local sidecar has completed its actions for PV
	pVProtectionComplete = "/pv_protection_complete"
	// PVCSyncComplete - Annotation which is set by the utils controller to indicate that it has finished the full PVC sync cycle
	pVCSyncComplete = "/pvc_sync_complete"
	// PVSyncComplete
	pVSyncComplete = "/pv_sync_complete"
	// ContextPrefix—prefix, if added to any of the key of protection-group-attributes, should be added as annotation to the DellCSIReplicationGroup
	contextPrefix = "/contextPrefix"
	// ProtectionGroupRemovedFinalizer- annotation added after the protection-group is successfully removed
	// and is used as a flag by the utils controller to process the deletion of remote replication-group
	protectionGroupRemovedAnnotation = "/protectionGroupDeleted"
	// DriverName
	// useful for filtering out objects for a particular driver
	driverName = "/driverName"
	// RemoteClusterID
	// Contains the identifier of the remote cluster
	// Used in annotations to enable utils controller to connect to the remote cluster
	// for syncing objects across clusters
	// Used in labels to enable filtering out objects which are replicated to a particular cluster
	remoteClusterID = "/remoteClusterID"
	// RemoteReplicationGroup
	// Contains the name of the associated DellCSIReplicationGroup on the remote cluster
	// Used in annotations as well as labels
	remoteReplicationGroup = "/remoteReplicationGroupName"
	// RGSyncComplete
	rGSyncComplete = "/rg_sync_complete"
	// RemotePV
	// Contains the name of the remotePV object
	remotePV = "/remotePV"
	// RemotePVC
	// Contains the name of the remote PVC object
	remotePVC = "/remotePVC"
	// RemotePVCNamespace
	// Contains the namespace of the remote PVC object
	remotePVCNamespace = "/remotePVCNamespace"
	// ReplicationGroup
	// Contains the name of the local DellCSIReplicationGroup object
	replicationGroup = "/replicationGroupName"
	// CreatedBy
	// Annotation which indicates that this object was created by the replication-controller-manager
	createdBy = "/createdBy"
	// resource requirements
	resourceRequest = "/resourceRequest"
	// DeletionRequested annotation to set for the deleted object
	deletionRequested = "/deletionRequested"
	// Self typically used when remote cluster is same as source
	Self = "self"
	// RemotePVRetentionPolicy
	// Indicates whether to retain or delete the target PV
	remotePVRetentionPolicy = "/remotePVRetentionPolicy"
	// RemoteRetentionValueRetain is the value for remotePVRetentionPolicy or remoteRGRetentionPolicy to mean retain
	RemoteRetentionValueRetain = "retain"
	// RemoteRetentionValueDelete is the value for remotePVRetentionPolicy or remoteRGRetentionPolicy to mean delete
	RemoteRetentionValueDelete = "delete"
	// RemoteRGRetentionPolicy
	// Indicates whether to retain or delete the target RG
	remoteRGRetentionPolicy = "/remoteRGRetentionPolicy"
	// Indicates if migration is requested for given volume. Value is the target SC.
	migrateTo = "/migrate-to"
	// Indicates target NS for migrated pvc
	migrateNS = "/namespace"
	// Indicates that this PV's been created by migrator sidecar
	createdByMigrator = "/createdByMigrator"
	// FailOver failover replication action name
	FailOver = "failover"
	// FailBack failback replication action name
	FailBack = "failback"
	// Suspend suspend replication action name
	Suspend = "suspend"
	// Resume resume replication action name
	Resume = "resume"
	// Establish establish replication action name
	Establish = "establish"
	// TestFailOver test failover replication action name
	TestFailOver = "testfailover"
	// TestFailOverStop stop test failover replication action name
	TestFailOverStop = "testfailoverstop"
	// Indicates the snapshot class to use when performing a remote snapshot.
	snapshotClass = "/snapshotClass"
	// Indicates the snapshot namespace to use when performing a remote snapshot.
	snapshotNamespace = "/snapshotNamespace"
	// Indicates the time which the last action was processed.
	actionProcessedTime = "/actionProcessedTime"
	// KubeSystemNamespace indicates the namespace of the system which the controller is installed on.
	KubeSystemNamespace = "kube-system"
	// ClusterUID indicates the clusterUID retrieved from the KubeSystem.
	ClusterUID = "clusterUID"
)
