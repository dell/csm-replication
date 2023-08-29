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

import (
	"context"
	"os"

	repv1 "github.com/dell/csm-replication/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// AddAnnotation adds annotation to k8s resources
func AddAnnotation(obj metav1.Object, annotationKey, annotationValue string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotationKey] = annotationValue
	obj.SetAnnotations(annotations)
}

// AddLabel adds labels to k8s resources
func AddLabel(obj metav1.Object, labelKey, labelValue string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[labelKey] = labelValue
	obj.SetLabels(labels)
}

// DeleteLabel deletes labels from k8s resources
func DeleteLabel(obj metav1.Object, labelKey string) bool {
	labels := obj.GetLabels()
	if _, ok := labels[labelKey]; !ok {
		return true
	}
	delete(labels, labelKey)
	obj.SetLabels(labels)
	return true
}

// AddFinalizerIfNotExist adds a finalizer to k8s resource, if it doesn't already exist and
// returns true else returns false
func AddFinalizerIfNotExist(obj metav1.Object, name string) bool {
	finalizers := obj.GetFinalizers()
	if finalizers == nil {
		finalizers = make([]string, 0)
	} else {
		// Check if finalizer already exists
		for _, finalizer := range finalizers {
			if finalizer == name {
				return false
			}
		}
	}
	finalizers = append(finalizers, name)
	obj.SetFinalizers(finalizers)
	return true
}

// RemoveFinalizerIfExists removes finalizer from k8s resource and returns true if
// exists, otherwise returns false
func RemoveFinalizerIfExists(obj metav1.Object, name string) bool {
	finalizerRemoved := false
	finalizers := obj.GetFinalizers()
	length := len(finalizers)
	for i := 0; i < length; i++ {
		if finalizers[i] == name {
			length = length - 1
			finalizers[i], finalizers[length] = finalizers[length], finalizers[i]
			break
		}
	}
	if length < len(finalizers) {
		finalizerRemoved = true
	}
	finalizers = finalizers[:length]
	obj.SetFinalizers(finalizers)
	return finalizerRemoved
}

// IsCSIFinalError return true only if there is no point in retrying
func IsCSIFinalError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		// Not a grpc error.
		return false
	}
	switch st.Code() {
	case codes.Canceled,
		codes.DeadlineExceeded,
		codes.Unavailable,
		codes.ResourceExhausted,
		codes.Internal,
		codes.Aborted:

		return false
	}
	return true
}

// IgnoreIfFinalError return error only if it is not final
func IgnoreIfFinalError(err error) error {
	if IsCSIFinalError(err) {
		return nil
	}
	return err
}

// UpdateConditions updates conditions status field by adding last action condition
func UpdateConditions(rg *repv1.DellCSIReplicationGroup, condition repv1.LastAction, maxConditions int) {
	rg.Status.Conditions = append([]repv1.LastAction{condition}, rg.Status.Conditions...)
	if len(rg.Status.Conditions) > maxConditions {
		rg.Status.Conditions = rg.Status.Conditions[:maxConditions]
	}
}

// PublishControllerEvent publishes event to all contoller pods
func PublishControllerEvent(ctx context.Context, client ctrlClient.Client, recorder record.EventRecorder, eventType string, reason string, msg string) error {
	pod := &v1.Pod{}
	err := client.Get(ctx, types.NamespacedName{Name: GetPodNameFromEnv(), Namespace: GetPodNameSpaceFromEnv()}, pod)
	if err != nil {
		return err
	}
	recorder.Event(pod, eventType, reason, msg)

	return nil
}

// GetPodNameFromEnv gets value of X_CSI_REPLICATION_POD_NAME env variable
func GetPodNameFromEnv() string {
	if p := os.Getenv("X_CSI_REPLICATION_POD_NAME"); p != "" {
		return p
	}
	return ""
}

// GetPodNameSpaceFromEnv gets value of X_CSI_REPLICATION_POD_NAMESPACE env variable
func GetPodNameSpaceFromEnv() string {
	if p := os.Getenv("X_CSI_REPLICATION_POD_NAMESPACE"); p != "" {
		return p
	}
	return ""
}

var (
	// StorageClassReplicationParam — storage class params to flag creation of replicated volumes
	StorageClassReplicationParam string
	// StorageClassRemoteStorageClassParam — storage class param for the name of the remote storage class
	StorageClassRemoteStorageClassParam string
	// StorageClassRemoteClusterParam — storage class params for the remote cluster identification
	StorageClassRemoteClusterParam string
	// ReplicationFinalizer — finalizer used by the replication sidecar and common controller for pre delete hook
	ReplicationFinalizer string
	// RGFinalizer - finalizer used by common controller for pre-delete hook for RG
	RGFinalizer string
	// RemoteVolumeAnnotation — annotation on the local PVC for details about the created remote volume
	RemoteVolumeAnnotation string
	// RemoteStorageClassAnnotation — annotation on the local PVC for the name of the remote storage class, to be used for remote PV.
	RemoteStorageClassAnnotation string
	// PVCProtectionComplete - annotation which implies that the local sidecar has completed its actions for PVC
	PVCProtectionComplete string
	// PVProtectionComplete - annotation which implies that the local sidecar has completed its actions for PV
	PVProtectionComplete string
	// PVCSyncComplete - Annotation which is set by the utils controller to indicate that it has finished the full PVC sync cycle
	PVCSyncComplete string
	// PVSyncComplete  - Annotation which is set by the utils controller to indicate that it has finished the full PV sync cycle
	PVSyncComplete string
	// ContextPrefix — prefix, if added to any of the key of protection-group-attributes, should be added as annotation to the DellCSIReplicationGroup
	ContextPrefix string
	// ProtectionGroupRemovedAnnotation - annotation added after the protection-group is successfully removed
	// and is used as a flag by the utils controller to process the deletion of remote replication-group
	ProtectionGroupRemovedAnnotation string
	// DriverName is useful for filtering out objects for a particular driver
	DriverName string
	// RemoteClusterID contains the identifier of the remote cluster
	// Used in annotations to enable utils controller to connect to the remote cluster
	// for syncing objects across clusters
	// Used in labels to enable filtering out objects which are replicated to a particular cluster
	RemoteClusterID string
	// RemoteReplicationGroup contains the name of the associated DellCSIReplicationGroup on the remote cluster
	// Used in annotations as well as labels
	RemoteReplicationGroup string
	// RGSyncComplete indicates whether RGs are synced or not
	RGSyncComplete string
	// RemotePV contains the name of the remotePV object
	RemotePV string
	// RemotePVC contains the name of the remote PVC object
	RemotePVC string
	// RemotePVCNamespace contains the namespace of the remote PVC object
	RemotePVCNamespace string
	// ReplicationGroup contains the name of the local DellCSIReplicationGroup object
	ReplicationGroup string
	// CreatedBy annotation which indicates that this object was created by the replication-controller-manager
	CreatedBy string
	// ResourceRequest contains the requested resource limits of the source PVC
	// To be used while creating the remote PVC
	ResourceRequest string
	// DeletionRequested annotation which will be set to the PV on the object deletion
	DeletionRequested string
	// SynchronizedDeletionStatus indicates what state the PV's synchronized deletion process is in
	// requested: PV has been issued a delete command by the remote controller and needs to run DeleteLocalVolume.
	// complete: DeleteLocalVolume has completed. Placed on PV by replicator sidecar to inform controller that deletion is done.
	SynchronizedDeletionStatus string
	// RemotePVRetentionPolicy indicates whether to retain or delete the target PV
	RemotePVRetentionPolicy string
	// RemoteRGRetentionPolicy indicates whether to retain or delete the target RG
	RemoteRGRetentionPolicy string
	// MigrationRequested  annotation indicates if migration is requested for given volume
	MigrationRequested string
	// MigrationNamespace indicates target pvc namespace
	MigrationNamespace string
	// CreatedByMigrator indicates that this PV's been created by migrator sidecar
	CreatedByMigrator string

	// SnapshotClass name of the desired snapshot class.
	SnapshotClass string
	// SnapshotNamespace name of the target namespace to create snapshots in.
	SnapshotNamespace string
	// ActionProcessedTime indicates when the last action was proccessed by the controller (if needed).
	ActionProcessedTime string

	// MigrationGroup contains the name of the local DellCSIMigrationGroup object
	MigrationGroup string
	// MigrationFinalizer — finalizer used by the migration sidecar for pre delete hook
	MigrationFinalizer string
)

// InitLabelsAndAnnotations initializes package visible constants by using customizable domain variable
func InitLabelsAndAnnotations(domain string) {
	StorageClassReplicationParam = domain + storageClassReplicationParam
	StorageClassRemoteStorageClassParam = domain + storageClassRemoteStorageClassParam
	StorageClassRemoteClusterParam = domain + storageClassRemoteClusterParam
	ReplicationFinalizer = domain + replicationFinalizer
	RGFinalizer = domain + rgFinalizer
	RemoteVolumeAnnotation = domain + remoteVolumeAnnotation
	RemoteStorageClassAnnotation = domain + remoteStorageClassAnnotation
	PVCProtectionComplete = domain + pVCProtectionComplete
	PVProtectionComplete = domain + pVProtectionComplete
	PVCSyncComplete = domain + pVCSyncComplete
	PVSyncComplete = domain + pVSyncComplete
	ContextPrefix = domain + contextPrefix
	ProtectionGroupRemovedAnnotation = domain + protectionGroupRemovedAnnotation
	DriverName = domain + driverName
	RemoteClusterID = domain + remoteClusterID
	RemoteReplicationGroup = domain + remoteReplicationGroup
	RGSyncComplete = domain + rGSyncComplete
	RemotePV = domain + remotePV
	RemotePVC = domain + remotePVC
	RemotePVCNamespace = domain + remotePVCNamespace
	ReplicationGroup = domain + replicationGroup
	CreatedBy = domain + createdBy
	ResourceRequest = domain + resourceRequest
	DeletionRequested = domain + deletionRequested
	SynchronizedDeletionStatus = domain + synchronizedDeletionStatus
	RemotePVRetentionPolicy = domain + remotePVRetentionPolicy
	RemoteRGRetentionPolicy = domain + remoteRGRetentionPolicy
	MigrationRequested = domain + migrateTo
	MigrationNamespace = domain + migrateNS
	CreatedByMigrator = domain + createdByMigrator
	SnapshotClass = domain + snapshotClass
	SnapshotNamespace = domain + snapshotNamespace
	ActionProcessedTime = domain + actionProcessedTime
	MigrationGroup = domain + migrationGroup
	MigrationFinalizer = domain + migrationFinalizer
}
