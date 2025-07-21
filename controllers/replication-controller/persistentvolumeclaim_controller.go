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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common/logger"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/dell/csm-replication/pkg/common/constants"
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
	ctx = context.WithValue(ctx, logger.LoggerContextKey, log)

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

	log.V(logger.InfoLevel).Info("Reconciling PVC event!!!")

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
		log.V(logger.DebugLevel).Info("Remote PV annotation already set. Verifying details")
	}

	log.V(logger.DebugLevel).Info("Checking if the PV already exists " + remotePVName)
	remotePV, err := rClient.GetPersistentVolume(ctx, remotePVName)
	if err != nil && errors.IsNotFound(err) {
		if remotePVAnnotationSet {
			// This is unexpected as the RemotePV annotation indicates that the remote PV should be created
			log.Error(err, "Something went wrong. Remote PV annotation already set")
			return ctrl.Result{}, err
		}
		log.V(logger.InfoLevel).Info("Will wait for remote PV to be created...")
		return ctrl.Result{RequeueAfter: controller.DefaultRetryInterval}, nil
	} else if err != nil {
		log.Error(err, "failed to check if remote PV exists")
		return ctrl.Result{}, err
	}

	// Get the remote PVC object
	remoteClaim := &v1.PersistentVolumeClaim{}
	remotePVCName := ""
	remotePVCNamespace := ""
	if remoteClusterID != controller.Self && r.AllowPVCCreationOnTarget {
		// if its not single cluster then check the pv status and create pvc on target cluster
		if remotePV.Status.Phase == v1.VolumeAvailable && remotePV.Spec.ClaimRef != nil {
			remoteClaim.Spec.AccessModes = remotePV.Spec.AccessModes
			remoteClaim.Spec.Resources.Requests = v1.ResourceList{
				v1.ResourceStorage: remotePV.Spec.Capacity[v1.ResourceStorage],
			}
			// updating pvc annotations and spec
			updatePVCAnnotationsAndSpec(remoteClaim, remoteClusterID, remotePV)
			// updating pvc labels
			updatePVCLabels(remoteClaim, claim, remoteClusterID)
			// Check if the namespace exists
			err := VerifyAndCreateNamespace(ctx, rClient, remoteClaim.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
			// creating PVC on target
			err = rClient.CreatePersistentVolumeClaim(ctx, remoteClaim)
			if err != nil {
				log.Error(err, "Failed to create remote PVC on target cluster")
				return ctrl.Result{}, err
			}
		}
	}
	if remotePV.Status.Phase == v1.VolumeBound && remotePV.Spec.ClaimRef != nil {
		remotePVCName = remotePV.Spec.ClaimRef.Name
		remotePVCNamespace = remotePV.Spec.ClaimRef.Namespace
		remoteClaim, err = rClient.GetPersistentVolumeClaim(ctx, remotePVCNamespace, remotePVCName)
		if err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	} else {
		remoteClaim = nil
	}

	isRemotePVCUpdated := false
	if remoteClaim != nil {
		// Update the remote PVC if it exists
		isRemotePVCUpdated, err = r.processRemotePVC(ctx, rClient, remoteClaim, req.Name, req.Namespace, localPVName)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		log.V(logger.InfoLevel).Info("Remote PVC has not been created yet. Information can't be synced")
	}

	err = r.processLocalPVC(ctx, claim, remotePVName, remotePVCName, remotePVCNamespace, remoteClusterID, isRemotePVCUpdated)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(logger.InfoLevel).Info("PVC Reconcile complete!!!!")
	return ctrl.Result{}, nil
}

// processRemotePVC
func (r *PersistentVolumeClaimReconciler) processRemotePVC(ctx context.Context,
	rClient connection.RemoteClusterClient,
	claim *v1.PersistentVolumeClaim,
	remotePVCName, remotePVCNamespace, remotePVName string,
) (bool, error) {
	log := logger.GetLoggerFromContext(ctx)
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
		log.V(logger.InfoLevel).Info("Successfully updated remote PVC with annotation")
		return true, nil
	}

	log.V(logger.InfoLevel).Info("Remote PVC already has the annotations set")
	return true, nil
}

func (r *PersistentVolumeClaimReconciler) processLocalPVC(ctx context.Context,
	claim *v1.PersistentVolumeClaim, remotePVName, remotePVCName, remotePVCNamespace,
	remoteClusterID string, isRemotePVCUpdated bool,
) error {
	log := logger.GetLoggerFromContext(ctx)
	if claim.Annotations[controller.PVCSyncComplete] == "yes" {
		log.V(logger.InfoLevel).Info("PVC Sync already completed")
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
	return predicate.NewPredicateFuncs(func(meta client.Object) bool {
		a := meta.GetAnnotations()
		return a != nil && a[controller.PVCProtectionComplete] == "yes"
	})
}

func updatePVCAnnotationsAndSpec(pvc *v1.PersistentVolumeClaim, remoteClusterID string, pv *v1.PersistentVolume) {
	// Context prefix
	contextPrefix, _ := getValueFromAnnotations(controller.ContextPrefix, pv.Annotations)
	controller.AddAnnotation(pvc, controller.ContextPrefix, contextPrefix)
	// pvc protection complete
	pvcProtectionComplete, _ := getValueFromAnnotations(controller.PVCProtectionComplete, pv.Annotations)
	controller.AddAnnotation(pvc, controller.PVCProtectionComplete, pvcProtectionComplete)
	// Created By
	controller.AddAnnotation(pvc, controller.CreatedBy, constants.DellReplicationController)
	// Remote PV Name
	remoteVolume, _ := getValueFromAnnotations(controller.RemoteVolumeAnnotation, pv.Annotations)
	pvc.Spec.VolumeName = remoteVolume
	controller.AddAnnotation(pvc, controller.RemotePV, remoteVolume)
	// Remote ClusterID
	controller.AddAnnotation(pvc, controller.RemoteClusterID, remoteClusterID)
	// Replication group
	controller.AddAnnotation(pvc, controller.ReplicationGroup, pv.Labels[controller.ReplicationGroup])
	// Remote Storage Class
	pvc.Spec.StorageClassName = &pv.Spec.StorageClassName
	controller.AddAnnotation(pvc, controller.RemoteStorageClassAnnotation, pv.Spec.StorageClassName)
	// Remote Volume
	remoteVolume, _ = getValueFromAnnotations(controller.RemoteVolumeAnnotation, pv.Annotations)
	controller.AddAnnotation(pvc, controller.RemoteVolumeAnnotation, remoteVolume)
	// remote PVC namespace
	remotePVCNamespace, _ := getValueFromAnnotations(controller.RemotePVCNamespace, pv.Annotations)
	pvc.Namespace = remotePVCNamespace
	controller.AddAnnotation(pvc, controller.RemotePVCNamespace, remotePVCNamespace)
	// remote PVC name
	remotePVCName, _ := getValueFromAnnotations(controller.RemotePVC, pv.Annotations)
	pvc.Name = remotePVCName
	controller.AddAnnotation(pvc, controller.RemotePVC, remotePVCName)

	// Remote PV Annotation
	remotePVString := fmt.Sprintf("%s/%s", pv.Namespace, pv.Name)
	if pv.Annotations != nil {
		remotePVString += fmt.Sprintf(" annotations: %v", pv.Annotations)
	}
	if pv.Labels != nil {
		remotePVString += fmt.Sprintf(" labels: %v", pv.Labels)
	}
	pvc.Annotations[controller.RemoteVolumeAnnotation] = remotePVString
}

func updatePVCLabels(pvc, volume *v1.PersistentVolumeClaim, remoteClusterID string) {
	// Driver Name
	controller.AddLabel(pvc, controller.DriverName, volume.Labels[controller.DriverName])
	// Remote ClusterID
	controller.AddLabel(pvc, controller.RemoteClusterID, remoteClusterID)
	// Replication group name
	controller.AddLabel(pvc, controller.ReplicationGroup, volume.Labels[controller.ReplicationGroup])
}

func VerifyAndCreateNamespace(ctx context.Context, rClient connection.RemoteClusterClient, namespace string) error {
	// Verify if the namespace exists
	log := logger.GetLoggerFromContext(ctx)
	if _, err := rClient.GetNamespace(ctx, namespace); err != nil {
		log.V(logger.InfoLevel).Info("Namespace - " + namespace + " not found, creating it.")
		NewNamespace := &v1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "Namespace",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		err = rClient.CreateNamespace(ctx, NewNamespace)
		if err != nil {
			msg := "unable to create the desired namespace" + namespace
			log.V(logger.ErrorLevel).Error(err, msg)
			return err
		}
	}
	return nil
}
