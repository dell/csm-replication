/*
Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package csimigrator

import (
	"context"
	"github.com/dell/dell-csi-extensions/migration"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"
	csimigration "github.com/dell/csm-replication/pkg/csi-clients/migration"
	"github.com/go-logr/logr"
	"golang.org/x/sync/singleflight"
	v1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	reconcile "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"
)

// PersistentVolumeReconciler reconciles PersistentVolume resources
type PersistentVolumeReconciler struct {
	client.Client
	Log               logr.Logger
	Scheme            *runtime.Scheme
	EventRecorder     record.EventRecorder
	DriverName        string
	MigrationClient   csimigration.Migration
	ContextPrefix     string
	SingleFlightGroup singleflight.Group
	Domain            string
	ReplDomain        string
}

const protectionIndexKey = "protection_id"

// Reconcile contains reconciliation logic that updates PersistentVolume depending on it's current state
func (r *PersistentVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("persistentvolume", req.NamespacedName)
	ctx = context.WithValue(ctx, common.LoggerContextKey, log)

	log.V(common.InfoLevel).Info("Begin reconcile - PV Controller")

	pv := new(v1.PersistentVolume)
	if err := r.Get(ctx, req.NamespacedName, pv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	storageClass := new(storageV1.StorageClass)
	err := r.Get(ctx, client.ObjectKey{
		Name: pv.Spec.StorageClassName,
	}, storageClass)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "The storage class specified in the PV doesn't exist", "StorageClassName", pv.Spec.StorageClassName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch storage class of the PV", "StorageClassName", pv.Spec.StorageClassName)
		return ctrl.Result{}, err
	}
	targetStorageClassName := pv.Annotations[controller.MigrationRequested]
	targetStorageClassNameSpaced := types.NamespacedName{Name: targetStorageClassName}
	targetStorageClass := new(storageV1.StorageClass)
	if err := r.Get(ctx, targetStorageClassNameSpaced, targetStorageClass); err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "The storage class specified for migration doesn't exist", "StorageClassName", targetStorageClassName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch target storage class", "StorageClassName", targetStorageClassName)
		return ctrl.Result{}, err
	}

	// TODO:  check both SC's parameters, detect behavior
	if storageClass.Name == targetStorageClassName {
		log.Error(errors.NewBadRequest("Source SC == Target SC"), "Unable to migrate withing single SC")
		return ctrl.Result{}, nil
	}

	replParam := r.ReplDomain + "/isReplicationEnabled"
	_, isSourceReplicated := storageClass.Parameters[replParam]
	_, isTargetReplicated := targetStorageClass.Parameters[replParam]
	var migrateType migration.MigrateTypes
	switch true {
	case (isSourceReplicated && isTargetReplicated) || (!isSourceReplicated && !isTargetReplicated):
		migrateType = migration.MigrateTypes_VERSION_UPGRADE
	case !isSourceReplicated && isTargetReplicated:
		migrateType = migration.MigrateTypes_NON_REPL_TO_REPL
	case isSourceReplicated && !isTargetReplicated:
		migrateType = migration.MigrateTypes_REPL_TO_NON_REPL
	default:
		log.Info("Strange migration type..")

	}
	migrateReq := &migration.VolumeMigrateRequest_Type{
		Type: migrateType,
	}
	var targetPVCNamespace string
	if ns, ok := pv.Annotations[controller.MigrationNamespace]; ok {
		targetPVCNamespace = ns
	} else if pv.Spec.ClaimRef != nil {
		targetPVCNamespace = pv.Spec.ClaimRef.Namespace
	} else {
		log.Error(errors.NewBadRequest("Unable to detect target NS"), "No annotation for target NS specified and unable to retrieve information from PVC")
		return ctrl.Result{}, nil
	}
	targetStorageClass.Parameters["csi.storage.k8s.io/pvc/namespace"] = targetPVCNamespace
	migrate, err := r.MigrationClient.VolumeMigrate(ctx, pv.Spec.CSI.VolumeHandle, targetStorageClassName, migrateReq, targetStorageClass.Parameters, storageClass.Parameters, false)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(common.DebugLevel).Info("Checking if a Migrated PV instance already exists")

	gotPv := new(v1.PersistentVolume)
	if err = r.Get(ctx, client.ObjectKey{
		Name: pv.Name + "-to-" + targetStorageClassName,
	}, gotPv); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to check for a pre-existing PV")
		return ctrl.Result{}, err
	}
	log.V(common.InfoLevel).Info("checked for already created PV. Result: ", err)
	if _, ok := gotPv.Annotations[controller.CreatedByMigrator]; !ok {
		pvT := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: pv.Name + "-to-" + targetStorageClassName,
				Annotations: map[string]string{
					controller.CreatedByMigrator:       "true",
					"csi.storage.k8s.io/pvc/namespace": targetPVCNamespace,
				},
			},
			Spec: v1.PersistentVolumeSpec{
				PersistentVolumeSource: v1.PersistentVolumeSource{
					CSI: &v1.CSIPersistentVolumeSource{
						Driver:           targetStorageClass.Provisioner,
						VolumeHandle:     migrate.GetMigratedVolume().GetVolumeId(),
						ReadOnly:         pv.Spec.CSI.ReadOnly,
						FSType:           migrate.GetMigratedVolume().GetFsType(),
						VolumeAttributes: migrate.GetMigratedVolume().GetVolumeContext(),
					},
				},
				StorageClassName: targetStorageClassName,
				AccessModes:      pv.Spec.AccessModes,
				MountOptions:     pv.Spec.MountOptions,
				Capacity:         v1.ResourceList{v1.ResourceStorage: bytesToQuantity(migrate.GetMigratedVolume().CapacityBytes)}},
		}
		log.V(common.InfoLevel).Info("trying to create migrated PV")
		err = r.Create(ctx, pvT, &client.CreateOptions{})
		if err != nil {
			log.V(common.ErrorLevel).Error(err, "migrated PV creation failed")
			return ctrl.Result{}, err
		}
	}

	pv.Spec.PersistentVolumeReclaimPolicy = v1.PersistentVolumeReclaimRetain
	log.V(common.InfoLevel).Info("removing annotation..")
	delete(pv.Annotations, controller.MigrationRequested)
	err = r.Update(ctx, pv, &client.UpdateOptions{})
	if err != nil {
		log.V(common.ErrorLevel).Error(err, "failed to update the PV")
		return ctrl.Result{}, err
	}
	log.V(common.InfoLevel).Info("Successfully created PV, publishing event..")
	r.EventRecorder.Eventf(pv, "Normal", "Migrated", "This PV has been successfully migrated to SC %s,"+
		" consider using new PV %s.", targetStorageClassName, pv.Name+"-to-"+targetStorageClassName)
	return ctrl.Result{}, nil

}

func isMigrationRequested() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(meta client.Object) bool {
		a := meta.GetAnnotations()
		_, ok := a[controller.MigrationRequested]
		return a != nil && ok
	})
}

func bytesToQuantity(bytes int64) resource.Quantity {
	quantity := resource.NewQuantity(bytes, resource.BinarySI)
	return *quantity
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *PersistentVolumeReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, limiter ratelimiter.RateLimiter, maxReconcilers int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.PersistentVolume{}, builder.WithPredicates(
			predicate.Or(
				isMigrationRequested(),
			),
		)).WithOptions(reconcile.Options{
		RateLimiter:             limiter,
		MaxConcurrentReconciles: maxReconcilers,
	}).
		Complete(r)
}
