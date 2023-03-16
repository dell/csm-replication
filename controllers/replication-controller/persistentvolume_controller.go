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

package replicationcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/dell-csi-extensions/replication"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/dell/csm-replication/pkg/connection"
	reconcile "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	replicated = "replicated"
)

// PersistentVolumeReconciler reconciles a PersistentVolume object
type PersistentVolumeReconciler struct {
	client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme
	EventRecorder      record.EventRecorder
	PVCRequeueInterval time.Duration
	Config             connection.MultiClusterClient
	Domain             string
}

// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;update;patch;list;watch;delete

// Reconcile contains reconciliation logic that updates PersistentVolume depending on it's current state
func (r *PersistentVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("persistentvolume", req.Name)
	ctx = context.WithValue(ctx, common.LoggerContextKey, log)

	r.Log.V(common.InfoLevel).Info("Reconciling PV event")

	volume := new(v1.PersistentVolume)
	if err := r.Get(ctx, req.NamespacedName, volume); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// First get the required fields from PVC
	localPVName := volume.Name
	// Lets start with the assumption that remote PV name is same as local PV
	remotePVName := localPVName

	log.V(common.DebugLevel).Info("Fetching details from the PV object")

	// Parse the local annotations
	localAnnotations := volume.Annotations

	// RemoteClusterID annotation
	remoteClusterID, err := getValueFromAnnotations(controller.RemoteClusterID, localAnnotations)
	if err != nil {
		log.Error(err, "remoteClusterID not set")
		r.EventRecorder.Eventf(volume, eventTypeWarning, eventReasonUpdated,
			"failed to fetch remote cluster id from annotations. error: %s", err.Error())
		return ctrl.Result{}, err
	}

	// For single cluster, the remote clusterID is 'self'
	// For this special case, we prefix the PV with 'replicated' keyword
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

	// Check for volume retention policy annotation
	retentionPolicy, ok := volume.Annotations[controller.RemotePVRetentionPolicy]
	if !ok {
		log.V(common.InfoLevel).Info("Retention policy not set, using retain as the default policy")
		retentionPolicy = "retain" // we will default to retain the PV if there is no retention policy is set
	}

	// Handle PV deletion if timestamp is set
	if !volume.DeletionTimestamp.IsZero() {
		// Process deletion of remote PV
		if _, ok := volume.Annotations[controller.DeletionRequested]; !ok {
			log.V(common.InfoLevel).Info("Deletion requested annotation not found")
			remoteVolume, err := rClient.GetPersistentVolume(ctx, volume.Annotations[controller.RemotePV])
			if err != nil {
				// If remote PV doesn't exist, proceed to removing finalizer
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to get remote volume")
					return ctrl.Result{}, err
				}
			} else {
				log.V(common.DebugLevel).Info("Got remote PV")
				if retentionPolicy == "delete" {
					log.V(common.InfoLevel).Info("Retention policy is set to Delete")
					if _, ok := remoteVolume.Annotations[controller.DeletionRequested]; !ok {
						// Add annotation on the remote PV to request its deletion
						remoteVolumeCopy := remoteVolume.DeepCopy()
						log.V(common.InfoLevel).Info("Adding deletion requested annotation to remote volume")
						controller.AddAnnotation(remoteVolumeCopy, controller.DeletionRequested, "yes")

						// also apply new annotation - SynchronizedDeletionStatus
						log.V(common.InfoLevel).Info("Adding sync delete annotation to remote")
						controller.AddAnnotation(remoteVolumeCopy, controller.SynchronizedDeletionStatus, "requested")
						err := rClient.UpdatePersistentVolume(ctx, remoteVolumeCopy)
						if err != nil {
							log.V(common.InfoLevel).Info("Error encountered in updating remote volume")
							return ctrl.Result{}, err
						}

						// Resetting the rate-limiter to requeue for the deletion of remote PV
						return ctrl.Result{RequeueAfter: 1 * time.Millisecond}, nil
					}
					// Requeueing because the remote PV still exists
					return ctrl.Result{Requeue: true}, nil
				}
			}
		}

		log.V(common.InfoLevel).Info("Removing finalizer on local volume")
		finalizerRemoved := controller.RemoveFinalizerIfExists(volume, controller.ReplicationFinalizer)
		if finalizerRemoved {
			return ctrl.Result{}, r.Update(ctx, volume)
		}
	}

	volumeCopy := volume.DeepCopy()

	// Check for the finalizer; add, if doesn't exist
	if finalizerAdded := controller.AddFinalizerIfNotExist(volumeCopy, controller.ReplicationFinalizer); finalizerAdded {
		log.V(common.DebugLevel).Info("Finalizer not found, adding it")
		return ctrl.Result{}, r.Update(ctx, volumeCopy)
	}
	// Check for deletion request annotation
	if _, ok := volumeCopy.Annotations[controller.DeletionRequested]; ok {
		log.V(common.InfoLevel).Info("Deletion Requested by remote's controller, annotation found")

		// Check for the sync deletion annotation. If it's 'complete' or nonexistent, delete the PV.
		// If it's 'requested', wait.
		syncDeleteStatus, ok := volume.Annotations[controller.SynchronizedDeletionStatus]
		if !ok || syncDeleteStatus == "complete" {
			log.V(common.InfoLevel).Info("Synchronized Deletion annotation either complete or not set, deleting PV")
			return ctrl.Result{}, r.Delete(ctx, volumeCopy)
		}

		log.V(common.InfoLevel).Info("Synchronized Deletion annotation exists and is not complete, cannot delete PV")
	}

	_, ok = volume.Annotations[controller.PVProtectionComplete]
	if ok {
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

		// RemoteVolumeAnnotation
		var remoteVolumeDetails replication.Volume
		var volumeHandle string
		if _, ok := volume.Annotations[controller.CreatedBy]; !ok {
			err = json.Unmarshal([]byte(volume.Annotations[controller.RemoteVolumeAnnotation]), &remoteVolumeDetails)
			if err != nil {
				log.Error(err, "Failed to unmarshal json for remote volume details")
				return ctrl.Result{}, err
			}

			volumeHandle = remoteVolumeDetails.VolumeId
			if volumeHandle == "" {
				volHandleErr := fmt.Errorf("volume_id missing from the remote volume annotation")
				log.Error(volHandleErr, "unexpected error")
				r.EventRecorder.Eventf(volume, eventTypeWarning, eventReasonUpdated, "%s", volHandleErr.Error())
				return ctrl.Result{}, volHandleErr
			}
		}

		// RemoteStorageClass Annotation
		remoteSCName, err := getValueFromAnnotations(controller.RemoteStorageClassAnnotation, localAnnotations)
		if err != nil {
			log.Error(err, "failed to fetch remote storage class name")
			r.EventRecorder.Eventf(volume, eventTypeWarning, eventReasonUpdated,
				"failed to fetch remote storage class name from annotations. error: %s", err.Error())
			return ctrl.Result{}, err
		}

		// ReplicationGroup Annotation
		localRGName, err := getValueFromAnnotations(controller.ReplicationGroup, localAnnotations)
		if err != nil {
			log.Error(err, "failed to fetch local replication group name")
			r.EventRecorder.Eventf(volume, eventTypeWarning, eventReasonUpdated,
				"failed to fetch local replication group name from annotations. error: %s", err.Error())
			return ctrl.Result{}, err
		}

		// Check if remote SC exists
		remoteSC, err := rClient.GetStorageClass(ctx, remoteSCName)
		if err != nil && errors.IsNotFound(err) {
			// Log an event and throw the error
			log.Error(err, "remote storage class doesn't exist")
			r.EventRecorder.Eventf(volume, eventTypeWarning, eventReasonUpdated,
				"remote storage class: %s doesn't exist on cluster: %s. error: %s",
				remoteSCName, remoteClusterID, err.Error())
			return ctrl.Result{}, err
		} else if err != nil {
			// This could be transient. So, throw an error
			log.Error(err, "failed to fetch remote storage class")
			return ctrl.Result{}, err
		}

		var localClusterID string
		if remoteClusterID == controller.Self {
			localClusterID = controller.Self
		} else {
			localClusterID = r.Config.GetClusterID()
		}

		pv := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:       remotePVName,
				Finalizers: []string{controller.ReplicationFinalizer},
			},
			Spec: v1.PersistentVolumeSpec{
				PersistentVolumeSource: v1.PersistentVolumeSource{CSI: &v1.CSIPersistentVolumeSource{
					Driver:           remoteSC.Provisioner,
					VolumeHandle:     volumeHandle,
					VolumeAttributes: remoteVolumeDetails.GetVolumeContext(),
				}},
				Capacity:                      volume.Spec.Capacity,
				AccessModes:                   volume.Spec.AccessModes,
				VolumeMode:                    volume.Spec.VolumeMode,
				StorageClassName:              remoteSCName,
				PersistentVolumeReclaimPolicy: *remoteSC.ReclaimPolicy,
			},
		}

		// Update Labels for remote PV
		updatePVLabels(pv, volume, localClusterID)

		// Add driver specific labels
		contextPrefix := volume.Annotations[controller.ContextPrefix]
		if contextPrefix != "" {
			for k, v := range remoteVolumeDetails.GetVolumeContext() {
				if strings.HasPrefix(k, contextPrefix) {
					labelKey := fmt.Sprintf("%s%s", r.Domain, strings.TrimPrefix(k, contextPrefix))
					controller.AddLabel(pv, labelKey, fmt.Sprintf("%v", v))
				}
			}
		}

		var resourceRequests []byte

		if volume.Spec.ClaimRef != nil {
			pvc := new(v1.PersistentVolumeClaim)
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: volume.Spec.ClaimRef.Namespace,
				Name:      volume.Spec.ClaimRef.Name,
			}, pvc); err != nil {
				return ctrl.Result{}, err
			}

			resourceRequests, err = json.Marshal(pvc.Spec.Resources.Requests)
			if err != nil {
				log.Error(err, "Unable to marshal PVC Resource requests")
				return ctrl.Result{}, err
			}

		}

		updatePVAnnotations(pv, volume, localClusterID, localAnnotations[controller.RemotePVRetentionPolicy], string(resourceRequests))

		// Query local RG object to verify if RemoteReplicationGroup annotation is available
		localRG := new(repv1.DellCSIReplicationGroup)
		err = r.Get(ctx, types.NamespacedName{Name: localRGName}, localRG)
		if err != nil {
			// Requeue
			return ctrl.Result{}, err
		}

		remoteRGName, err := getValueFromAnnotations(controller.RemoteReplicationGroup, localRG.Annotations)
		if err != nil {
			// We can still continue as we can always requeue the request
			log.V(common.DebugLevel).Info("Remote RG annotation has not been set on the local RG")
		} else {
			// Update the annotation for the remote PV object
			// TODO: Also add local volume information to remote PV
			controller.AddAnnotation(pv, controller.ReplicationGroup, remoteRGName)
			controller.AddLabel(pv, controller.ReplicationGroup, remoteRGName)
		}

		log.V(common.DebugLevel).Info("Checking if the PV already exists " + remotePVName)
		remotePV, err := rClient.GetPersistentVolume(ctx, remotePVName)
		createRemotePV := false
		if err != nil && errors.IsNotFound(err) {
			if remotePVAnnotationSet {
				// This is unexpected as the RemotePV annotation indicates that the remote PV should be created
				log.Error(err, "Something went wrong. Remote PV annotation already set")
				log.V(common.InfoLevel).Info("Creating the remote PV again")
			}
			createRemotePV = true
		} else if err != nil {
			log.Error(err, "failed to check if remote PV exists")
			return ctrl.Result{}, err
		}

		if createRemotePV {
			// We need to create the PV
			err = rClient.CreatePersistentVolume(ctx, pv)
			if err != nil {
				log.Error(err, "Failed to create remote PV on target cluster")
				return ctrl.Result{}, err
			}
			log.V(common.InfoLevel).Info(fmt.Sprintf("Successfully created the remote PV with name: %s on cluster: %s",
				remotePVName, remoteClusterID))
			r.EventRecorder.Eventf(volume, eventTypeNormal, eventReasonUpdated,
				"Created Remote PV with name: %s on cluster: %s", remotePVName, remoteClusterID)
			// fetch the newly created PV object
			remotePV, err = rClient.GetPersistentVolume(ctx, remotePVName)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else {
			// Remote PV already exists
			// TODO: Verify if the Remote PV matches with the one we are expecting

			// For now just verify if the cluster id matches
			if remotePV.Annotations[controller.RemoteClusterID] != localClusterID {
				// This PV was created by someone else
				// For now, lets just raise an event and stop the reconcile
				log.Error(fmt.Errorf("conflicting PV with name: %s exists on ClusterId: %s",
					remotePVName, remoteClusterID), "stopping reconcile")
				r.EventRecorder.Eventf(volume, eventTypeWarning, eventReasonUpdated,
					"Found conflicting PV %s on remote ClusterId: %s", remotePVName, remoteClusterID)
				return ctrl.Result{}, nil
			}
		}

		// Get the local PV object
		localPV := v1.PersistentVolume{}
		err = r.Get(ctx, types.NamespacedName{Name: localPVName}, &localPV)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Now update the remote PV with RemoteReplicationGroup annotation if required
		requeue, err := r.processRemotePV(ctx, rClient, remotePV, remoteRGName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if requeue {
			return ctrl.Result{RequeueAfter: controller.DefaultRetryInterval}, nil
		}

		err = r.processLocalPV(ctx, localPV.DeepCopy(), remotePVName, remoteClusterID)
		if err != nil {
			return ctrl.Result{}, err
		}

		log.V(common.InfoLevel).Info("PV Reconcile complete!!!!!")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *PersistentVolumeReconciler) processRemotePV(ctx context.Context,
	rClient connection.RemoteClusterClient, remotePV *v1.PersistentVolume, remoteRGName string) (bool, error) {
	log := common.GetLoggerFromContext(ctx)
	if remoteRGName != "" {
		// Update the annotation for the remote PV object
		//controller.AddAnnotation(remotePV, controller.ReplicationGroup, remoteRGName)
		if remotePV.Annotations[controller.ReplicationGroup] == "" {
			// Set the annotation
			log.V(common.DebugLevel).Info(fmt.Sprintf("Setting the ReplicationGroup label & annotation %s on remotePV %s", remoteRGName, remotePV.Name))
			controller.AddAnnotation(remotePV, controller.ReplicationGroup, remoteRGName)
			controller.AddLabel(remotePV, controller.ReplicationGroup, remoteRGName)
			err := rClient.UpdatePersistentVolume(ctx, remotePV)
			if err != nil {
				return true, err
			}
		} else {
			log.V(common.DebugLevel).Info("ReplicationGroup Annotation already set")
		}
		return false, nil
	}
	return true, nil
}

func (r *PersistentVolumeReconciler) processLocalPV(ctx context.Context, localPV *v1.PersistentVolume, remotePVName, remoteClusterID string) error {
	log := common.GetLoggerFromContext(ctx)
	// Update the local PV with the remote details
	// First check if the PV sync has been completed
	if localPV.Annotations[controller.PVSyncComplete] == "yes" {
		log.V(common.InfoLevel).Info("PV has already been synced")
		return nil
	}
	updatePV := false
	remotePVNameFromLocalPVAnnotation := localPV.Annotations[controller.RemotePV]
	remoteClusterIDFromLocalPVAnnotation := localPV.Annotations[controller.RemoteClusterID]

	if remotePVNameFromLocalPVAnnotation == "" {
		controller.AddAnnotation(localPV, controller.RemotePV, remotePVName)
		updatePV = true
	} else if remotePVNameFromLocalPVAnnotation != remotePVName {
		// Conflict - ??
	} else {
		log.V(common.InfoLevel).Info(fmt.Sprintf("%s already set to %s for local PV: %s",
			controller.RemotePV, remotePVNameFromLocalPVAnnotation, localPV.Name))
	}

	if remoteClusterIDFromLocalPVAnnotation == "" {
		controller.AddAnnotation(localPV, controller.RemoteClusterID, remoteClusterID)
		updatePV = true
	} else if remoteClusterIDFromLocalPVAnnotation != remoteClusterID {
		// Conflict - ??
	} else {
		log.V(common.InfoLevel).Info(fmt.Sprintf("%s already set to %s for local PV: %s",
			controller.RemoteClusterID, remoteClusterIDFromLocalPVAnnotation, localPV.Name))
	}

	if updatePV {
		// Finally add the PV sync complete annotation
		controller.AddAnnotation(localPV, controller.PVSyncComplete, "yes")
		err := r.Update(ctx, localPV)
		if err != nil {
			return err
		}
		log.V(common.InfoLevel).Info("Successfully updated local PV with remote annotations")
		r.EventRecorder.Eventf(localPV, eventTypeNormal, eventReasonUpdated,
			"PV sync complete for ClusterId: %s", remoteClusterID)
	}
	return nil
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *PersistentVolumeReconciler) SetupWithManager(mgr ctrl.Manager, limiter ratelimiter.RateLimiter, maxReconcilers int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.PersistentVolume{}, builder.WithPredicates(
			predicate.Or(
				pvProtectionIsComplete(),
				hasDeletionTimestamp(),
				isDeletionRequested(),
			),
		)).WithOptions(reconcile.Options{
		RateLimiter:             limiter,
		MaxConcurrentReconciles: maxReconcilers,
	}).
		Complete(r)
}

func pvProtectionIsComplete() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(meta client.Object) bool {
		a := meta.GetAnnotations()
		return a != nil && a[controller.PVProtectionComplete] == "yes"
	})
}

func hasDeletionTimestamp() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(meta client.Object) bool {
		return meta.GetDeletionTimestamp() != nil
	})
}

func isDeletionRequested() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(meta client.Object) bool {
		a := meta.GetAnnotations()
		return a != nil && a[controller.DeletionRequested] == "yes"
	})
}

func getValueFromAnnotations(key string, annotations map[string]string) (string, error) {
	if val, ok := annotations[key]; ok {
		if val == "" {
			return "", fmt.Errorf("missing value for %s", key)
		}
		return val, nil
	}
	return "", fmt.Errorf("not set")
}

func updatePVAnnotations(pv, volume *v1.PersistentVolume, remoteClusterID, remotePVRetentionPolicy, resourceRequest string) {
	if volume.Spec.CSI != nil {
		controller.AddAnnotation(pv, "pv.kubernetes.io/provisioned-by", volume.Spec.CSI.Driver)
	}
	// Created By
	controller.AddAnnotation(pv, controller.CreatedBy, common.DellReplicationController)
	// Remote PV Name
	controller.AddAnnotation(pv, controller.RemotePV, volume.Name)

	if volume.Spec.ClaimRef != nil {
		// Remote PVC Name
		controller.AddAnnotation(pv, controller.RemotePVC, volume.Spec.ClaimRef.Name)
		// Remote PVC Namespace
		controller.AddAnnotation(pv, controller.RemotePVCNamespace, volume.Spec.ClaimRef.Namespace)
	}
	// Remote ClusterID
	controller.AddAnnotation(pv, controller.RemoteClusterID, remoteClusterID)
	// Remote Storage Class
	controller.AddAnnotation(pv, controller.RemoteStorageClassAnnotation, volume.Spec.StorageClassName)
	// Resource Requests
	if resourceRequest != "" {
		controller.AddAnnotation(pv, controller.ResourceRequest, resourceRequest)
	}
	// RemotePVRetentionPolicy
	controller.AddAnnotation(pv, controller.RemotePVRetentionPolicy, remotePVRetentionPolicy)
}

func updatePVLabels(pv, volume *v1.PersistentVolume, remoteClusterID string) {
	// Driver Name
	controller.AddLabel(pv, controller.DriverName, volume.Labels[controller.DriverName])
	// Remote ClusterID
	controller.AddLabel(pv, controller.RemoteClusterID, remoteClusterID)
	// Remote PVC Namespace
	if volume.Spec.ClaimRef != nil {
		controller.AddLabel(pv, controller.RemotePVCNamespace, volume.Spec.ClaimRef.Namespace)
	}
}
