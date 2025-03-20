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
	"time"

	csireplicator "github.com/dell/csm-replication/controllers/csi-replicator"
	"github.com/dell/csm-replication/pkg/common"

	repv1 "github.com/dell/csm-replication/api/v1"
	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/connection"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
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
)

// ReplicationGroupReconciler reconciles a ReplicationGroup object
type ReplicationGroupReconciler struct {
	client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme
	EventRecorder      record.EventRecorder
	PVCRequeueInterval time.Duration
	Config             connection.MultiClusterClient
	Domain             string
}

// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsireplicationgroups,verbs=get;list;watch;update;patch;delete;create
// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsireplicationgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch

// Reconcile contains reconciliation logic that updates ReplicationGroup depending on it's current state
func (r *ReplicationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("dellcsireplicationgroup", req.Name)
	ctx = context.WithValue(ctx, common.LoggerContextKey, log)

	localRG := new(repv1.DellCSIReplicationGroup)
	err := r.Get(ctx, req.NamespacedName, localRG)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(common.InfoLevel).Info("Reconciling RG event!!!")
	localRGName := req.Name
	remoteRGName := localRG.Annotations[controller.RemoteReplicationGroup]
	if remoteRGName == "" {
		remoteRGName = localRGName
	}
	rgSyncComplete := false

	if localRG.Annotations == nil {
		log.V(common.InfoLevel).Info("RG is not ready yet, requeue as we will get another event")
		return ctrl.Result{}, nil
	} else if localRG.Annotations[controller.RGSyncComplete] == "yes" {
		log.V(common.DebugLevel).Info("RG Sync already completed")
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
		log.V(common.InfoLevel).Info("Deletion timestamp is not zero")
		log.V(common.InfoLevel).WithValues(localRG.Annotations).Info("Annotations")
		_, ok := localRG.Annotations[controller.DeletionRequested]
		log.V(common.InfoLevel).WithValues(ok).Info("Deletion requested?", ok)

		if _, ok := localRG.Annotations[controller.DeletionRequested]; !ok {
			log.V(common.InfoLevel).Info("Deletion requested annotation not found")
			remoteRG, err := remoteClient.GetReplicationGroup(ctx, localRG.Annotations[controller.RemoteReplicationGroup])
			if err != nil {
				log.V(common.ErrorLevel).WithValues(err.Error()).Info("error getting replication group")
				// If remote RG doesn't exist, proceed to removing finalizer
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to get remote replication group")
					return ctrl.Result{}, err
				}
			} else {
				log.V(common.InfoLevel).Info("Got remote RG")
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

		log.V(common.InfoLevel).Info("Removing finalizer RGFinalizer")
		finalizerRemoved := controller.RemoveFinalizerIfExists(localRG, controller.RGFinalizer)
		if finalizerRemoved {
			log.V(common.InfoLevel).Info("Updating rg copy to remove finalizer")
			return ctrl.Result{}, r.Update(ctx, localRG)
		}
	}

	rgCopy := localRG.DeepCopy()

	log.V(common.InfoLevel).Info("Adding finalizer RGFinalizer")
	// Check for the finalizer; add, if doesn't exist
	if finalizerAdded := controller.AddFinalizerIfNotExist(rgCopy, controller.RGFinalizer); finalizerAdded {
		log.V(common.InfoLevel).Info("Finalizer not found adding it")
		return ctrl.Result{}, r.Update(ctx, rgCopy)
	}
	log.V(common.InfoLevel).Info("Trying to delete RG if deletion request annotation found")
	// Check for deletion request annotation
	if _, ok := rgCopy.Annotations[controller.DeletionRequested]; ok {
		log.V(common.InfoLevel).Info("Deletion Requested annotation found and deleting the remote RG")
		return ctrl.Result{}, r.Delete(ctx, rgCopy)
	}

	createRG := false

	// If the RG already exists on the Remote Cluster,
	// We treat this as idempotent.
	log.V(common.InfoLevel).Info(fmt.Sprintf("Checking if remote RG with the name %s exists on ClusterId: %s",
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
			log.V(common.InfoLevel).Info("RG not found on target cluster. " +
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
		log.V(common.InfoLevel).Info(" The RG already exists on the remote cluster")
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
		log.V(common.InfoLevel).Info("The remote RG has been successfully created!!")
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

	err = r.processLastActionResult(ctx, localRG, remoteClient, log)
	if err != nil {
		r.EventRecorder.Eventf(localRG, eventTypeWarning, eventReasonUpdated,
			"failed to process the last action %s", localRG.Status.LastAction.Condition)
	}

	log.V(common.InfoLevel).Info("RG has already been synced to the remote cluster")
	return ctrl.Result{}, nil
}

func (r *ReplicationGroupReconciler) processLastActionResult(ctx context.Context, group *repv1.DellCSIReplicationGroup, remoteClient connection.RemoteClusterClient, log logr.Logger) error {
	if len(group.Status.Conditions) == 0 || group.Status.LastAction.Time == nil {
		log.V(common.InfoLevel).Info("No action to process")
		return nil
	}

	if group.Status.LastAction.ErrorMessage != "" {
		return fmt.Errorf("last action failed: %s", group.Status.LastAction.Condition)
	}

	val, ok := group.Annotations[controller.ActionProcessedTime]
	if !ok {
		log.V(common.InfoLevel).Info("Action Processed does not exist.")
		return nil
	}

	if val == group.Status.LastAction.Time.GoString() {
		log.V(common.InfoLevel).Info("Last action has already been processed")
		return nil
	}

	if strings.Contains(group.Status.LastAction.Condition, "CREATE_SNAPSHOT") {
		if err := getDellCsiReplicationGroupProcessSnapshotEvent(r, ctx, group, remoteClient, log); err != nil {
			return err
		}
	}

	// Informing the RG that the last action has been processed.
	controller.AddAnnotation(group, controller.ActionProcessedTime, group.Status.LastAction.Time.GoString())

	return getDellCsiReplicationGroupUpdate(r, ctx, group)
}

func (r *ReplicationGroupReconciler) processSnapshotEvent(ctx context.Context, group *repv1.DellCSIReplicationGroup, remoteClient connection.RemoteClusterClient, log logr.Logger) error {
	lastAction := group.Status.LastAction

	val, ok := group.Annotations[csireplicator.Action]
	if !ok {
		log.V(common.InfoLevel).Info("No action", "val", val)
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
		log.V(common.InfoLevel).Info("Namespace - " + actionAnnotation.SnapshotNamespace + " not found, creating it.")
		nsRef := makeNamespaceReference(actionAnnotation.SnapshotNamespace)

		err = getDellCsiReplicationGroupCreateNamespace(remoteClient, ctx, nsRef)
		if err != nil {
			msg := "unable to create the desired namespace" + actionAnnotation.SnapshotNamespace
			log.V(common.ErrorLevel).Error(err, msg)
			return err
		}
	}

	for volumeHandle, snapshotHandle := range lastAction.ActionAttributes {
		msg := "ActionAttributes - volumeHandle: " + volumeHandle + ", snapshotHandle: " + snapshotHandle
		log.V(common.InfoLevel).Info(msg)

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
