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
	"encoding/json"
	"fmt"
	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	csiext "github.com/dell/dell-csi-extensions/migration"
	"strings"
	"time"

	"github.com/dell/csm-replication/pkg/common"
	csimigration "github.com/dell/csm-replication/pkg/csi-clients/migration"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NoState empty state of MigrationGroup
	NoState = ""
	// ReadyState name of first valid state of MigrationGroup
	ReadyState = "Ready"
	// ErrorState name of error state of MigrationGroup
	ErrorState = "Error"
	// MigratedState name of post migrate call state of MigrationGroup
	MigratedState = "Migrated"
	// CommitReadyState name of state of MigrationGroup post node rescan operation
	CommitReadyState = "CommitReady"
	// DeletingState name deletion state of MigrationGroup
	DeletingState = "Deleting"
	// CurrentState name of current state of CR
	CurrentState = "CurrentState"
)

type ActionType string

// MigrationGroupReconciler reconciles PersistentVolume resources
type MigrationGroupReconciler struct {
	client.Client
	Log                        logr.Logger
	Scheme                     *runtime.Scheme
	EventRecorder              record.EventRecorder
	DriverName                 string
	MigrationClient            csimigration.Migration
	SupportedActions           []*csiext.SupportedActions
	MaxRetryDurationForActions time.Duration
}

// ActionAnnotation represents annotation that contains information about migration action
type ActionAnnotation struct {
	ActionName string `json:"name"`
}

// Reconcile contains reconciliation logic that updates MigrationGroup depending on it's current state
func (r *MigrationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("persistentvolumeclaim", req.NamespacedName)
	ctx = context.WithValue(ctx, common.LoggerContextKey, log)

	log.V(common.InfoLevel).Info("Begin reconcile - MG Controller")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err := r.Get(ctx, req.NamespacedName, mg)
	if err != nil {
		log.Error(err, "MG not found", "mg", mg)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	currentState := mg.Spec.State

	// Handle deletion by checking for deletion timestamp
	if !mg.DeletionTimestamp.IsZero() && strings.Contains(currentState, DeletingState) {
		return r.processMGForDeletion(ctx, mg.DeepCopy())
	}

	var NodeRescan bool //Defining a flag to call the node rescan call
	var NextState string
	var ArrayMigrationAction migration.ActionTypes
	NodeRescan = false
	switch currentState {
	case NoState:
		return r.processMGInNoState(ctx, mg.DeepCopy())
	case ErrorState:
		if mg.Spec.LastAction != "" {
			NextState = MigratedState
		}
		fallthrough
	case ReadyState:
		ArrayMigrationAction = migration.ActionTypes_MG_MIGRATE
		NextState = MigratedState
	case MigratedState:
		NodeRescan = true
		NextState = CommitReadyState
	case CommitReadyState:
		ArrayMigrationAction = migration.ActionTypes_MG_COMMIT
		NextState = DeletingState
	case DeletingState:
		log.Info("Array migration has completed successfully; MigrationGroup can be deleted manually")
		//Define what to do with CR in this state
	default:
		log.Info("Strange migration type..")
		return ctrl.Result{}, fmt.Errorf("Unknown state")

	}

	if NodeRescan {
		//Implement code to call node sidecar and run rescan on all node (future)
	} else {
		ArrayMigrateReq := &migration.ArrayMigrateRequest_Action{
			Action: ArrayMigrationAction,
		}

		ArrayMigrateReqParams = map[string]string{
			"DriverName":    mg.Spec.DriverName,
			"SourceArrayID": mg.Spec.SourceArrayID,
			"TargetArrayID": mg.Spec.TargetArrayID,
		}

		ArrayMigrateResponse, err := r.MigrationClient.ArrayMigrate(ctx, ArrayMigrateReq, ArrayMigrateReqParams)
		CurrentAction := ArrayMigrateResponse.GetAction()
		if err != nil {
			r.EventRecorder.Eventf(mg, v1.EventTypeWarning, "Error",
				"Action [%s] on DellCSIMigrationGroup [%s] failed with error [%s] ",
				CurrentAction.String(), mg.Name, err)
			NextState = ErrorState
			if err := r.updateMGSpecWithState(ctx, mg.DeepCopy(), NextState); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		if ArrayMigrateResponse.GetSuccess() {
			log.V(common.InfoLevel).Info("Successfully executed action [%s]", CurrentAction.String())
		}
	}

	//update state field of the mg
	if err := r.updateMGSpecWithState(ctx, mg.DeepCopy(), NextState); err != nil {
		return ctrl.Result{}, err
	}
	//update annotation
	isSpecUpdated := updateMGSpecWithActionResult(ctx, mg, NextState)
	if isSpecUpdated {
		if err := r.Update(ctx, mg.DeepCopy()); err != nil {
			log.Error(err, "Failed to update spec", "mg", mg, "Next State", NextState)
			return ctrl.Result{}, err
		}
		log.V(common.InfoLevel).Info("Successfully updated spec", "Next State", NextState)
	}
	return ctrl.Result, err
}

//Getting MG to its first valid state
func (r *MigrationGroupReconciler) processMGInNoState(ctx context.Context, dellCSIMigrationGroup *storagev1alpha1.DellCSIMigrationGroup) (ctrl.Result, error) {
	ok, err := r.addFinalizer(ctx, dellCSIMigrationGroup.DeepCopy())
	if err != nil {
		return ctrl.Result{}, err
	}
	if ok {
		return ctrl.Result{Requeue: true}, nil
	}
	if err := r.updateMGSpecWithState(ctx, dellCSIMigrationGroup.DeepCopy(), ReadyState); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

//Update mg spec with current state
func (r *MigrationGroupReconciler) updateMGSpecWithState(ctx context.Context, mg *storagev1alpha1.DellCSIMigrationGroup, NextState string) error {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Begin updating MG spec with", "State", NextState)
	mg.Spec.State = state
	if err := r.Update(ctx, mg.DeepCopy()); err != nil {
		log.Error(err, "Failed updating to", "State", NextState)
		return err
	}
	log.V(common.InfoLevel).Info("Successfully updated to", "state", NextState)
	return nil
}

//Update mg with annotation
func (r *MigrationGroupReconciler) updateMGSpecWithActionResult(ctx context.Context, mg *storagev1alpha1.DellCSIMigrationGroup, NextState string) bool {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Begin updating RG spec with", "Annotation", NextState)

	isUpdated := false
	actionAnnotation := ActionAnnotation{
		ActionName: NextState,
	}
	bytes, _ := json.Marshal(&actionAnnotation)
	controllers.AddAnnotation(mg, CurrentState, string(bytes))
	log.V(common.InfoLevel).Info("Updating", "annotation", string(bytes))
	err := r.Update(ctx, mg.DeepCopy())
	if err != nil {
		log.Error(err, "Failed to update", "annotation", string(bytes))
		return isUpdated
	}
	log.V(common.InfoLevel).Info("MG was successfully updated with", "Action Result", NextState)

	isUpdated = true
	return isUpdated
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *MigrationGroupReconciler) SetupWithManager(mgr ctrl.Manager, limiter ratelimiter.RateLimiter, maxReconcilers int) error {
	if r.MaxRetryDurationForActions == 0 {
		r.MaxRetryDurationForActions = MaxRetryDurationForActions
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&storagev1alpha1.DellCSIMigrationGroup{}).
		WithOptions(reconciler.Options{
			RateLimiter:             limiter,
			MaxConcurrentReconciles: maxReconcilers,
		}).
		Complete(r)
}

//Function to add a Finalizer to MG
func (r *MigrationGroupReconciler) addFinalizer(ctx context.Context, mg *storagev1alpha1.DellCSIMigrationGroup) (bool, error) {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Adding finalizer")

	ok := controllers.AddFinalizerIfNotExist(mg, controllers.MigrationFinalizer)
	if ok {
		if err := r.Update(ctx, mg.DeepCopy()); err != nil {
			log.Error(err, "Failed to add finalizer", "mg", mg)
			return ok, err
		}
		log.V(common.DebugLevel).Info("Successfully add finalizer. Requesting a requeue")
	}
	return ok, nil
}

//processing for deletion
func (r *MigrationGroupReconciler) processMGForDeletion(ctx context.Context, dellCSIMigrationGroup *storagev1alpha1.DellCSIMigrationGroup) (ctrl.Result, error) {
	log := common.GetLoggerFromContext(ctx)

	if dellCSIMigrationGroup.Spec.State != DeletingState {
		err := r.updateMGSpecWithState(ctx, dellCSIMigrationGroup.DeepCopy(), DeletingState)
		return ctrl.Result{}, err
	}
	err := r.removeFinalizer(ctx, dellCSIMigrationGroup.DeepCopy())
	return ctrl.Result{}, err
}

func (r *MigrationGroupReconciler) removeFinalizer(ctx context.Context, mg *storagev1alpha1.DellCSIMigrationGroup) error {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Removing finalizer")

	// Remove migration group finalizer
	if ok := controllers.RemoveFinalizerIfExists(mg, controllers.MigrationFinalizer); ok {
		// Adding annotation to mark the removal of protection-group
		//controllers.AddAnnotation(mg, controllers.MigrationGroupRemovedAnnotation, "yes")
		if err := r.Update(ctx, mg.DeepCopy()); err != nil {
			log.Error(err, "Failed to remove finalizer", "mg", mg, "MigrationGroupRemovedAnnotation")
			return err
		}
		log.V(common.InfoLevel).Info("Finalizer removed successfully")
	}
	return nil
}
