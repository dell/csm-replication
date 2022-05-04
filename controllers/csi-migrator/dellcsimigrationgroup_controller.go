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
	Completed  bool   `json:"completed"`
	FinalError string `json:"finalError"`
	FinishTime string `json:"finishTime"`
}

// ActionResult represents end result of migration action
type ActionResult struct {
	ActionType   ActionType
	Time         time.Time
	Error        error
	IsFinalError bool
}

//Helper modules
func (a ActionType) String() string {
	return strings.ToUpper(string(a))
}

// Equals allows to check if provided string is equal to current action type
func (a ActionType) Equals(ctx context.Context, val string) bool {
	log := common.GetLoggerFromContext(ctx)
	if strings.ToUpper(string(a)) == strings.ToUpper(val) {
		log.V(common.DebugLevel).Info("Current action type is equal", "val", val, "a", string(a))
		return true
	}
	log.V(common.DebugLevel).Info("Current action type is not equal", "val", val, "a", string(a))
	return false
}

func (a ActionType) getInProgressState() string {
	return fmt.Sprintf("%s_IN_PROGRESS", strings.ToUpper(string(a)))
}

func (a ActionType) getSuccessfulString() string {
	return fmt.Sprintf("Action %s succeeded", a.String())
}

func (a ActionType) getErrorString(errorMsg string) string {
	return fmt.Sprintf("Action %s failed with error %s", a.String(), errorMsg)
}

func updateMGSpecWithActionResult(ctx context.Context, rg *storagev1alpha1.DellCSIMigrationGroup, result *ActionResult) bool {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Begin updating MG spec with", "Action Result", result)

	isUpdated := false
	actionAnnotation := ActionAnnotation{
		ActionName: result.ActionType.String(),
	}
	if result.Error != nil && result.IsFinalError {
		actionAnnotation.FinalError = result.Error.Error()
	}
	if (result.Error != nil && result.IsFinalError) || (result.Error == nil) {
		rg.Spec.Action = ""
		actionAnnotation.Completed = true
		finishTime := &metav1.Time{Time: result.Time}
		buff, _ := json.Marshal(finishTime)
		actionAnnotation.FinishTime = string(buff)
		bytes, _ := json.Marshal(&actionAnnotation)
		controllers.AddAnnotation(rg, Action, string(bytes))

		log.V(common.InfoLevel).Info("MG was successfully updated with", "Action Result", result)

		isUpdated = true
		return isUpdated
	}

	log.V(common.InfoLevel).Info("MG was not updated with", "Action Result", result)
	return isUpdated
}

func getActionResultFromActionAnnotation(ctx context.Context, actionAnnotation ActionAnnotation) (*ActionResult, error) {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Getting result from action annotation..")

	var finalErr error
	finalError := false
	if actionAnnotation.FinalError != "" {
		log.V(common.InfoLevel).Info("There is final error", "actionAnnotation.FinalError", actionAnnotation.FinalError)
		finalErr = fmt.Errorf(actionAnnotation.FinalError)
		finalError = true
	}

	var finishTime metav1.Time
	err := json.Unmarshal([]byte(actionAnnotation.FinishTime), &finishTime)
	if err != nil {
		log.Error(err, "Cannot unmarshall file")
		return nil, err
	}

	actionResult := ActionResult{
		ActionType:   ActionType(actionAnnotation.ActionName),
		Time:         time.Time{},
		Error:        finalErr,
		IsFinalError: finalError,
	}
	return &actionResult, nil
}

/////////////////////////////////
// Reconcile contains reconciliation logic that updates MigrationGroup depending on it's current state
func (r *MigrationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//Not sure what we are doing her - nidtara
	log := r.Log.WithValues("persistentvolumeclaim", req.NamespacedName)
	ctx = context.WithValue(ctx, common.LoggerContextKey, log)
	//
	log.V(common.InfoLevel).Info("Begin reconcile - MG Controller")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err := r.Get(ctx, req.NamespacedName, mg) // getting the namespace name of the mg
	if err != nil {
		log.Error(err, "MG not found", "mg", mg)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	currentState := mg.Spec.State

	//nidtara some more code to fetch values
	switch currentState {
	case "Created":

	//
	case "Migrated":
	case "Commit_Ready":
	case "Commited":
	default:
		return ctrl.Result{}, fmt.Errorf("Unknown state")
		log.Info("Strange migration type..")

	}
	return ctrl.Result{}, nil
}

func (r *MigrationGroupReconciler) processMG(ctx context.Context, dellCSIMigrationGroup *storagev1alpha1.DellCSIMigrationGroup) (ctrl.Result, error) {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Start processing MG")

	actionType := ActionType(dellCSIMigrationGroup.Spec.Action)
	_, err = r.getAction(actionType)
	// Invalid action type
	if err != nil {
		// Reset the action to empty & raise an event
		// Most importantly finish the reconcile
		log.Error(err, "Invalid action type or not supported by driver", "actionType", actionType)
		/*dellCSIReplicationGroup.Spec.Action = ""
		err1 := r.Update(ctx, dellCSIReplicationGroup)
		if err1 != nil {
			log.Error(err, "Failed to update", "dellCSIReplicationGroup", dellCSIReplicationGroup)
			return ctrl.Result{}, err1
		*/
		return ctrl.Result{}, nil
	}

	//Update the Action
	actionAnnotation := ActionAnnotation{
		ActionName: actionType.String(),
	}
	bytes, _ := json.Marshal(&actionAnnotation)
	controllers.AddAnnotation(dellCSIMigrationGroup, Action, string(bytes))
	log.V(common.InfoLevel).Info("Updating", "annotation", string(bytes))
	err := r.Update(ctx, dellCSIMigrationGroup)
	if err != nil {
		log.Error(err, "Failed to update", "annotation", string(bytes))
		return ctrl.Result{}, err
	}

	dellCSIMigrationGroup.Spec.Action = actionType.getInProgressState()

	log.V(common.InfoLevel).Info("Updating", "state", actionType.getInProgressState())
	/*
		err = r.Status().Update(ctx, dellCSIMigrationGroup.DeepCopy())
		log.Error(err, "Failed to update", "state", actionType.getInProgressState())
	*/
	return ctrl.Result{}, nil
}

/*
func (r *ReplicationGroupReconciler) processRGInNoState(ctx context.Context, dellCSIReplicationGroup *storagev1alpha1.DellCSIReplicationGroup) (ctrl.Result, error) {
	ok, err := r.addFinalizer(ctx, dellCSIReplicationGroup.DeepCopy())
	if err != nil {
		return ctrl.Result{}, err
	}
	if ok {
		return ctrl.Result{Requeue: true}, nil
	}
	if err := r.updateState(ctx, dellCSIReplicationGroup.DeepCopy(), ReadyState); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
*/

/*
func (r *ReplicationGroupReconciler) processRGForDeletion(ctx context.Context, dellCSIReplicationGroup *storagev1alpha1.DellCSIReplicationGroup) (ctrl.Result, error) {
	log := common.GetLoggerFromContext(ctx)

	if dellCSIReplicationGroup.Spec.ProtectionGroupID != "" {
		log.V(common.DebugLevel).Info("Deleting the protection-group associated with this replication-group")
		if err := r.deleteProtectionGroup(ctx, dellCSIReplicationGroup.DeepCopy()); err != nil {
			if !controllers.IsCSIFinalError(err) && dellCSIReplicationGroup.Status.State != DeletingState {
				if err := r.updateState(ctx, dellCSIReplicationGroup.DeepCopy(), DeletingState); err != nil {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, controllers.IgnoreIfFinalError(err)
		}
	} else {
		if dellCSIReplicationGroup.Status.State != DeletingState {
			err := r.updateState(ctx, dellCSIReplicationGroup.DeepCopy(), DeletingState)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	err := r.removeFinalizer(ctx, dellCSIReplicationGroup.DeepCopy())
	return ctrl.Result{}, err
}
*/

func getActionInProgress(ctx context.Context, annotations map[string]string) (*ActionAnnotation, error) {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.DebugLevel).Info("Getting the action in progress from annotation")

	val, ok := annotations[Action]
	if !ok {
		log.V(common.InfoLevel).Info("No action", "val", val)
		return nil, nil
	}
	var actionAnnotation ActionAnnotation
	err := json.Unmarshal([]byte(val), &actionAnnotation)
	if err != nil {
		log.Error(err, "JSON unmarshal error", "actionAnnotation", actionAnnotation)
		return nil, err
	}
	log.V(common.InfoLevel).Info("Action was got", "actionAnnotation", actionAnnotation)
	return &actionAnnotation, nil
}

func (r *MigrationGroupReconciler) updateState(ctx context.Context, mg *storagev1alpha1.DellCSIMigrationGroup, state string) error {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Updating to", "state", state)

	mg.Spec.State = state
	/*
		if err := r.Status().Update(ctx, rg); err != nil {
			log.Error(err, "Failed updating to", "state", state)
			return err
		}
	*/
	log.V(common.InfoLevel).Info("Successfully updated to", "state", state)
	return nil
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
