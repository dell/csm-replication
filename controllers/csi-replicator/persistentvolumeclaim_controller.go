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

package csireplicator

import (
	"context"
	"fmt"
	"strings"

	"github.com/dell/csm-replication/pkg/common/logger"

	"golang.org/x/sync/singleflight"

	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/errors"

	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"

	v1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controller "github.com/dell/csm-replication/controllers"
)

// PersistentVolumeClaimReconciler reconciles a PersistentVolumeClaim object
type PersistentVolumeClaimReconciler struct {
	client.Client
	Log               logr.Logger
	Scheme            *runtime.Scheme
	EventRecorder     record.EventRecorder
	DriverName        string
	ReplicationClient csireplication.Replication
	ContextPrefix     string
	SingleFlightGroup singleflight.Group
	Domain            string
}

// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;update;patch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch

// Reconcile contains reconciliation logic that updates PersistentVolumeClaim depending on it's current state
func (r *PersistentVolumeClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("persistentvolumeclaim", req.NamespacedName)
	ctx = context.WithValue(ctx, logger.LoggerContextKey, log)
	log.V(logger.InfoLevel).Info("Begin reconcile - PVC controller")

	claim := new(v1.PersistentVolumeClaim)
	err := r.Get(ctx, req.NamespacedName, claim)
	if err != nil {
		log.Error(err, "Failed to find claim's namespace", "req.NamespacedName", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scName string
	scList := new(storageV1.StorageClassList)
	if claim.Spec.StorageClassName == nil {
		listError := func() error {
			err = r.List(ctx, scList, &client.ListOptions{})
			for _, sc := range scList.Items {
				if _, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]; ok {
					scName = sc.Name
					return nil
				}
			}
			log.Error(err, "No StorageClass specified and no default SC present in cluster")
			return client.IgnoreNotFound(fmt.Errorf("%s", "Default SC not found "))
		}()
		if listError != nil {
			return ctrl.Result{}, listError
		}
	} else {
		scName = *claim.Spec.StorageClassName
	}

	storageClass := new(storageV1.StorageClass)
	err = r.Get(ctx, client.ObjectKey{
		Name: scName,
	}, storageClass)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "The storage class specified in the PVC doesn't exist", "storageClass", storageClass)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch storage class of the PVC", "storageClass", storageClass)
		return ctrl.Result{}, err
	}

	if !shouldContinue(ctx, storageClass, r.DriverName) {
		return ctrl.Result{}, nil
	}

	log.V(logger.DebugLevel).Info("Checking for the PVC state")

	if claim.Status.Phase != v1.ClaimBound {
		log.V(logger.InfoLevel).Info("PVC not in bound state yet", "claim", claim)
		return ctrl.Result{}, nil
	}

	log.V(logger.DebugLevel).Info("Getting VolumeHandle from the PV")

	pv := new(v1.PersistentVolume)
	if err := r.Get(ctx, client.ObjectKey{
		Name: claim.Spec.VolumeName,
	}, pv); err != nil {
		log.Error(err, "Failed to fetch PV details of the PVC", "pv", pv, "claim", claim)
		return ctrl.Result{}, err
	}

	isClaimUpdated := false
	// Add remote-volume-annotations and labels to the PVC, if not already exist
	if _, ok := claim.Annotations[controller.RemoteVolumeAnnotation]; !ok {
		if _, ok := pv.Annotations[controller.RemoteVolumeAnnotation]; !ok {
			log.V(logger.InfoLevel).Info("Waiting for RemoteVolume to be created by PV controller and set the corresponding annotation")
			return ctrl.Result{Requeue: true, RequeueAfter: controller.DefaultRetryInterval}, nil
		}
		if err := r.processClaimForRemoteVolume(ctx, claim.DeepCopy(), pv, storageClass.Parameters, pv.Annotations[controller.RemoteVolumeAnnotation]); err != nil {
			log.Error(err, "Failed to fetch PV details of the PVC", "pv", pv, "claim", claim)
			return ctrl.Result{}, err
		}
		isClaimUpdated = true
	}

	_, ok := claim.Annotations[controller.ReplicationGroup]
	if !ok {
		if isClaimUpdated {
			// PVC has already been updated with remote-volume-annotations,
			// in the current reconcile invocation, so fetching a new PVC object
			if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
				log.Error(err, "Failed to get PVC to add replication-group annotation", "claim", claim)
				return ctrl.Result{}, err
			}
			log.V(logger.InfoLevel).Info("Successfully fetched the PVC again to add replication-group annotation to it")
		}
		var err error

		_, ok = pv.Annotations[controller.ReplicationGroup]
		if ok {
			if pv.Annotations[controller.ReplicationGroup] == "" {
				log.V(logger.InfoLevel).Info("RG annotation is empty on PV, retry..")
				return ctrl.Result{Requeue: true, RequeueAfter: controller.DefaultRetryInterval}, nil
			}
		} else {
			log.V(logger.InfoLevel).Info("RG annotation is missing on PV, retry..")
			return ctrl.Result{Requeue: true, RequeueAfter: controller.DefaultRetryInterval}, nil
		}
		err = r.processClaimForReplicationGroup(ctx, claim.DeepCopy(), pv)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *PersistentVolumeClaimReconciler) processClaimForRemoteVolume(ctx context.Context, claim *v1.PersistentVolumeClaim,
	pv *v1.PersistentVolume, scParams map[string]string, buffer string,
) error {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Begin process claim for remote-volume")

	log.V(logger.DebugLevel).Info("Adding remote-volume annotation to the PVC")
	controller.AddAnnotation(claim, controller.RemoteVolumeAnnotation, buffer)
	log.V(logger.DebugLevel).Info("Adding remote-storage-class")
	controller.AddAnnotation(claim, controller.RemoteStorageClassAnnotation, scParams[controller.StorageClassRemoteStorageClassParam])
	log.V(logger.DebugLevel).Info("Adding remote-cluster annotation and label")
	if remoteCluster, ok := scParams[controller.StorageClassRemoteClusterParam]; ok {
		controller.AddAnnotation(claim, controller.RemoteClusterID, remoteCluster)
		controller.AddLabel(claim, controller.RemoteClusterID, remoteCluster)
		controller.AddLabel(claim, controller.DriverName, r.DriverName)
	}
	if r.ContextPrefix != "" {
		if pv.Spec.CSI != nil {
			for key, value := range pv.Spec.CSI.VolumeAttributes {
				if strings.HasPrefix(key, r.ContextPrefix) {
					labelKey := fmt.Sprintf("%s%s", r.Domain, strings.TrimPrefix(key, r.ContextPrefix))
					controller.AddLabel(claim, labelKey, value)
				}
			}
			controller.AddAnnotation(claim, controller.ContextPrefix, r.ContextPrefix)
		}
	}
	err := r.Update(ctx, claim)
	if err != nil {
		log.Error(err, "Failed to add remote volume annotation to the pvc", "claim", claim)
		return err
	}
	log.V(logger.InfoLevel).Info("RemoteVolume annotation added to the PVC")
	return nil
}

func (r *PersistentVolumeClaimReconciler) processClaimForReplicationGroup(ctx context.Context, claim *v1.PersistentVolumeClaim, pv *v1.PersistentVolume) error {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Begin process claim for replication-group")

	log.V(logger.DebugLevel).Info("Adding replication-group annotation to the PVC")
	controller.AddAnnotation(claim, controller.ReplicationGroup, pv.Annotations[controller.ReplicationGroup])
	log.V(logger.DebugLevel).Info("Adding PVC protection complete annotation")
	controller.AddAnnotation(claim, controller.PVCProtectionComplete, "yes")
	log.V(logger.DebugLevel).Info("Adding replication-group label to the PVC")
	controller.AddLabel(claim, controller.ReplicationGroup, pv.Annotations[controller.ReplicationGroup])

	if err := r.Update(ctx, claim); err != nil {
		log.Error(err, "Failed to add replication-group annotation to the PVC", "claim", claim)
		return err
	}
	log.V(logger.InfoLevel).Info("Replication-group annotation and label added to the pvc")
	r.EventRecorder.Eventf(claim, v1.EventTypeNormal, "Updated", "DellCSIReplicationGroup[%s] annotation added to the pvc", pv.Annotations[controller.ReplicationGroup])

	return nil
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *PersistentVolumeClaimReconciler) SetupWithManager(mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.PersistentVolumeClaim{}).
		WithOptions(reconciler.Options{
			MaxConcurrentReconciles: maxReconcilers,
			RateLimiter:             limiter,
		}).
		Complete(r)
}
