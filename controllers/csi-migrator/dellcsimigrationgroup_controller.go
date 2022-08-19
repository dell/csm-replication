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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"

	"github.com/dell/csm-replication/pkg/common"
	csimigration "github.com/dell/csm-replication/pkg/csi-clients/migration"
	migration "github.com/dell/dell-csi-extensions/migration"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
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
	// CommittedState name of state of MigrationGroup post commit operation
	CommittedState = "Committed"
	// DeletingState name deletion state of MigrationGroup
	DeletingState = "Deleting"
	//ArrayMigrationState key used to annotate MigrationGroup CR through ArrayMigration phases
	ArrayMigrationState = "ArrayMigrate"
	// MaxRetryDurationForActions maximum amount of time between retries of failed action
	MaxRetryDurationForActions = 1 * time.Hour
	// MaxNumberOfConditions maximum length of conditions list
	MaxNumberOfConditions = 20
	// SymmetrixIDParam key for storing arrayID
	SymmetrixIDParam = "SYMID"
	// RemoteSymIDParam key for storing remote arrayID
	RemoteSymIDParam = "RemoteSYMID"
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
	MaxRetryDurationForActions time.Duration
}

// ActionAnnotation represents annotation that contains information about migration action
type ActionAnnotation struct {
	ActionName string `json:"name"`
}

// NodeList has node names on which rescan will happen
type NodeList struct {
	NodeNames map[string]string
	AllSynced bool
}

var NodesToRescan NodeList

// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsimigrationgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsimigrationgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch

// Reconcile contains reconciliation logic that updates MigrationGroup depending on it's current state
func (r *MigrationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("MigrationGroup", req.NamespacedName)
	ctx = context.WithValue(ctx, common.LoggerContextKey, log)

	log.V(common.InfoLevel).Info("Begin reconcile - MG Controller")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err := r.Client.Get(ctx, req.NamespacedName, mg)
	if err != nil {
		log.Error(err, "MG not found", "mg", mg)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	currentState := mg.Status.State

	// Handle deletion by checking for deletion timestamp
	if !mg.DeletionTimestamp.IsZero() && strings.Contains(currentState, DeletingState) {
		return r.processMGForDeletion(ctx, mg.DeepCopy())
	}

	var NodeRescan bool //Defining a flag to call the node rescan call
	var NextState, NextAnnotationAction string
	var ArrayMigrationAction migration.ActionTypes
	NodeRescan = false
	switch currentState {
	case NoState:
		log.V(common.InfoLevel).Info("Processing MG with no state")
		return r.processMGInNoState(ctx, mg.DeepCopy())
	case ErrorState:
		if mg.Status.LastAction != "" {
			NextState = mg.Status.LastAction
		} else {
			NextState = ReadyState
		}
		NextAnnotationAction = "Retry"
	case ReadyState:
		ArrayMigrationAction = migration.ActionTypes_MG_MIGRATE
		NextState = MigratedState
		NextAnnotationAction = "Migrate"
	case MigratedState:
		NodeRescan = true
		NextState = CommitReadyState
		NextAnnotationAction = "NodeRescan"
	case CommitReadyState:
		ArrayMigrationAction = migration.ActionTypes_MG_COMMIT
		NextState = CommittedState
		NextAnnotationAction = "Commit"
	case CommittedState:
		NextState = DeletingState
		NextAnnotationAction = "Delete"
	case DeletingState:
		log.Info("Migration has completed successfully; MigrationGroup can be deleted")
	default:
		log.Info("Strange migration type..")
		return ctrl.Result{}, fmt.Errorf("Unknown state")
	}

	if NodeRescan {
		// Wait for rescan on all nodes
		if !NodesToRescan.AllSynced {
			// Get all Node Pods in driver's namespace
			podList := &corev1.PodList{}
			label := strings.Replace(mg.Spec.DriverName, "csi-", "", 1) + "-node"
			opts := []client.ListOption{
				client.MatchingLabels{"app": label},
			}
			err = r.Client.List(ctx, podList, opts...)
			if err != nil && errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			// Sync node for scanning
			allNodesScanned := true
			for _, nodePod := range podList.Items {
				labels := nodePod.GetLabels()
				if _, ok := labels[controllers.NodeReScanned]; !ok {
					log.Info("Awaiting rescan on Nodes")
					allNodesScanned = false
					break
				}
			}
			// All scanning done, in next reconcile skip this step
			if allNodesScanned {
				NodesToRescan.AllSynced = true
			}
		}
		// Check if all node rescanned
		if !NodesToRescan.AllSynced {
			err := fmt.Errorf("few nodes awaiting rescan")
			return ctrl.Result{}, err
		}
	} else if (NextAnnotationAction == "Migrate") || (NextAnnotationAction == "Commit") {

		//Include format and config validation on the driver side
		ArrayMigrateReqParams := map[string]string{
			"DriverName":     mg.Spec.DriverName,
			SymmetrixIDParam: mg.Spec.SourceID,
			RemoteSymIDParam: mg.Spec.TargetID,
		}

		ActionType := &migration.Action{
			ActionTypes: ArrayMigrationAction,
		}

		ArrayMigrateReq := &migration.ArrayMigrateRequest_Action{
			Action: ActionType,
		}

		ArrayMigrateResponse, err := r.MigrationClient.ArrayMigrate(ctx, ArrayMigrateReq, ArrayMigrateReqParams)
		mg.Status.LastAction = currentState
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
	} else if NextAnnotationAction == "Delete" {
		//reset NodesToRescan
		NodesToRescan.AllSynced = false
		mg.Status.LastAction = currentState
	}
	//update state field of the mg
	if err := r.updateMGSpecWithState(ctx, mg.DeepCopy(), NextState); err != nil {
		return ctrl.Result{}, err
	}
	//update annotation
	isSpecUpdated := r.updateMGSpecWithActionResult(ctx, mg, NextAnnotationAction)
	if isSpecUpdated {
		if err := r.Client.Update(ctx, mg.DeepCopy()); err != nil {
			log.Error(err, "Failed to update spec", "mg", mg, "Next State", NextState)
			return ctrl.Result{}, err
		}
		log.V(common.InfoLevel).Info("Successfully updated spec", "Next State", NextState)
	}
	return ctrl.Result{}, err
}

//Getting MG to its first valid state
func (r *MigrationGroupReconciler) processMGInNoState(ctx context.Context, dellCSIMigrationGroup *storagev1alpha1.DellCSIMigrationGroup) (ctrl.Result, error) {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Processing MG in NoState")
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
	mg.Status.State = NextState
	if err := r.Status().Update(ctx, mg.DeepCopy()); err != nil {
		log.Error(err, "Failed updating to", "State", NextState)
		return err
	}
	log.V(common.InfoLevel).Info("Successfully updated to", "state", NextState)
	return nil
}

//Update mg with annotation
func (r *MigrationGroupReconciler) updateMGSpecWithActionResult(ctx context.Context, mg *storagev1alpha1.DellCSIMigrationGroup, NextState string) bool {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Begin updating MG status with", "Annotation", NextState)

	isUpdated := false
	actionAnnotation := ActionAnnotation{
		ActionName: NextState,
	}
	bytes, _ := json.Marshal(&actionAnnotation)
	controllers.AddAnnotation(mg, ArrayMigrationState, string(bytes))
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
		if err := r.Update(ctx, mg); err != nil {
			log.Error(err, "Failed to add finalizer", "mg", mg)
			return ok, err
		}
		log.V(common.DebugLevel).Info("Successfully add finalizer. Requesting a requeue")
	}
	return ok, nil
}

//processing for deletion
func (r *MigrationGroupReconciler) processMGForDeletion(ctx context.Context, dellCSIMigrationGroup *storagev1alpha1.DellCSIMigrationGroup) (ctrl.Result, error) {
	//log := common.GetLoggerFromContext(ctx)

	if dellCSIMigrationGroup.Status.State != DeletingState {
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
