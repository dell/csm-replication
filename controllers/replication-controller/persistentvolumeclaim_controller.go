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
	"fmt"
	"time"

	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/dell/csm-replication/pkg/connection"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PersistentVolumeClaimReconciler reconciles a PersistentVolumeClaim object
type PersistentVolumeClaimReconciler struct {
	client.Client
	Log                      logr.Logger
	Scheme                   *runtime.Scheme
	EventRecorder            record.EventRecorder
	PVCRequeueInterval       time.Duration
	Config                   connection.MultiClusterClient
	Domain                   string
	AllowPVCCreationOnTarget bool
}

var (
	getPersistentVolumeClaimUpdatePersistentVolumeClaim = func(r connection.RemoteClusterClient, ctx context.Context, claim *v1.PersistentVolumeClaim) error {
		return r.UpdatePersistentVolumeClaim(ctx, claim)
	}
	getPersistentVolumeClaimUpdate = func(r *PersistentVolumeClaimReconciler, ctx context.Context, claim *v1.PersistentVolumeClaim) error {
		return r.Update(ctx, claim)
	}
	getPersistentVolumeClaimReconcilerEventf = func(r *PersistentVolumeClaimReconciler, claim *v1.PersistentVolumeClaim, eventTypeNormal string, eventReasonUpdated string, message string, remoteClusterID string) {
		r.EventRecorder.Eventf(claim, eventTypeNormal, eventReasonUpdated,
			message, remoteClusterID)
	}
)

// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;update;patch;list;watch;create
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

// Reconcile contains reconciliation logic that updates PersistentVolumeClaim depending on it's current state
func (r *PersistentVolumeClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// If we have received the reconcile request, it means that the sidecar has completed its protection
	log := r.Log.WithValues("persistentvolumeclaim", req.NamespacedName)
	ctx = context.WithValue(ctx, common.LoggerContextKey, log)

	claim := new(v1.PersistentVolumeClaim)
	err := r.Get(ctx, req.NamespacedName, claim)
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// First get the required fields from PVC
	localPVName := claim.Spec.VolumeName
	// Lets start with the assumption that remote PV name is same as local PV
	remotePVName := localPVName

	log.V(common.InfoLevel).Info("Reconciling PVC event!!!")

	// Parse the local annotations
	localAnnotations := claim.Annotations

	// RemoteClusterID annotation
	remoteClusterID, err := getValueFromAnnotations(controller.RemoteClusterID, localAnnotations)
	if err != nil {
		log.Error(err, "remoteClusterID not set")
		r.EventRecorder.Eventf(claim, eventTypeWarning, eventReasonUpdated,
			"failed to fetch remote cluster id from annotations. error: %s", err.Error())
		return ctrl.Result{}, err
	}

	// For single cluster, the remote clusterID is 'self'
	// For this special case, we prefix the PV with 'self' keyword
	// in order to allow a replicated PV to exist on the source
	// cluster itself
	if remoteClusterID == controller.Self {
		remotePVName = replicated + "-" + localPVName
	}

	// Get the remote client
	rClient, err := r.Config.GetConnection(remoteClusterID)
	if err != nil {
		return ctrl.Result{}, err
	}

	// RemotePV Annotation
	remotePVAnnotationSet := false
	if localAnnotations[controller.RemotePV] != "" {
		// We have already reconciled this in the past
		// Lets verify again if everything is proper
		remotePVAnnotationSet = true
		// Update the remote PV name to point to the one in annotation
		remotePVName = localAnnotations[controller.RemotePV]
		log.V(common.DebugLevel).Info("Remote PV annotation already set. Verifying details")
	}

	log.V(common.DebugLevel).Info("Checking if the PV already exists " + remotePVName)
	remotePV, err := rClient.GetPersistentVolume(ctx, remotePVName)
	if err != nil && errors.IsNotFound(err) {
		if remotePVAnnotationSet {
			// This is unexpected as the RemotePV annotation indicates that the remote PV should be created
			log.Error(err, "Something went wrong. Remote PV annotation already set")
			return ctrl.Result{}, err
		}
		log.V(common.InfoLevel).Info("Will wait for remote PV to be created...")
		return ctrl.Result{RequeueAfter: controller.DefaultRetryInterval}, nil
	} else if err != nil {
		log.Error(err, "failed to check if remote PV exists")
		return ctrl.Result{}, err
	}

	// Get the remote PVC object
	remoteClaim := &v1.PersistentVolumeClaim{}
	remotePVCName := ""
	remotePVCNamespace := ""
	fmt.Printf("ssss allow pvc creation falg is %v \n", r.AllowPVCCreationOnTarget)
	if remoteClusterID != controller.Self && r.AllowPVCCreationOnTarget {
		//if its not single cluster then check the pv status and create pvc on target cluster
		if remotePV.Status.Phase == v1.VolumeAvailable && remotePV.Spec.ClaimRef != nil {
			fmt.Printf("ssss inside if check of volume available \n")
			remoteClaim.Spec.AccessModes = remotePV.Spec.AccessModes
			remoteClaim.Spec.Resources.Requests = v1.ResourceList{
				v1.ResourceStorage: remotePV.Spec.Capacity[v1.ResourceStorage],
			}
			requests := remoteClaim.Spec.Resources.Requests
			requestsStr := fmt.Sprintf("%v", requests)
			fmt.Printf("ssss updating annotations for pvc \n")
			//updating pvc annotations
			updatePVCAnnotations(remoteClaim, remoteClusterID, requestsStr, remotePV)
			fmt.Printf("ssss updating labels for pvc \n")
			//updating pvc labels
			updatePVCLabels(remoteClaim, claim, remoteClusterID)
			err = rClient.CreatePersistentVolumeClaim(ctx, remoteClaim)
			if err != nil {
				log.Error(err, "Failed to create remote PVC on target cluster")
				return ctrl.Result{}, err
			}
			fmt.Printf("ssss PVC created without error %+v\n", remoteClaim)
		}
	}
	if remotePV.Status.Phase == v1.VolumeBound && remotePV.Spec.ClaimRef != nil {
		fmt.Printf("ssss if volume is bound and claim ref not nil %+v\n", remotePV.Spec.ClaimRef)
		remotePVCName = remotePV.Spec.ClaimRef.Name
		remotePVCNamespace = remotePV.Spec.ClaimRef.Namespace
		remoteClaim, err = rClient.GetPersistentVolumeClaim(ctx, remotePVCNamespace, remotePVCName)
		if err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	} else {
		remoteClaim = nil
	}

	//Shefali
	// Handle PVC deletion if timestamp is set in remote PV
	if remotePV.DeletionTimestamp.IsZero() {
		fmt.Printf("ssss If deletion time stamp added to remote PVC namespace %s pvc name  %s \n", remotePVCNamespace, remotePV.Spec.ClaimRef.Name)
		// Process deletion of remote PV
		if _, ok := remoteClaim.Annotations[controller.DeletionRequested]; !ok {
			log.V(common.InfoLevel).Info("Deletion requested annotation not found")
			remoteVolumeClaim, err := rClient.GetPersistentVolumeClaim(ctx, remotePVCNamespace, remotePV.Spec.ClaimRef.Name)
			fmt.Printf("ssss after get pvc \n")
			if err != nil {
				// If remote PVC doesn't exist, proceed to removing finalizer
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to get remote volume claim")
					return ctrl.Result{}, err
				}
			} else {
				log.V(common.DebugLevel).Info("Got remote PVC")
				if remotePV.Annotations[controller.RemoteRGRetentionPolicy] == "delete" {
					fmt.Printf("ssss RG retention policy is set to delete \n")
					log.V(common.InfoLevel).Info("Retention policy is set to Delete")
					if _, ok := remoteVolumeClaim.Annotations[controller.DeletionRequested]; !ok {
						// Add annotation on the remote PV to request its deletion
						remoteVolumeClaimCopy := remoteVolumeClaim.DeepCopy()
						log.V(common.InfoLevel).Info("Adding deletion requested annotation to remote volume claim")
						controller.AddAnnotation(remoteVolumeClaimCopy, controller.DeletionRequested, "yes")

						// also apply new annotation - SynchronizedDeletionStatus
						log.V(common.InfoLevel).Info("Adding sync delete annotation to remote pvc")
						controller.AddAnnotation(remoteVolumeClaimCopy, controller.SynchronizedDeletionStatus, "requested")
						err := rClient.UpdatePersistentVolumeClaim(ctx, remoteVolumeClaimCopy)
						if err != nil {
							log.V(common.InfoLevel).Info("Error encountered in updating remote volume claim")
							return ctrl.Result{}, err
						}

						// Resetting the rate-limiter to requeue for the deletion of remote PV
						return ctrl.Result{RequeueAfter: 1 * time.Millisecond}, nil
					}
					// Requeueing local PV because the remote PV still exists.
					return ctrl.Result{Requeue: true}, nil
				}
			}
		}

		// Check for the sync deletion annotation. If it exists and is not complete, requeue.
		// Otherwise, remove finalizer.
		syncDeleteStatus, ok := remotePV.Annotations[controller.SynchronizedDeletionStatus]
		if ok && syncDeleteStatus != "complete" {
			fmt.Printf("ssss deletion annotation is not syncronized \n")
			log.V(common.InfoLevel).Info("Synchronized Deletion annotation exists and is not complete, requeueing.")
			return ctrl.Result{Requeue: true}, nil
		}

		log.V(common.InfoLevel).Info("Removing finalizer on remote volume claim")
		finalizerRemoved := controller.RemoveFinalizerIfExists(remoteClaim, controller.ReplicationFinalizer)
		fmt.Printf("ssss After removing finalizer \n")
		if finalizerRemoved {
			return ctrl.Result{}, r.Update(ctx, remoteClaim)
		}
	}

	remoteClaimCopy := remoteClaim.DeepCopy()

	// Check for the finalizer; add, if doesn't exist
	if finalizerAdded := controller.AddFinalizerIfNotExist(remoteClaimCopy, controller.ReplicationFinalizer); finalizerAdded {
		log.V(common.DebugLevel).Info("Finalizer not found, adding it")
		return ctrl.Result{}, r.Update(ctx, remoteClaimCopy)
	}
	// Check for deletion request annotation.
	if _, ok := remoteClaimCopy.Annotations[controller.DeletionRequested]; ok {
		log.V(common.InfoLevel).Info("Deletion Requested by remote's controller, annotation found")
		return ctrl.Result{}, r.Delete(ctx, remoteClaimCopy)
	}

	isRemotePVCUpdated := false
	if remoteClaim != nil {
		// Update the remote PVC if it exists
		isRemotePVCUpdated, err = r.processRemotePVC(ctx, rClient, remoteClaim, req.Name, req.Namespace, localPVName)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		log.V(common.InfoLevel).Info("Remote PVC has not been created yet. Information can't be synced")
	}

	err = r.processLocalPVC(ctx, claim, remotePVName, remotePVCName, remotePVCNamespace, remoteClusterID, isRemotePVCUpdated)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(common.InfoLevel).Info("PVC Reconcile complete!!!!")
	return ctrl.Result{}, nil
}

func (r *PersistentVolumeClaimReconciler) processRemotePVC(ctx context.Context,
	rClient connection.RemoteClusterClient,
	claim *v1.PersistentVolumeClaim,
	remotePVCName, remotePVCNamespace, remotePVName string,
) (bool, error) {
	log := common.GetLoggerFromContext(ctx)
	isUpdated := false
	// Just apply the missing annotation
	if claim.Annotations[controller.RemotePVC] == "" {
		claim.Annotations[controller.RemotePVC] = remotePVCName
		isUpdated = true
	}
	if claim.Annotations[controller.RemotePVCNamespace] == "" {
		claim.Annotations[controller.RemotePVCNamespace] = remotePVCNamespace
		isUpdated = true
	}
	if claim.Annotations[controller.RemotePV] == "" {
		claim.Annotations[controller.RemotePV] = remotePVName
		isUpdated = true
	}
	if isUpdated {
		err := getPersistentVolumeClaimUpdatePersistentVolumeClaim(rClient, ctx, claim)
		if err != nil {
			return false, err
		}
		log.V(common.InfoLevel).Info("Successfully updated remote PVC with annotation")
		return true, nil
	}

	log.V(common.InfoLevel).Info("Remote PVC already has the annotations set")
	return true, nil
}

func (r *PersistentVolumeClaimReconciler) processLocalPVC(ctx context.Context,
	claim *v1.PersistentVolumeClaim, remotePVName, remotePVCName, remotePVCNamespace,
	remoteClusterID string, isRemotePVCUpdated bool,
) error {
	log := common.GetLoggerFromContext(ctx)
	if claim.Annotations[controller.PVCSyncComplete] == "yes" {
		log.V(common.InfoLevel).Info("PVC Sync already completed")
		return nil
	}
	// Apply the remote PV annotation if required
	if claim.Annotations[controller.RemotePV] == "" {
		controller.AddAnnotation(claim, controller.RemotePV, remotePVName)
	}
	// Apply the remotePVC name and remotePVCNamespace annotation
	if claim.Annotations[controller.RemotePVC] == "" && remotePVCName != "" {
		controller.AddAnnotation(claim, controller.RemotePVC, remotePVCName)
	}
	if claim.Annotations[controller.RemotePVCNamespace] == "" && remotePVCNamespace != "" {
		controller.AddAnnotation(claim, controller.RemotePVCNamespace, remotePVCNamespace)
	}

	if isRemotePVCUpdated {
		controller.AddAnnotation(claim, controller.PVCSyncComplete, "yes")
	}
	// err := r.Update(ctx, claim)
	err := getPersistentVolumeClaimUpdate(r, ctx, claim)
	if err != nil {
		return err
	}

	if isRemotePVCUpdated {
		// r.EventRecorder.Eventf(claim, eventTypeNormal, eventReasonUpdated,
		// 	"PVC sync complete for ClusterId: %s", remoteClusterID)
		getPersistentVolumeClaimReconcilerEventf(r, claim, eventTypeNormal, eventReasonUpdated,
			"PV sync complete for ClusterId: %s", remoteClusterID)
	}
	return nil
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *PersistentVolumeClaimReconciler) SetupWithManager(mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.PersistentVolumeClaim{}, builder.WithPredicates(
			predicate.And(
				pvcProtectionIsComplete(),
			),
		)).WithOptions(reconciler.Options{
		MaxConcurrentReconciles: maxReconcilers,
		RateLimiter:             limiter,
	}).Complete(r)
}

func pvcProtectionIsComplete() predicate.Predicate {
	return newPredicateFuncs(func(meta client.Object) bool {
		a := getAnnotations(meta)
		return a != nil && a[controller.PVCProtectionComplete] == "yes"
	})
}

func updatePVCAnnotations(pvc *v1.PersistentVolumeClaim, remoteClusterID, resourceRequest string, pv *v1.PersistentVolume) {

	//Context prefix
	contextPrefix, _ := getValueFromAnnotations(controller.ContextPrefix, pv.Annotations)
	controller.AddAnnotation(pvc, controller.ContextPrefix, contextPrefix)
	//pvc protection complete
	pvcProtectionComplete, _ := getValueFromAnnotations(controller.PVCProtectionComplete, pv.Annotations)
	controller.AddAnnotation(pvc, controller.PVCProtectionComplete, pvcProtectionComplete)
	// Created By
	controller.AddAnnotation(pvc, controller.CreatedBy, common.DellReplicationController)
	// Remote PV Name
	remoteVolume, _ := getValueFromAnnotations(controller.RemoteVolumeAnnotation, pv.Annotations)
	pvc.Spec.VolumeName = remoteVolume
	controller.AddAnnotation(pvc, controller.RemotePV, remoteVolume)
	// Remote ClusterID
	controller.AddAnnotation(pvc, controller.RemoteClusterID, remoteClusterID)
	//Replication group
	controller.AddAnnotation(pvc, controller.ReplicationGroup, pv.Labels[controller.ReplicationGroup])
	// Remote Storage Class
	pvc.Spec.StorageClassName = &pv.Spec.StorageClassName
	controller.AddAnnotation(pvc, controller.RemoteStorageClassAnnotation, pv.Spec.StorageClassName)
	//Remote Volume
	remoteVolume, _ = getValueFromAnnotations(controller.RemoteVolumeAnnotation, pv.Annotations)
	controller.AddAnnotation(pvc, controller.RemoteVolumeAnnotation, remoteVolume)
	//remote PVC namespace
	remotePVCNamespace, _ := getValueFromAnnotations(controller.RemotePVCNamespace, pv.Annotations)
	pvc.Namespace = remotePVCNamespace
	controller.AddAnnotation(pvc, controller.RemotePVCNamespace, remotePVCNamespace)
	//remote PVC name
	remotePVCName, _ := getValueFromAnnotations(controller.RemotePVC, pv.Annotations)
	pvc.Name = remotePVCName
	controller.AddAnnotation(pvc, controller.RemotePVC, remotePVCName)
}

func updatePVCLabels(pvc, volume *v1.PersistentVolumeClaim, remoteClusterID string) {
	// Driver Name
	controller.AddLabel(pvc, controller.DriverName, volume.Labels[controller.DriverName])
	// Remote ClusterID
	controller.AddLabel(pvc, controller.RemoteClusterID, remoteClusterID)
	// Replication group name
	controller.AddLabel(pvc, controller.ReplicationGroup, volume.Labels[controller.ReplicationGroup])
}
