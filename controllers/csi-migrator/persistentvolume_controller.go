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

package csi_migrator

import (
	"context"
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

	//if !shouldContinue(ctx, storageClass, r.DriverName) {
	//	return ctrl.Result{}, nil
	//	}

	// TODO: create target namespace annotation; check it, detect behavior

	return ctrl.Result{}, nil
}

func isMigrationRequested() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(meta client.Object) bool {
		a := meta.GetAnnotations()
		_, ok := a[controller.MigrationRequested]
		return a != nil && ok
	})
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
