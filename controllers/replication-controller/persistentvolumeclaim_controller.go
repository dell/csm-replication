/*
Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	reconcile "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"
	"time"

	"github.com/dell/csm-replication/pkg/connection"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PersistentVolumeClaimReconciler reconciles a PersistentVolumeClaim object
type PersistentVolumeClaimReconciler struct {
	client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme
	EventRecorder      record.EventRecorder
	PVCRequeueInterval time.Duration
	Config             connection.MultiClusterClient
	Domain             string
}

// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;update;patch;list;watch;create
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

// Reconcile contains reconciliation logic that updates PersistentVolumeClaim depending on it's current state
func (r *PersistentVolumeClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// If we have received the reconcile request, it means that the sidecar has completed its protection
	log := r.Log.WithValues("persistentvolumeclaim", req.NamespacedName)

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
		isRemotePVCUpdated, err = r.processRemotePVC(ctx, log, rClient, remoteClaim, req.Name, req.Namespace, localPVName)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		log.V(common.InfoLevel).Info("Remote PVC has not been created yet. Information can't be synced")
	}

	err = r.processLocalPVC(ctx, log, claim, remotePVName, remotePVCName, remotePVCNamespace, remoteClusterID, isRemotePVCUpdated)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(common.InfoLevel).Info("PVC Reconcile complete!!!!")
	return ctrl.Result{}, nil
}

func (r *PersistentVolumeClaimReconciler) processRemotePVC(ctx context.Context, log logr.Logger,
	rClient connection.RemoteClusterClient,
	claim *v1.PersistentVolumeClaim,
	remotePVCName, remotePVCNamespace, remotePVName string) (bool, error) {
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
		err := rClient.UpdatePersistentVolumeClaim(ctx, claim)
		if err != nil {
			return false, err
		}
		log.V(common.InfoLevel).Info("Successfully updated remote PVC with annotation")
		return true, nil
	}

	log.V(common.InfoLevel).Info("Remote PVC already has the annotations set")
	return true, nil
}

func (r *PersistentVolumeClaimReconciler) processLocalPVC(ctx context.Context, log logr.Logger,
	claim *v1.PersistentVolumeClaim, remotePVName, remotePVCName, remotePVCNamespace,
	remoteClusterID string, isRemotePVCUpdated bool) error {
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
	err := r.Update(ctx, claim)
	if err != nil {
		return err
	}

	if isRemotePVCUpdated {
		r.EventRecorder.Eventf(claim, eventTypeNormal, eventReasonUpdated,
			"PVC sync complete for ClusterId: %s", remoteClusterID)
	}
	return nil
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *PersistentVolumeClaimReconciler) SetupWithManager(mgr ctrl.Manager, limiter ratelimiter.RateLimiter, maxReconcilers int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.PersistentVolumeClaim{}, builder.WithPredicates(
			predicate.And(
				pvcProtectionIsComplete(),
			),
		)).WithOptions(reconcile.Options{
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
