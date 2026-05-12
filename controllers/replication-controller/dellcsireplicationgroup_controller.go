/*
 Copyright © 2021-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package replicationcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	csireplicator "github.com/dell/csm-replication/controllers/csi-replicator"

	repv1 "github.com/dell/csm-replication/api/v1"
	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common/logger"
	"github.com/dell/csm-replication/pkg/connection"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	eventTypeNormal    = "Normal"
	eventTypeWarning   = "Warning"
	eventReasonUpdated = "Updated"
)

var (
	getDellCsiReplicationGroupGetSnapshotClass = func(r connection.RemoteClusterClient, ctx context.Context, snapClassName string) (*s1.VolumeSnapshotClass, error) {
		return r.GetSnapshotClass(ctx, snapClassName)
	}
	getDellCsiReplicationGroupGetNamespace = func(r connection.RemoteClusterClient, ctx context.Context, namespace string) (*v1.Namespace, error) {
		return r.GetNamespace(ctx, namespace)
	}
	getDellCsiReplicationGroupCreateNamespace = func(r connection.RemoteClusterClient, ctx context.Context, content *v1.Namespace) error {
		return r.CreateNamespace(ctx, content)
	}
	getDellCsiReplicationGroupCreateSnapshotContent = func(r connection.RemoteClusterClient, ctx context.Context, snapContent *s1.VolumeSnapshotContent) error {
		return r.CreateSnapshotContent(ctx, snapContent)
	}
	getDellCsiReplicationGroupCreateSnapshotObject = func(remoteClient connection.RemoteClusterClient, ctx context.Context, snapshot *s1.VolumeSnapshot) error {
		return remoteClient.CreateSnapshotObject(ctx, snapshot)
	}
	getDellCsiReplicationGroupProcessSnapshotEvent = func(r *ReplicationGroupReconciler, ctx context.Context, group *repv1.DellCSIReplicationGroup, remoteClient connection.RemoteClusterClient, log logr.Logger) error {
		return r.processSnapshotEvent(ctx, group, remoteClient, log)
	}
	getDellCsiReplicationGroupUpdate = func(r *ReplicationGroupReconciler, ctx context.Context, group *repv1.DellCSIReplicationGroup) error {
		return r.Update(ctx, group)
	}
	getPersistentVolume = func(ctx context.Context, client connection.RemoteClusterClient, pvName string) (*v1.PersistentVolume, error) {
		return client.GetPersistentVolume(ctx, pvName)
	}
	updatePersistentVolume = func(ctx context.Context, client connection.RemoteClusterClient, pv *v1.PersistentVolume) error {
		return client.UpdatePersistentVolume(ctx, pv)
	}
	deletePersistentVolumeClaim = func(ctx context.Context, client connection.RemoteClusterClient, pvc *v1.PersistentVolumeClaim) error {
		return client.DeletePersistentVolumeClaim(ctx, pvc)
	}
	createPersistentVolumeClaim = func(ctx context.Context, client connection.RemoteClusterClient, pvc *v1.PersistentVolumeClaim) error {
		return client.CreatePersistentVolumeClaim(ctx, pvc)
	}
	getPersistentVolumeClaim = func(ctx context.Context, client connection.RemoteClusterClient, namespace string, pvcName string) (*v1.PersistentVolumeClaim, error) {
		return client.GetPersistentVolumeClaim(ctx, namespace, pvcName)
	}
	sleep = func(d time.Duration) {
		time.Sleep(d)
	}
	getObject = func(ctx context.Context, c connection.RemoteClusterClient, key client.ObjectKey, obj client.Object) error {
		return c.GetObject(ctx, key, obj)
	}
	updateObject = func(ctx context.Context, c connection.RemoteClusterClient, obj client.Object) error {
		return c.UpdateObject(ctx, obj)
	}
	deleteObject = func(ctx context.Context, c connection.RemoteClusterClient, obj client.Object) error {
		return c.DeleteObject(ctx, obj)
	}
)

// ReplicationGroupReconciler reconciles a ReplicationGroup object
type ReplicationGroupReconciler struct {
	client.Client
	Log                    logr.Logger
	Scheme                 *runtime.Scheme
	EventRecorder          record.EventRecorder
	PVCRequeueInterval     time.Duration
	Config                 connection.MultiClusterClient
	Domain                 string
	DisablePVCRemap        bool
	EnableKubevirtPVCRemap bool
}

// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsireplicationgroups,verbs=get;list;watch;update;patch;delete;create
// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsireplicationgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch

// Reconcile contains reconciliation logic that updates ReplicationGroup depending on it's current state
func (r *ReplicationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("dellcsireplicationgroup", req.Name)
	ctx = context.WithValue(ctx, logger.LoggerContextKey, log)

	localRG := new(repv1.DellCSIReplicationGroup)
	err := r.Get(ctx, req.NamespacedName, localRG)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(logger.InfoLevel).Info("Reconciling RG event!!!")
	localRGName := req.Name
	remoteRGName := localRG.Annotations[controller.RemoteReplicationGroup]
	if remoteRGName == "" {
		remoteRGName = localRGName
	}
	rgSyncComplete := false

	if localRG.Annotations == nil {
		log.V(logger.InfoLevel).Info("RG is not ready yet, requeue as we will get another event")
		return ctrl.Result{}, nil
	} else if localRG.Annotations[controller.RGSyncComplete] == "yes" {
		log.V(logger.DebugLevel).Info("RG Sync already completed")
		remoteRGName = localRG.Annotations[controller.RemoteReplicationGroup]
		rgSyncComplete = true
		// Continue as we can re verify
	}

	localClusterID := r.Config.GetClusterID()
	remoteClusterID := localRG.Spec.RemoteClusterID

	if remoteClusterID == controller.Self {
		localClusterID = controller.Self

		if !strings.HasPrefix(localRGName, replicated) {
			remoteRGName = replicated + "-" + localRGName
		}
	}

	annotations := make(map[string]string)
	annotations[controller.RemoteReplicationGroup] = localRGName
	annotations[controller.RemoteRGRetentionPolicy] = localRG.Annotations[controller.RemoteRGRetentionPolicy]
	annotations[controller.RemoteClusterID] = localClusterID

	labels := make(map[string]string)

	labels[controller.DriverName] = localRG.Labels[controller.DriverName]
	labels[controller.RemoteClusterID] = localClusterID

	// Apply driver specific labels
	remoteRGAttributes := localRG.Spec.RemoteProtectionGroupAttributes
	contextPrefix := localRG.Annotations[controller.ContextPrefix]
	if contextPrefix != "" {
		for k, v := range remoteRGAttributes {
			if strings.HasPrefix(k, contextPrefix) {
				labelKey := fmt.Sprintf("%s%s", r.Domain, strings.TrimPrefix(k, contextPrefix))
				labels[labelKey] = v
			}
		}
	}

	remoteRG := &repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        remoteRGName,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: repv1.DellCSIReplicationGroupSpec{
			DriverName:                      localRG.Spec.DriverName,
			Action:                          "",
			RemoteClusterID:                 localClusterID,
			ProtectionGroupID:               localRG.Spec.RemoteProtectionGroupID,
			ProtectionGroupAttributes:       localRG.Spec.RemoteProtectionGroupAttributes,
			RemoteProtectionGroupID:         localRG.Spec.ProtectionGroupID,
			RemoteProtectionGroupAttributes: localRG.Spec.ProtectionGroupAttributes,
		},
	}

	// Try to get the client
	remoteClient, err := r.Config.GetConnection(remoteClusterID)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check for RG retention policy annotation
	retentionPolicy, ok := localRG.Annotations[controller.RemoteRGRetentionPolicy]
	if !ok {
		log.Info(fmt.Sprintf("RetentionPolicy:found:%v,value-->%s", ok, retentionPolicy))
		log.Info("Retention policy not set, using retain as the default policy")
		retentionPolicy = controller.RemoteRetentionValueRetain // we will default to retain the RG if there is no retention policy is set
	}

	// Handle RG deletion if timestamp is set
	if !localRG.DeletionTimestamp.IsZero() {
		// Process deletion of remote RG
		log.V(logger.InfoLevel).Info("Deletion timestamp is not zero")
		log.V(logger.InfoLevel).WithValues(localRG.Annotations).Info("Annotations")
		_, ok := localRG.Annotations[controller.DeletionRequested]
		log.V(logger.InfoLevel).WithValues(ok).Info("Deletion requested?", ok)

		if _, ok := localRG.Annotations[controller.DeletionRequested]; !ok {
			log.V(logger.InfoLevel).Info("Deletion requested annotation not found")
			remoteRG, err := remoteClient.GetReplicationGroup(ctx, localRG.Annotations[controller.RemoteReplicationGroup])
			if err != nil {
				log.V(logger.ErrorLevel).WithValues(err.Error()).Info("error getting replication group")
				// If remote RG doesn't exist, proceed to removing finalizer
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to get remote replication group")
					return ctrl.Result{}, err
				}
			} else {
				log.V(logger.InfoLevel).Info("Got remote RG")
				if strings.ToLower(retentionPolicy) == controller.RemoteRetentionValueDelete {
					log.Info("Retention policy is set to Delete")
					if _, ok := remoteRG.Annotations[controller.DeletionRequested]; !ok {
						// Add annotation on the remote RG to request its deletion
						remoteRGCopy := remoteRG.DeepCopy()
						controller.AddAnnotation(remoteRGCopy, controller.DeletionRequested, "yes")
						err := remoteClient.UpdateReplicationGroup(ctx, remoteRGCopy)
						if err != nil {
							return ctrl.Result{}, err
						}
						// Resetting the rate-limiter to requeue for the deletion of remote RG
						return ctrl.Result{RequeueAfter: 1 * time.Millisecond}, nil
					}
					// Requeueing because the remote PV still exists
					return ctrl.Result{Requeue: true}, nil
				}
			}
		}

		log.V(logger.InfoLevel).Info("Removing finalizer RGFinalizer")
		finalizerRemoved := controller.RemoveFinalizerIfExists(localRG, controller.RGFinalizer)
		if finalizerRemoved {
			log.V(logger.InfoLevel).Info("Updating rg copy to remove finalizer")
			return ctrl.Result{}, r.Update(ctx, localRG)
		}
	}

	rgCopy := localRG.DeepCopy()

	log.V(logger.InfoLevel).Info("Adding finalizer RGFinalizer")
	// Check for the finalizer; add, if doesn't exist
	if finalizerAdded := controller.AddFinalizerIfNotExist(rgCopy, controller.RGFinalizer); finalizerAdded {
		log.V(logger.InfoLevel).Info("Finalizer not found adding it")
		return ctrl.Result{}, r.Update(ctx, rgCopy)
	}
	log.V(logger.InfoLevel).Info("Trying to delete RG if deletion request annotation found")
	// Check for deletion request annotation
	if _, ok := rgCopy.Annotations[controller.DeletionRequested]; ok {
		log.V(logger.InfoLevel).Info("Deletion Requested annotation found and deleting the remote RG")
		return ctrl.Result{}, r.Delete(ctx, rgCopy)
	}

	createRG := false

	// If the RG already exists on the Remote Cluster,
	// We treat this as idempotent.
	log.V(logger.InfoLevel).Info(fmt.Sprintf("Checking if remote RG with the name %s exists on ClusterId: %s",
		remoteRGName, remoteClusterID))
	rgObj, err := remoteClient.GetReplicationGroup(ctx, remoteRGName)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "failed to get RG details on the remote cluster")
		return ctrl.Result{Requeue: true}, err
	} else if errors.IsNotFound(err) {
		if rgSyncComplete {
			log.Error(err, "Something went wrong. Local RG has already been synced to the remote cluster")
			// If the RG has been successfully synced to the remote cluster once
			// and now it's not found,
			// Let's not recreate the RGs in this case.
			log.V(logger.InfoLevel).Info("RG not found on target cluster. " +
				"Since the local RG carries a SyncComplete annotation, " +
				"we will not be creating RG on remote once again.")
			return ctrl.Result{}, nil
		}
		// This is a special case. Controller tries to endlessly create
		// replicated RGs in single cluster scenario.
		// This check prevents controller from doing that.
		if strings.Contains(remoteRGName, "replicated-replicated") {
			createRG = false
		} else {
			createRG = true
		}
	} else {
		// We got the object
		log.V(logger.InfoLevel).Info(" The RG already exists on the remote cluster")
		// First verify the source cluster for this RG
		if rgObj.Spec.RemoteClusterID == localClusterID {
			// Confirmed that this object was created by this controller
			// Check other fields to see if this matches everything from our object
			// If fields don't match, then it could mean that this is a leftover object or someone edited it
			// Verify driver name
			if rgObj.Spec.DriverName != remoteRG.Spec.DriverName {
				// Lets create a new object
				remoteRGName = fmt.Sprintf("SourceClusterId-%s-%s", localClusterID, localRGName)
				remoteRG.Name = remoteRGName
				createRG = true
				rgSyncComplete = false
			} else {
				if rgObj.Spec.ProtectionGroupID != remoteRG.Spec.ProtectionGroupID ||
					rgObj.Spec.RemoteProtectionGroupID != remoteRG.Spec.RemoteProtectionGroupID {
					// Don't know how to proceed here
					// Lets raise an event and stop reconciling
					r.EventRecorder.Eventf(localRG, eventTypeWarning, eventReasonUpdated,
						"Found conflicting RG on remote ClusterId: %s", remoteClusterID)
					log.Error(fmt.Errorf("conflicting RG with name: %s exists on ClusterId: %s",
						localRGName, remoteClusterID), "stopping reconcile")
					return ctrl.Result{}, nil
				}
			}
		} else {
			// update the name of the RG and create it
			remoteRGName = fmt.Sprintf("SourceClusterId-%s-%s", localClusterID, localRGName)
			remoteRG.Name = remoteRGName
			createRG = true
			rgSyncComplete = false
		}
	}

	if createRG {
		err = remoteClient.CreateReplicationGroup(ctx, remoteRG)
		if err != nil {
			log.Error(err, "failed to create remote CR for DellCSIReplicationGroup")
			r.EventRecorder.Eventf(localRG, eventTypeWarning, eventReasonUpdated,
				"Failed to create remote CR for DellCSIReplicationGroup on ClusterId: %s", remoteClusterID)
			return ctrl.Result{}, err
		}
		log.V(logger.InfoLevel).Info("The remote RG has been successfully created!!")
		r.EventRecorder.Eventf(localRG, eventTypeNormal, eventReasonUpdated,
			"Created remote ReplicationGroup with name: %s on cluster: %s", remoteRGName, remoteClusterID)
	}

	// Update the RemoteReplicationGroup annotation on the local RG if required
	if !rgSyncComplete {
		if strings.Contains(localRGName, replicated) {
			remoteRGName = strings.TrimPrefix(localRGName, "replicated-")
		}
		controller.AddAnnotation(localRG, controller.RemoteReplicationGroup, remoteRGName)
		controller.AddAnnotation(localRG, controller.RGSyncComplete, "yes")
		err = r.Update(ctx, localRG)
		return ctrl.Result{}, err
	}

	err = r.processLastActionResult(ctx, localRG, remoteRG, remoteClient, log)
	if err != nil {
		log.V(logger.ErrorLevel).Error(err, "failed to process the last action")
		r.EventRecorder.Eventf(localRG, eventTypeWarning, eventReasonUpdated,
			"failed to process the last action %s", localRG.Status.LastAction.Condition)
	}

	log.V(logger.InfoLevel).Info("RG has already been synced to the remote cluster")
	return ctrl.Result{}, nil
}

func (r *ReplicationGroupReconciler) processLastActionResult(ctx context.Context, group *repv1.DellCSIReplicationGroup, remoteGroup *repv1.DellCSIReplicationGroup, remoteClient connection.RemoteClusterClient, log logr.Logger) error {
	if len(group.Status.Conditions) == 0 || group.Status.LastAction.Time == nil {
		log.V(logger.InfoLevel).Info("No action to process")
		return nil
	}

	if group.Status.LastAction.ErrorMessage != "" {
		return fmt.Errorf("last action failed: %s", group.Status.LastAction.Condition)
	}

	val, ok := group.Annotations[controller.ActionProcessedTime]
	if !ok {
		log.V(logger.InfoLevel).Info("Action Processed does not exist.")
		return nil
	}

	if val == group.Status.LastAction.Time.GoString() {
		log.V(logger.InfoLevel).Info("Last action has already been processed")
		return nil
	}

	if strings.Contains(group.Status.LastAction.Condition, "CREATE_SNAPSHOT") {
		if err := getDellCsiReplicationGroupProcessSnapshotEvent(r, ctx, group, remoteClient, log); err != nil {
			return err
		}
	}

	if strings.Contains(group.Status.LastAction.Condition, "FAILOVER_REMOTE") {
		if err := r.processFailoverEvent(ctx, group, remoteClient, log); err != nil {
			return err
		}
	}

	if strings.Contains(group.Status.LastAction.Condition, "UNPLANNED_FAILOVER_LOCAL") {
		if err := r.processFailoverEvent(ctx, remoteGroup, remoteClient, log); err != nil {
			return err
		}
	}

	if strings.Contains(group.Status.LastAction.Condition, "FAILBACK_LOCAL") {
		if err := r.processFailBackEvent(ctx, remoteGroup, remoteClient, log); err != nil {
			return err
		}
	}

	if strings.Contains(group.Status.LastAction.Condition, "ACTION_FAILBACK_DISCARD_CHANGES_LOCAL") {
		if err := r.processFailBackEvent(ctx, remoteGroup, remoteClient, log); err != nil {
			return err
		}
	}

	// Informing the RG that the last action has been processed.
	controller.AddAnnotation(group, controller.ActionProcessedTime, group.Status.LastAction.Time.GoString())

	return getDellCsiReplicationGroupUpdate(r, ctx, group)
}

func (r *ReplicationGroupReconciler) processFailoverEvent(ctx context.Context, group *repv1.DellCSIReplicationGroup, remoteClient connection.RemoteClusterClient, log logr.Logger) error {
	if r.DisablePVCRemap {
		return nil
	}
	remoteClusterID := group.Annotations[controller.RemoteClusterID]
	if remoteClusterID == "self" {
		rgName := group.Name
		rgTarget := group.Annotations[controller.RemoteReplicationGroup]

		err := r.swapAllPVC(ctx, remoteClient, rgName, rgTarget, log)
		if err != nil {
			log.Error(err, "Error swapping all PVCs")
			return err
		}
	}
	return nil
}

func (r *ReplicationGroupReconciler) processFailBackEvent(ctx context.Context, group *repv1.DellCSIReplicationGroup, remoteClient connection.RemoteClusterClient, log logr.Logger) error {
	if r.DisablePVCRemap {
		return nil
	}
	remoteClusterID := group.Annotations[controller.RemoteClusterID]
	if remoteClusterID == "self" {
		rgName := group.Name
		rgTarget := group.Annotations[controller.RemoteReplicationGroup]

		err := r.swapAllPVC(ctx, remoteClient, rgName, rgTarget, log)
		if err != nil {
			log.Error(err, "Error swapping all PVCs")
			return err
		}
	}
	return nil
}

func (r *ReplicationGroupReconciler) processSnapshotEvent(ctx context.Context, group *repv1.DellCSIReplicationGroup, remoteClient connection.RemoteClusterClient, log logr.Logger) error {
	lastAction := group.Status.LastAction

	val, ok := group.Annotations[csireplicator.Action]
	if !ok {
		log.V(logger.InfoLevel).Info("No action", "val", val)
		return nil
	}

	var actionAnnotation csireplicator.ActionAnnotation
	err := json.Unmarshal([]byte(val), &actionAnnotation)
	if err != nil {
		log.Error(err, "JSON unmarshal error", "actionAnnotation", actionAnnotation)
		return err
	}

	if _, err := getDellCsiReplicationGroupGetSnapshotClass(remoteClient, ctx, actionAnnotation.SnapshotClass); err != nil {
		log.Error(err, "Snapshot class does not exist on remote cluster. Not creating the remote snapshots.")
		return err
	}

	if _, err := getDellCsiReplicationGroupGetNamespace(remoteClient, ctx, actionAnnotation.SnapshotNamespace); err != nil {
		log.V(logger.InfoLevel).Info("Namespace - " + actionAnnotation.SnapshotNamespace + " not found, creating it.")
		nsRef := makeNamespaceReference(actionAnnotation.SnapshotNamespace)

		err = getDellCsiReplicationGroupCreateNamespace(remoteClient, ctx, nsRef)
		if err != nil {
			msg := "unable to create the desired namespace" + actionAnnotation.SnapshotNamespace
			log.V(logger.ErrorLevel).Error(err, msg)
			return err
		}
	}

	for volumeHandle, snapshotHandle := range lastAction.ActionAttributes {
		msg := "ActionAttributes - volumeHandle: " + volumeHandle + ", snapshotHandle: " + snapshotHandle
		log.V(logger.InfoLevel).Info(msg)

		snapRef := makeSnapReference(snapshotHandle, actionAnnotation.SnapshotNamespace)
		sc := makeStorageClassContent(group.Labels[controller.DriverName], actionAnnotation.SnapshotClass)
		snapContent := makeVolSnapContent(snapshotHandle, volumeHandle, *snapRef, sc)

		err = getDellCsiReplicationGroupCreateSnapshotContent(remoteClient, ctx, snapContent)
		if err != nil {
			log.Error(err, "unable to create snapshot content")
			return err
		}

		snapshot := makeSnapshotObject(snapRef.Name, snapContent.Name, sc.ObjectMeta.Name, actionAnnotation.SnapshotNamespace)
		err = getDellCsiReplicationGroupCreateSnapshotObject(remoteClient, ctx, snapshot)
		if err != nil {
			log.Error(err, "unable to create snapshot object")
			return err
		}
	}

	return nil
}

func makeNamespaceReference(namespace string) *v1.Namespace {
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
}

func makeSnapReference(snapName, namespace string) *v1.ObjectReference {
	return &v1.ObjectReference{
		Kind:       "VolumeSnapshot",
		APIVersion: "snapshot.storage.k8s.io/v1",
		Name:       "snapshot-" + snapName,
		Namespace:  namespace,
	}
}

func makeSnapshotObject(snapName, contentName, className, namespace string) *s1.VolumeSnapshot {
	volsnap := &s1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapName,
			Namespace: namespace,
		},
		Spec: s1.VolumeSnapshotSpec{
			Source: s1.VolumeSnapshotSource{
				VolumeSnapshotContentName: &contentName,
			},
			VolumeSnapshotClassName: &className,
		},
	}
	return volsnap
}

func makeStorageClassContent(driver, snapClass string) *s1.VolumeSnapshotClass {
	return &s1.VolumeSnapshotClass{
		Driver:         driver,
		DeletionPolicy: "Retain",
		ObjectMeta: metav1.ObjectMeta{
			Name: snapClass,
		},
	}
}

func makeVolSnapContent(snapName, volumeName string, snapRef v1.ObjectReference, sc *s1.VolumeSnapshotClass) *s1.VolumeSnapshotContent {
	volsnapcontent := &s1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "volume-" + volumeName + "-" + strconv.FormatInt(time.Now().Unix(), 10),
		},
		Spec: s1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: snapRef,
			Source: s1.VolumeSnapshotContentSource{
				SnapshotHandle: &snapName,
			},
			VolumeSnapshotClassName: &sc.Name,
			DeletionPolicy:          sc.DeletionPolicy,
			Driver:                  sc.Driver,
		},
	}
	return volsnapcontent
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *ReplicationGroupReconciler) SetupWithManager(mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&repv1.DellCSIReplicationGroup{}).
		WithOptions(reconciler.Options{
			RateLimiter:             limiter,
			MaxConcurrentReconciles: maxReconcilers,
		}).
		Complete(r)
}

// pvcSwapBackup stores PVC details and original PV reclaim policies for crash recovery during PVC swap.
// It is serialized as JSON and stored as an annotation on the local PV before any destructive action.
type pvcSwapBackup struct {
	PVC            *v1.PersistentVolumeClaim        `json:"pvc"`
	LocalPVPolicy  v1.PersistentVolumeReclaimPolicy `json:"localPVPolicy"`
	RemotePVPolicy v1.PersistentVolumeReclaimPolicy `json:"remotePVPolicy"`
}

// savePVCBackupToPV persists the PVC swap backup as a JSON annotation on the local PV.
// It also sets the PV reclaim policy to Retain in the same update to minimize API calls.
func savePVCBackupToPV(ctx context.Context, c connection.RemoteClusterClient, localPV *v1.PersistentVolume, backup *pvcSwapBackup, log logr.Logger) error {
	data, err := json.Marshal(backup)
	if err != nil {
		return fmt.Errorf("error marshaling PVC backup: %w", err)
	}
	if localPV.Annotations == nil {
		localPV.Annotations = make(map[string]string)
	}
	localPV.Annotations[controller.PendingPVCSwap] = string(data)
	localPV.Spec.PersistentVolumeReclaimPolicy = v1.PersistentVolumeReclaimRetain
	if err := updatePersistentVolume(ctx, c, localPV); err != nil {
		return fmt.Errorf("error saving PVC backup to PV %s: %w", localPV.Name, err)
	}
	log.V(logger.InfoLevel).Info(fmt.Sprintf("Saved PVC swap backup to PV %s and set Retain policy", localPV.Name))
	return nil
}

// recoverPVCBackup attempts to recover a PVC swap backup from the local PV.
// It uses the target PV's RemotePV annotation to find the local PV name.
func recoverPVCBackup(ctx context.Context, c connection.RemoteClusterClient, targetPV string, log logr.Logger) (*pvcSwapBackup, error) {
	remotePV, err := getPersistentVolume(ctx, c, targetPV)
	if err != nil {
		return nil, fmt.Errorf("error getting target PV %s for recovery: %w", targetPV, err)
	}
	localPVName := remotePV.Annotations[controller.RemotePV]
	if localPVName == "" {
		return nil, fmt.Errorf("target PV %s has no RemotePV annotation", targetPV)
	}
	localPV, err := getPersistentVolume(ctx, c, localPVName)
	if err != nil {
		return nil, fmt.Errorf("error getting local PV %s for recovery: %w", localPVName, err)
	}
	backupJSON, ok := localPV.Annotations[controller.PendingPVCSwap]
	if !ok {
		return nil, fmt.Errorf("local PV %s has no pending PVC swap annotation", localPVName)
	}
	var backup pvcSwapBackup
	if err := json.Unmarshal([]byte(backupJSON), &backup); err != nil {
		return nil, fmt.Errorf("error unmarshaling PVC backup from PV %s: %w", localPVName, err)
	}
	log.V(logger.InfoLevel).Info(fmt.Sprintf("Recovered PVC swap backup from PV %s", localPVName))
	return &backup, nil
}

// Give a replication group name and target, swapAllPVC reassigns the PVC from local volume to remote volume.
// It also retains the original reclaimPolicy and operates within a single cluster.
// After processing listed PVCs, it checks for orphaned PVs with pending swap annotations
// to recover PVCs that were mid-swap when the controller crashed.
func (r *ReplicationGroupReconciler) swapAllPVC(ctx context.Context, c connection.RemoteClusterClient, rgName string, rgTarget string, log logr.Logger) error {
	pvcs, err := c.ListPersistentVolumeClaim(ctx, client.MatchingLabels{controller.ReplicationGroup: rgName})
	if err != nil {
		return fmt.Errorf("failed to list PVCs: %w", err)
	}

	// Build set of PVC names being swapped (to detect orphans later)
	swappedPVCNames := make(map[string]bool)

	var wg sync.WaitGroup
	errChan := make(chan error, len(pvcs.Items))

	for _, pvc := range pvcs.Items {
		swappedPVCNames[pvc.Namespace+"/"+pvc.Name] = true
		wg.Add(1)
		go func(pvc v1.PersistentVolumeClaim) {
			defer wg.Done()
			err := r.swapPVC(ctx, c, pvc.Name, pvc.Namespace, pvc.Annotations[controller.RemotePV], rgTarget, log)
			if err != nil {
				errChan <- fmt.Errorf("error swapping PVC %s/%s: %s", pvc.Namespace, pvc.Name, err)
			}
		}(pvc)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	// Recovery: check for PVs with pending swap annotations whose PVCs were not in the list.
	// This handles the case where the controller crashed after PVC deletion but before recreation.
	var pvList v1.PersistentVolumeList
	if listErr := r.Client.List(ctx, &pvList, client.MatchingLabels{controller.ReplicationGroup: rgName}); listErr == nil {
		for i := range pvList.Items {
			pv := &pvList.Items[i]
			backupJSON, ok := pv.Annotations[controller.PendingPVCSwap]
			if !ok {
				continue
			}
			var backup pvcSwapBackup
			if err := json.Unmarshal([]byte(backupJSON), &backup); err != nil {
				log.V(logger.WarnLevel).Info(fmt.Sprintf("Failed to unmarshal PVC backup from PV %s: %v", pv.Name, err))
				continue
			}
			key := backup.PVC.Namespace + "/" + backup.PVC.Name
			if swappedPVCNames[key] {
				continue // already handled in the main loop
			}
			targetPV := backup.PVC.Annotations[controller.RemotePV]
			log.V(logger.InfoLevel).Info(fmt.Sprintf("Recovering orphaned PVC swap for %s from PV %s", key, pv.Name))
			if err := r.swapPVC(ctx, c, backup.PVC.Name, backup.PVC.Namespace, targetPV, rgTarget, log); err != nil {
				errs = append(errs, fmt.Errorf("error recovering PVC swap %s: %s", key, err))
			}
		}
	} else {
		log.V(logger.WarnLevel).Info(fmt.Sprintf("Failed to list PVs for swap recovery: %v", listErr))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors occurred while swapping PVCs: %s", errs)
	}

	return nil
}

func (r *ReplicationGroupReconciler) swapPVC(ctx context.Context, client connection.RemoteClusterClient, pvcName, namespace, targetPV, rgTarget string, log logr.Logger) error {
	var pvc *v1.PersistentVolumeClaim
	var localPVPolicy, remotePVPolicy v1.PersistentVolumeReclaimPolicy
	recovered := false

	// Read the PVC
	pvcFetched, err := getPersistentVolumeClaim(ctx, client, namespace, pvcName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("error getting pvc %s: %s", pvcName, err)
		}
		// PVC not found — attempt recovery from local PV backup annotation
		backup, recoveryErr := recoverPVCBackup(ctx, client, targetPV, log)
		if recoveryErr != nil {
			return fmt.Errorf("PVC %s/%s not found and recovery failed: %w", namespace, pvcName, recoveryErr)
		}
		pvc = backup.PVC
		localPVPolicy = backup.LocalPVPolicy
		remotePVPolicy = backup.RemotePVPolicy
		recovered = true
		log.V(logger.InfoLevel).Info(fmt.Sprintf("Recovered PVC %s/%s from PV backup, skipping to recreation", namespace, pvcName))
	} else {
		pvc = pvcFetched

		// If KubeVirt PVC remap is disabled but the PVC is owned by a DataVolume,
		// skip the swap entirely — we cannot safely delete a DV-owned PVC without
		// first handling the DataVolume dependency.
		if !r.EnableKubevirtPVCRemap && findDataVolumeOwnerRef(pvc.OwnerReferences) != nil {
			log.V(logger.WarnLevel).Info(fmt.Sprintf("PVC %s/%s is owned by a DataVolume but KubeVirt PVC remap is disabled; skipping PVC swap",
				pvc.Namespace, pvc.Name))
			return nil
		}
	}

	if !recovered {
		// Normal flow: read PVs, save backup, set Retain, handle DV, delete/wait

		// Save the Reclaim Policy for both PVs
		localPV, err := getPersistentVolume(ctx, client, pvc.Spec.VolumeName)
		if err != nil {
			return fmt.Errorf("error retrieving local PV %s", pvc.Spec.VolumeName)
		}
		localPVPolicy = localPV.Spec.PersistentVolumeReclaimPolicy

		remotePV, err := getPersistentVolume(ctx, client, pvc.Annotations[controller.RemotePV])
		if err != nil {
			return fmt.Errorf("error retrieving remote PV %s", pvc.Annotations[controller.RemotePV])
		}

		// The target PV should be unclaimed.
		if remotePV.Spec.ClaimRef != nil {
			if remotePV.Spec.ClaimRef.Name == controller.ReservedPVCName && remotePV.Spec.ClaimRef.Namespace == controller.ReservedPVCNamespace {
				err = removeReservedClaimRefForTargetPV(ctx, client, remotePV.Name, log)
				if err != nil {
					return fmt.Errorf("error removing PV claim ref from %s: %s", remotePV, err.Error())
				}
			} else {
				// During recovery, the PVC might be deleted but the PV still has the ClaimRef.
				// Check if the PVC still exists before failing.
				_, pvcErr := getPersistentVolumeClaim(ctx, client, remotePV.Spec.ClaimRef.Namespace, remotePV.Spec.ClaimRef.Name)
				if pvcErr != nil && errors.IsNotFound(pvcErr) {
					// PVC doesn't exist, safe to remove the ClaimRef
					log.V(logger.InfoLevel).Info(fmt.Sprintf("PVC %s/%s not found, removing stale ClaimRef from target PV %s",
						remotePV.Spec.ClaimRef.Namespace, remotePV.Spec.ClaimRef.Name, remotePV.Name))
					remotePV.Spec.ClaimRef = nil
					if err := updatePersistentVolume(ctx, client, remotePV); err != nil {
						return fmt.Errorf("error removing stale ClaimRef from target PV %s: %s", remotePV.Name, err)
					}
				} else {
					return fmt.Errorf("target PV %s is claimed by PVC %s/%s", remotePV.Name,
						remotePV.Spec.ClaimRef.Namespace, remotePV.Spec.ClaimRef.Name)
				}
			}
		}

		remotePVPolicy = remotePV.Spec.PersistentVolumeReclaimPolicy

		// Save PVC backup to local PV and set Retain in a single update.
		// This persists the PVC details so they survive a pod crash.
		backup := &pvcSwapBackup{
			PVC:            pvc,
			LocalPVPolicy:  localPVPolicy,
			RemotePVPolicy: remotePVPolicy,
		}
		if err := savePVCBackupToPV(ctx, client, localPV, backup, log); err != nil {
			return err
		}

		// Make the remote PV reclaim policy Retain
		if err = setPVReclaimPolicy(ctx, client, pvc.Annotations[controller.RemotePV], "Retain"); err != nil {
			return fmt.Errorf("error making target PV %s reclaim policy Retain: %s", pvc.Annotations[controller.RemotePV], err)
		}

		// Handle DataVolume dependencies before PVC deletion.
		// If the DV was deleted, Kubernetes GC will cascade-delete the PVC.
		// If no DV was involved, we delete the PVC explicitly below.
		dvDeleted := false
		if r.EnableKubevirtPVCRemap {
			var dvErr error
			dvDeleted, dvErr = r.handleDataVolumeDependencies(ctx, client, pvc, log)
			if dvErr != nil {
				return dvErr
			}
		}

		if !dvDeleted {
			// No DV involvement — delete the PVC explicitly
			err = deletePersistentVolumeClaim(ctx, client, pvc)
			if err != nil {
				return fmt.Errorf("error deleting PVC %s with errror %s", pvcName, err)
			}
		}

		// Wait until PVC is deleted (either by GC cascade or explicit delete above)
		done := false
		for iteration := 0; !done; iteration++ {
			sleep(2 * time.Second)
			_, err := getPersistentVolumeClaim(ctx, client, namespace, pvcName)
			if err != nil {
				if errors.IsNotFound(err) {
					done = true
					break
				}

				return fmt.Errorf("error when waiting for PVC %s/%s to be deleted", namespace, pvcName)
			}

			if iteration > 30 {
				return fmt.Errorf("timed out waiting on PVC %s/%s to be deleted", namespace, pvcName)
			}
		}
	}

	// --- Common recreation path (normal + recovery) ---

	// Swap some fields in the PVC (using the in-memory or recovered copy).
	localPV := pvc.Spec.VolumeName
	pvc.Annotations[controller.RemotePV] = pvc.Spec.VolumeName
	pvc.Spec.VolumeName = targetPV

	remoteStorageClassName := pvc.Annotations[controller.StorageClassRemoteStorageClassParam]
	pvc.Annotations[controller.StorageClassRemoteStorageClassParam] = *pvc.Spec.StorageClassName
	pvc.Annotations[controller.ReplicationGroup] = rgTarget
	pvc.Labels[controller.ReplicationGroup] = rgTarget
	pvc.Spec.StorageClassName = &remoteStorageClassName
	pvc.ObjectMeta.ResourceVersion = ""

	// Strip DataVolume/CDI artifacts from the PVC before recreation so the new PVC
	// is not tied to the deleted DataVolume or CDI import sources.
	if r.EnableKubevirtPVCRemap {
		pvc.OwnerReferences = removeDataVolumeOwnerRef(pvc.OwnerReferences)
		removeCDIAnnotations(pvc)
	}
	pvc.Spec.DataSource = nil
	pvc.Spec.DataSourceRef = nil

	// Remove the pending swap annotation before recreation
	delete(pvc.Annotations, controller.PendingPVCSwap)

	// Re-create the PVC, now pointing to the target.
	err = createPersistentVolumeClaim(ctx, client, pvc)
	if err != nil {
		return fmt.Errorf("unable to create PVC %s: %s", pvc.Name, err.Error())
	}

	// Update the target PV's claimref to point to our pvc.
	pvcUID := pvc.ObjectMeta.UID
	pvcResourceVersion := pvc.ObjectMeta.ResourceVersion
	err = updatePVClaimRef(ctx, client, targetPV, pvc.Namespace, pvcResourceVersion, pvc.Name, pvcUID, log)
	if err != nil {
		return fmt.Errorf("unable to update PV ClaimRef for %s: %s", targetPV, err.Error())
	}

	// Verify pvc is created and bound to new PVs
	// remotePV is the current localPVName arg, localPV is the current remotePVName arg
	err = verifyPVC(ctx, client, targetPV, localPV, pvcName, namespace)
	if err != nil {
		return fmt.Errorf("error verifying PVC %s: %s", pvcName, err.Error())
	}

	// Remove the PVC reclaim of local PV
	err = removePVClaimRef(ctx, client, localPV, namespace, pvcName, log)
	if err != nil {
		return fmt.Errorf("error removing PV claim ref from %s: %s", localPV, err.Error())
	}

	// Updating the claimRef of localPV to reserved/reserved and clear backup annotation
	pv, err := getPersistentVolume(ctx, client, localPV)
	if err != nil {
		return fmt.Errorf("error retrieving PV %s: %s", localPV, err.Error())
	}

	claimRef := &v1.ObjectReference{
		APIVersion: "v1",
		Kind:       "PersistentVolumeClaim",
		Name:       controller.ReservedPVCName,
		Namespace:  controller.ReservedPVCNamespace,
	}
	pv.Spec.ClaimRef = claimRef
	delete(pv.Annotations, controller.PendingPVCSwap)

	err = updatePersistentVolume(ctx, client, pv)
	if err != nil {
		return fmt.Errorf("error updating PV %s: %s", localPV, err.Error())
	}
	log.V(logger.InfoLevel).Info(fmt.Sprintf("added remote pv %s claimref %s/%s", pv.Name, controller.ReservedPVCName, controller.ReservedPVCNamespace))

	// Restore the PVs original volume reclaim policy
	err = setPVReclaimPolicy(ctx, client, pvc.Spec.VolumeName, remotePVPolicy)
	if err != nil {
		return fmt.Errorf("restoring source PV reclaim policy of %s: %s", pvc.Spec.VolumeName, err.Error())
	}
	err = setPVReclaimPolicy(ctx, client, pvc.Annotations[controller.RemotePV], localPVPolicy)
	if err != nil {
		return fmt.Errorf("restoring remote PV reclaim policy of %s: %s", pvc.Annotations[controller.RemotePV], err.Error())
	}

	return nil
}

func verifyPVC(ctx context.Context, client connection.RemoteClusterClient, localPVName string, remotePVName string, pvcName string, namespace string) error {
	for iteration := 0; iteration < 30; iteration++ {
		pvc, err := getPersistentVolumeClaim(ctx, client, namespace, pvcName)

		if (err == nil) && (localPVName == pvc.Spec.VolumeName) && (remotePVName == pvc.Annotations[controller.RemotePV]) {
			return nil
		}

		sleep(2 * time.Second)
	}

	return fmt.Errorf("timed out waiting on PVC %s/%s to be created and bound", namespace, pvcName)
}

func setPVReclaimPolicy(ctx context.Context, client connection.RemoteClusterClient, pvName string, prevPolicy v1.PersistentVolumeReclaimPolicy) error {
	for iteration := 0; iteration < 30; iteration++ {
		pv, err := getPersistentVolume(ctx, client, pvName)
		if err != nil {
			return fmt.Errorf("error retrieving PV %s: %s", pvName, err.Error())
		}

		pv.Spec.PersistentVolumeReclaimPolicy = prevPolicy
		err = updatePersistentVolume(ctx, client, pv)
		if err != nil {
			return fmt.Errorf("error updating PV %s: %s", pvName, err.Error())
		}

		pv, err = getPersistentVolume(ctx, client, pvName)
		if err != nil {
			return fmt.Errorf("error retrieving PV %s: %s", pvName, err.Error())
		}

		if pv.Spec.PersistentVolumeReclaimPolicy == prevPolicy {
			return nil
		}

		sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting on PV VolumeReclaimPolicy to be set to previous policy")
}

func removePVClaimRef(ctx context.Context, client connection.RemoteClusterClient, pvName, pvcNamespace, pvcName string, log logr.Logger) error {
	log.V(logger.InfoLevel).Info(fmt.Sprintf("Removing ClaimRef on LocalPV: %s", pvName))
	for iteration := 0; iteration < 30; iteration++ {
		pv, err := getPersistentVolume(ctx, client, pvName)
		if err != nil {
			return fmt.Errorf("error retrieving PV %s: %s", pvName, err.Error())
		}

		if pv.Spec.ClaimRef == nil {
			log.V(logger.InfoLevel).Info(fmt.Sprintf("ClaimRef removed from LocalPV: %s", pvName))
			return nil
		}
		pv.Spec.ClaimRef = nil
		pv.Annotations[controller.RemotePVCNamespace] = pvcNamespace
		pv.Labels[controller.RemotePVCNamespace] = pvcNamespace
		pv.Annotations[controller.RemotePVC] = pvcName

		err = updatePersistentVolume(ctx, client, pv)
		if err != nil {
			if !errors.IsConflict(err) {
				return fmt.Errorf("error updating PV %s: %s", pvName, err.Error())
			}

			log.V(logger.InfoLevel).Info(fmt.Sprintf("Issue updating PV %s so trying again", pvName))
			sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("timed out waiting on Local PV Claim Ref to be removed")
}

func removeReservedClaimRefForTargetPV(ctx context.Context, client connection.RemoteClusterClient, pvName string, log logr.Logger) error {
	log.V(logger.InfoLevel).Info(fmt.Sprintf("Removing ClaimRef on Target PV: %s", pvName))
	for iteration := 0; iteration < 30; iteration++ {
		log.V(logger.DebugLevel).Info(fmt.Sprintf("*** ITERATION: %d ***", iteration))
		pv, err := getPersistentVolume(ctx, client, pvName)
		if err != nil {
			return fmt.Errorf("error retrieving PV %s: %s", pvName, err.Error())
		}

		if pv.Spec.ClaimRef == nil {
			log.V(logger.InfoLevel).Info(fmt.Sprintf("Reserved ClaimRef removed from TargetPV: %s", pvName))
			return nil
		}

		pv.Spec.ClaimRef = nil
		err = updatePersistentVolume(ctx, client, pv)
		if err != nil {
			if !errors.IsConflict(err) {
				return fmt.Errorf("error updating PV %s: %s", pvName, err.Error())
			}

			log.V(logger.InfoLevel).Info(fmt.Sprintf("Issue updating PV %s so trying again", pvName))
			sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("timed out waiting on Target PV Reserved Claim Ref to be removed")
}

func updatePVClaimRef(ctx context.Context, client connection.RemoteClusterClient, pvName, pvcNamespace, pvcResourceVersion, pvcName string, pvcUID types.UID, log logr.Logger) error {
	for iteration := 0; iteration < 30; iteration++ {
		pv, err := getPersistentVolume(ctx, client, pvName)
		if err != nil {
			log.V(logger.InfoLevel).Info(fmt.Sprintf("Error retrieving PV %s: %s", pvName, err.Error()))

			return err
		}

		if pv.Spec.ClaimRef != nil {
			return nil
		}

		pv.Spec.ClaimRef = &v1.ObjectReference{
			Kind:            "PersistentVolumeClaim",
			Namespace:       pvcNamespace,
			Name:            pvcName,
			UID:             pvcUID,
			ResourceVersion: pvcResourceVersion,
		}

		if val, exists := pv.Annotations[controller.RemotePVCNamespace]; exists && val != "" {
			pv.Annotations[controller.RemotePVCNamespace] = ""
		}

		if val, exists := pv.Labels[controller.RemotePVCNamespace]; exists && val != "" {
			pv.Labels[controller.RemotePVCNamespace] = ""
		}

		if val, exists := pv.Annotations[controller.RemotePVC]; exists && val != "" {
			pv.Annotations[controller.RemotePVC] = ""
		}

		err = updatePersistentVolume(ctx, client, pv)
		if err != nil {
			if !errors.IsConflict(err) {
				return fmt.Errorf("error updating PV %s: %s", pvName, err.Error())
			}

			log.V(logger.InfoLevel).Info(fmt.Sprintf("Issue retrieving latest for %s and trying again", pvName))
			sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("timed out updating the claim ref")
}

// handleDataVolumeDependencies detects and handles DataVolume ownership on the PVC
// before PVC deletion during remap. Returns dvDeleted=true if a DataVolume was deleted,
// meaning the PVC will be garbage-collected by Kubernetes and should not be deleted explicitly.
//
//  1. If the PVC has no DataVolume ownerRef: return (false, nil).
//  2. If the PVC has a DataVolume ownerRef, fetch the DV.
//     a. If the DV is not found: return (false, nil) — DV already gone, PVC may still exist.
//     b. If the DV is owned by a VirtualMachine: return (false, error) — skip cleanup.
//     c. If the DV is NOT owned by a VM: remove finalizers, delete DV, return (true, nil).
//
// When dvDeleted=true, the caller should NOT explicitly delete the PVC. Kubernetes garbage
// collection will cascade-delete the PVC via the DV ownerReference. The caller should wait
// for PVC deletion before recreating it.
func (r *ReplicationGroupReconciler) handleDataVolumeDependencies(ctx context.Context, c connection.RemoteClusterClient, pvc *v1.PersistentVolumeClaim, log logr.Logger) (bool, error) {
	dvOwnerRef := findDataVolumeOwnerRef(pvc.OwnerReferences)
	if dvOwnerRef == nil {
		return false, nil
	}

	log.V(logger.InfoLevel).Info(fmt.Sprintf("PVC %s/%s has DataVolume ownerRef %s, fetching DV to check ownership",
		pvc.Namespace, pvc.Name, dvOwnerRef.Name))

	// Fetch the DataVolume to inspect its ownerReferences
	dv := &unstructured.Unstructured{}
	dv.SetGroupVersionKind(schema.FromAPIVersionAndKind(dvOwnerRef.APIVersion, dvOwnerRef.Kind))
	err := getObject(ctx, c, client.ObjectKey{Name: dvOwnerRef.Name, Namespace: pvc.Namespace}, dv)
	if err != nil {
		if errors.IsNotFound(err) {
			// DV already deleted — PVC may still exist, caller should delete it explicitly
			log.V(logger.InfoLevel).Info(fmt.Sprintf("DataVolume %s/%s not found, PVC deletion will be handled by caller",
				pvc.Namespace, dvOwnerRef.Name))
			return false, nil
		}
		return false, fmt.Errorf("error fetching DataVolume %s/%s: %w", pvc.Namespace, dvOwnerRef.Name, err)
	}

	// Check if the DV is owned by a VirtualMachine
	vmOwnerRef := findVMOwnerRef(dv.GetOwnerReferences())
	if vmOwnerRef != nil {
		msg := fmt.Sprintf("PVC %s/%s is backed by DataVolume %s owned by VirtualMachine %s; skipping cleanup — "+
			"stop the VM and remove the dataVolumeTemplate before retrying PVC remap",
			pvc.Namespace, pvc.Name, dvOwnerRef.Name, vmOwnerRef.Name)
		log.V(logger.WarnLevel).Info(msg)
		r.EventRecorder.Event(pvc, eventTypeWarning, "VirtualMachineOwnedDataVolume", msg)
		return false, fmt.Errorf("%s", msg)
	}

	// DV is NOT owned by a VM — remove finalizers and delete DV.
	// Kubernetes GC will cascade-delete the PVC via the ownerReference.
	log.V(logger.InfoLevel).Info(fmt.Sprintf("DataVolume %s/%s is standalone (not VM-owned), removing finalizers and deleting",
		pvc.Namespace, dvOwnerRef.Name))

	// Remove finalizers from DV so deletion is immediate
	if len(dv.GetFinalizers()) > 0 {
		log.V(logger.InfoLevel).Info(fmt.Sprintf("Removing finalizers %v from DataVolume %s/%s",
			dv.GetFinalizers(), pvc.Namespace, dvOwnerRef.Name))
		dv.SetFinalizers(nil)
		if err := updateObject(ctx, c, dv); err != nil {
			return false, fmt.Errorf("error removing finalizers from DataVolume %s/%s: %w",
				pvc.Namespace, dvOwnerRef.Name, err)
		}
	}

	if err := deleteObject(ctx, c, dv); err != nil {
		if !errors.IsNotFound(err) {
			return false, fmt.Errorf("error deleting DataVolume %s/%s: %w", pvc.Namespace, dvOwnerRef.Name, err)
		}
	}

	log.V(logger.InfoLevel).Info(fmt.Sprintf("Deleted DataVolume %s/%s; PVC will be garbage-collected",
		pvc.Namespace, dvOwnerRef.Name))
	return true, nil
}

// findDataVolumeOwnerRef returns the first OwnerReference that points to a DataVolume (cdi.kubevirt.io).
func findDataVolumeOwnerRef(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i, ref := range refs {
		if strings.Contains(ref.APIVersion, "cdi.kubevirt.io") && ref.Kind == "DataVolume" {
			return &refs[i]
		}
	}
	return nil
}

// findVMOwnerRef returns the first OwnerReference that points to a VirtualMachine (kubevirt.io).
func findVMOwnerRef(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i, ref := range refs {
		if strings.Contains(ref.APIVersion, "kubevirt.io") && ref.Kind == "VirtualMachine" {
			return &refs[i]
		}
	}
	return nil
}

// removeDataVolumeOwnerRef returns the ownerReferences list with all DataVolume (cdi.kubevirt.io) entries removed.
func removeDataVolumeOwnerRef(refs []metav1.OwnerReference) []metav1.OwnerReference {
	filtered := make([]metav1.OwnerReference, 0, len(refs))
	for _, ref := range refs {
		if strings.Contains(ref.APIVersion, "cdi.kubevirt.io") && ref.Kind == "DataVolume" {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

// removeCDIAnnotations removes all CDI-related annotations from the PVC.
// CDI annotations use the "cdi.kubevirt.io" prefix.
func removeCDIAnnotations(pvc *v1.PersistentVolumeClaim) {
	if pvc.Annotations == nil {
		return
	}
	for key := range pvc.Annotations {
		if strings.Contains(key, "cdi.kubevirt.io") {
			delete(pvc.Annotations, key)
		}
	}
}
