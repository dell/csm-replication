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

package csi_replicator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dell/csm-replication/pkg/common"

	v1 "k8s.io/api/core/v1"

	"strings"
	"time"

	csiext "github.com/dell/dell-csi-extensions/replication"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"

	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dell/csm-replication/controllers"
)

const (
	ReadyState                 = "Ready"
	InvalidState               = "Invalid"
	ErrorState                 = "Error"
	NoState                    = ""
	InProgress                 = "IN_PROGRESS"
	DeletingState              = "Deleting"
	Action                     = "Action"
	MaxRetryDurationForActions = 1 * time.Hour
	MaxNumberOfConditions      = 20
)

type ActionType string

func (a ActionType) String() string {
	return strings.ToUpper(string(a))
}

func (a ActionType) Equals(val string) bool {
	if strings.ToUpper(string(a)) == strings.ToUpper(val) {
		return true
	}
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

type ActionResult struct {
	ActionType   ActionType
	Time         time.Time
	Error        error
	IsFinalError bool
	PGStatus     *csiext.StorageProtectionGroupStatus
}

type ActionAnnotation struct {
	ActionName            string `json:"name"`
	Completed             bool   `json:"completed"`
	FinalError            string `json:"finalError"`
	FinishTime            string `json:"finishTime"`
	ProtectionGroupStatus string `json:"protectionGroupStatus"`
}

func updateRGSpecWithActionResult(rg *storagev1alpha1.DellCSIReplicationGroup, result *ActionResult) bool {
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
		if result.PGStatus != nil {
			buff, _ := json.Marshal(result.PGStatus)
			actionAnnotation.ProtectionGroupStatus = string(buff)
		}
		finishTime := &metav1.Time{Time: result.Time}
		buff, _ := json.Marshal(finishTime)
		actionAnnotation.FinishTime = string(buff)
		bytes, _ := json.Marshal(&actionAnnotation)
		controllers.AddAnnotation(rg, Action, string(bytes))
		isUpdated = true
	}
	return isUpdated
}

func getActionResultFromActionAnnotation(actionAnnotation ActionAnnotation) (*ActionResult, error) {
	var finalErr error
	finalError := false
	if actionAnnotation.FinalError != "" {
		finalErr = fmt.Errorf(actionAnnotation.FinalError)
		finalError = true
	}

	var finishTime metav1.Time
	err := json.Unmarshal([]byte(actionAnnotation.FinishTime), &finishTime)
	if err != nil {
		return nil, err
	}

	pgStatus := new(csiext.StorageProtectionGroupStatus)
	if actionAnnotation.ProtectionGroupStatus != "" {
		err = json.Unmarshal([]byte(actionAnnotation.ProtectionGroupStatus), pgStatus)
		if err != nil {
			return nil, err
		}
	}

	actionResult := ActionResult{
		ActionType:   ActionType(actionAnnotation.ActionName),
		Time:         time.Time{},
		Error:        finalErr,
		IsFinalError: finalError,
		PGStatus:     pgStatus,
	}
	return &actionResult, nil
}

func updateRGStatusWithActionResult(rg *storagev1alpha1.DellCSIReplicationGroup, actionResult *ActionResult) error {
	var result *ActionResult
	if actionResult == nil {
		actionAnnotation, err := getActionInProgress(rg.Annotations)
		if err != nil {
			return err
		}
		result, err = getActionResultFromActionAnnotation(*actionAnnotation)
		if err != nil {
			return err
		}
	} else {
		result = actionResult
	}
	if result.Error == nil {
		// Update to ReadyState
		rg.Status.State = ReadyState
	} else {
		if result.IsFinalError {
			rg.Status.State = ErrorState
		}
	}
	updateConditionsWithActionResult(rg, result)
	updateLastAction(rg, result)
	// Update the RG link state if we got a status
	if result.PGStatus != nil {
		updateRGLinkState(rg, result.PGStatus.State.String(), result.PGStatus.IsSource, "")
	}
	return nil
}

func updateConditionsWithActionResult(rg *storagev1alpha1.DellCSIReplicationGroup, result *ActionResult) {
	condition := storagev1alpha1.LastAction{
		Condition: result.ActionType.getSuccessfulString(),
		Time:      &metav1.Time{Time: result.Time},
	}
	if result.Error != nil && result.IsFinalError {
		condition.Condition = result.ActionType.getErrorString(result.Error.Error())
	}
	if result.Error == nil || (result.Error != nil && result.IsFinalError) {
		controllers.UpdateConditions(rg, condition, MaxNumberOfConditions)
	}
}

func updateLastAction(rg *storagev1alpha1.DellCSIReplicationGroup, result *ActionResult) {
	rg.Status.LastAction.Time = &metav1.Time{Time: result.Time}
	if result.Error != nil {
		rg.Status.LastAction.ErrorMessage = result.Error.Error()
		rg.Status.LastAction.Condition = result.ActionType.getErrorString(result.Error.Error())
		if rg.Status.LastAction.FirstFailure == nil {
			rg.Status.LastAction.FirstFailure = rg.Status.LastAction.Time
		}
	} else {
		rg.Status.LastAction.Condition = result.ActionType.getSuccessfulString()
		rg.Status.LastAction.ErrorMessage = "" // Reset any older errors
	}
}

type ReplicationGroupReconciler struct {
	client.Client
	Log                        logr.Logger
	Scheme                     *runtime.Scheme
	EventRecorder              record.EventRecorder
	DriverName                 string
	ReplicationClient          csireplication.Replication
	SupportedActions           []*csiext.SupportedActions
	MaxRetryDurationForActions time.Duration
}

// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsireplicationgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsireplicationgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch

func (r *ReplicationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("dellcsireplicationgroup", req.NamespacedName)

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err := r.Get(ctx, req.NamespacedName, rg)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	currentState := rg.Status.State

	// Handle deletion by checking for deletion timestamp
	if !rg.DeletionTimestamp.IsZero() && !strings.Contains(currentState, InProgress) {
		return r.processRGForDeletion(ctx, rg.DeepCopy(), log)
	}

	// If no protection group ID set(the only mandatory field), we set it to Invalid state and return
	if rg.Spec.ProtectionGroupID == "" {
		r.EventRecorder.Event(rg, v1.EventTypeWarning, "Invalid", "Missing mandatory value - protectionGroupID")
		if rg.Status.State != InvalidState {
			if err := r.updateState(ctx, rg.DeepCopy(), InvalidState, log); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, fmt.Errorf("missing protection group id")
		}
		return ctrl.Result{}, nil
	}

	switch rg.Status.State {
	case NoState:
		return r.processRGInNoState(ctx, rg.DeepCopy(), log)
	case InvalidState:
		// Update the state to no state
		err := r.updateState(ctx, rg.DeepCopy(), "", log)
		return ctrl.Result{}, err
	case ErrorState:
		fallthrough
	case ReadyState:
		return r.processRG(ctx, rg.DeepCopy(), log)
	default:
		if strings.Contains(rg.Status.State, InProgress) {
			return r.processRGInActionInProgressState(ctx, rg.DeepCopy(), log)
		} else {
			return ctrl.Result{}, fmt.Errorf("unknown state")
		}
	}
}

func (r *ReplicationGroupReconciler) getAction(actionType ActionType) (*csiext.ExecuteActionRequest_Action, error) {
	for _, supportedAction := range r.SupportedActions {
		if supportedAction == nil {
			continue
		}
		actionT := supportedAction.GetType()
		if actionT.String() == actionType.String() {
			var action = &csiext.ExecuteActionRequest_Action{
				Action: &csiext.Action{},
			}
			action.Action.ActionTypes = actionT
			return action, nil
		}
	}
	return nil, fmt.Errorf("unsupported action")
}

func (r *ReplicationGroupReconciler) deleteProtectionGroup(ctx context.Context, rg *storagev1alpha1.DellCSIReplicationGroup, log logr.Logger) error {
	// Make CSI call to delete replication group
	err := r.ReplicationClient.DeleteStorageProtectionGroup(ctx, rg.Spec.ProtectionGroupID, rg.Spec.ProtectionGroupAttributes)
	if err != nil {
		log.Error(err, "Failed to delete protection-group")
		return err
	}
	log.V(common.InfoLevel).Info("Successfully deleted the protection-group")
	return nil
}

func (r *ReplicationGroupReconciler) removeFinalizer(ctx context.Context, rg *storagev1alpha1.DellCSIReplicationGroup, log logr.Logger) error {
	// Remove replication-protection finalizer
	if ok := controllers.RemoveFinalizerIfExists(rg, controllers.ReplicationFinalizer); ok {
		// Adding annotation to mark the removal of protection-group
		controllers.AddAnnotation(rg, controllers.ProtectionGroupRemovedAnnotation, "yes")
		if err := r.Update(ctx, rg); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return err
		}
		log.V(common.InfoLevel).Info("Finalizer removed successfully")
	}
	return nil
}

func (r *ReplicationGroupReconciler) addFinalizer(ctx context.Context, rg *storagev1alpha1.DellCSIReplicationGroup, log logr.Logger) (bool, error) {
	ok := controllers.AddFinalizerIfNotExist(rg, controllers.ReplicationFinalizer)
	if ok {
		if err := r.Update(ctx, rg); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ok, err
		}
		log.V(common.DebugLevel).Info("Finalizer added successfully")
	}
	return ok, nil
}

func (r *ReplicationGroupReconciler) updateState(ctx context.Context, rg *storagev1alpha1.DellCSIReplicationGroup, state string, log logr.Logger) error {
	rg.Status.State = state
	if err := r.Status().Update(ctx, rg); err != nil {
		log.Error(err, "Failed to update the state")
		return err
	}
	log.V(common.InfoLevel).Info("State updated successfully")
	return nil
}

func (r *ReplicationGroupReconciler) SetupWithManager(mgr ctrl.Manager, limiter ratelimiter.RateLimiter, maxReconcilers int) error {
	if r.MaxRetryDurationForActions == 0 {
		r.MaxRetryDurationForActions = MaxRetryDurationForActions
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&storagev1alpha1.DellCSIReplicationGroup{}).
		WithOptions(reconciler.Options{
			RateLimiter:             limiter,
			MaxConcurrentReconciles: maxReconcilers,
		}).
		Complete(r)
}

func getActionInProgress(annotations map[string]string) (*ActionAnnotation, error) {
	val, ok := annotations[Action]
	if !ok {
		return nil, nil
	}
	var actionAnnotation ActionAnnotation
	err := json.Unmarshal([]byte(val), &actionAnnotation)
	if err != nil {
		return nil, err
	}
	return &actionAnnotation, nil
}

func resetRGSpecForInvalidAction(rg *storagev1alpha1.DellCSIReplicationGroup) {
	rg.Spec.Action = ""
	delete(rg.Annotations, Action)
}

func (r *ReplicationGroupReconciler) processRGInActionInProgressState(ctx context.Context,
	rg *storagev1alpha1.DellCSIReplicationGroup, log logr.Logger) (ctrl.Result, error) {
	// Get the action in progress from annotation
	inProgress, err := getActionInProgress(rg.Annotations)
	if err != nil || inProgress == nil {
		// Either the annotation is not set or not set properly
		// Mostly points to User error
		if rg.Spec.Action != "" {
			// Action set
			actionType := ActionType(rg.Spec.Action)
			_, err := r.getAction(actionType)
			if err != nil {
				// Action invalid
				r.EventRecorder.Event(rg, v1.EventTypeWarning, "Invalid",
					"State is InProgress & Invalid action set in spec")
				log.V(common.InfoLevel).Info("Warning: State is InProgress but invalid action set in spec")
				log.V(common.InfoLevel).Info("Resetting the CR state to Ready")
				resetRGSpecForInvalidAction(rg)
				err = r.Update(ctx, rg)
				return ctrl.Result{}, err
			}
			// Action valid
			r.EventRecorder.Event(rg, v1.EventTypeWarning, "Invalid",
				"State is InProgress but missing action annotation")
			actionAnnotation := ActionAnnotation{
				ActionName: actionType.String(),
				Completed:  false,
			}
			bytes, _ := json.Marshal(&actionAnnotation)
			controllers.AddAnnotation(rg, Action, string(bytes))
			if err := r.Update(ctx, rg); err != nil {
				return ctrl.Result{}, err
			}
			inProgress = &actionAnnotation
		} else {
			// Action not set
			// Nothing to do here
			// What should be the final state?
			r.EventRecorder.Event(rg, v1.EventTypeWarning, "Invalid",
				"State is InProgress but action & action annotation not set")
			log.V(common.InfoLevel).Info("Warning: State is InProgress but action & action annotation not set")
			log.V(common.InfoLevel).Info("Resetting the CR state to Ready")
			// Delete any incorrect annotation
			resetRGSpecForInvalidAction(rg)
			if err := r.Update(ctx, rg); err != nil {
				return ctrl.Result{}, err
			}
			err = r.updateState(ctx, rg, ReadyState, log)
			return ctrl.Result{}, err
		}
	}
	if inProgress.Completed {
		// If the action field is still set, then reset it
		if rg.Spec.Action != "" {
			rg.Spec.Action = ""
			if err := r.Update(ctx, rg); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Update the status
		if err := updateRGStatusWithActionResult(rg, nil); err == nil {
			err1 := r.Status().Update(ctx, rg.DeepCopy())
			return ctrl.Result{}, err1
		}
	}
	actionType := ActionType(inProgress.ActionName)
	if !actionType.Equals(rg.Spec.Action) {
		r.EventRecorder.Eventf(rg, v1.EventTypeWarning, "InProgress",
			"Action [%s] on DellCSIReplicationGroup [%s] is in Progress, cannot execute [%s] ", actionType.String(), rg.Name, rg.Spec.Action)
	}
	action, err := r.getAction(actionType)
	if err != nil {
		r.EventRecorder.Eventf(rg, v1.EventTypeWarning, "Unsupported",
			"Action changed to an invalid value %s while another action execution was in progress. [%s]",
			actionType.String(), err.Error())
		log.Error(err, "can not proceed!")
		err = r.updateState(ctx, rg.DeepCopy(), ErrorState, log)
		return ctrl.Result{}, err
	}
	// Make API call to Execute Action
	actionResult := r.executeAction(ctx, rg.DeepCopy(), actionType, action, log)
	if actionResult.Error != nil {
		// Raise event in case of an error
		r.EventRecorder.Eventf(rg, v1.EventTypeWarning, "Error",
			"Action [%s] on DellCSIReplicationGroup [%s] failed with error [%s] ",
			actionType.String(), rg.Name, actionResult.Error.Error())
	}
	// Update spec
	isSpecUpdated := updateRGSpecWithActionResult(rg, actionResult)
	if isSpecUpdated {
		if err := r.Update(ctx, rg); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Successfully updated spec", "Action Result", actionResult)
	}
	// Update status
	err = updateRGStatusWithActionResult(rg, actionResult)
	if err != nil {
		r.Log.Error(err, "failed to update status with action result")
		return ctrl.Result{}, err
	}
	err = r.Status().Update(ctx, rg)
	if err != nil {
		r.Log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}
	log.Info("Successfully updated status", "state", rg.Status.State)
	// In case of Action success & successful status update, we raise an event
	if actionResult.Error == nil {
		r.EventRecorder.Eventf(rg, v1.EventTypeNormal, "Updated",
			"DellCSIReplicationGroup [%s] state updated to [%s] after successful action ", rg.Name, ReadyState)
	}

	return ctrl.Result{}, controllers.IgnoreIfFinalError(actionResult.Error)
}

func (r *ReplicationGroupReconciler) executeAction(ctx context.Context, rg *storagev1alpha1.DellCSIReplicationGroup,
	actionType ActionType, action *csiext.ExecuteActionRequest_Action, log logr.Logger) *ActionResult {
	actionResult := ActionResult{
		ActionType: actionType,
		PGStatus:   nil,
	}
	// Make API call to Execute Action
	res, err := r.ReplicationClient.ExecuteAction(ctx, rg.Spec.ProtectionGroupID, action,
		rg.Spec.ProtectionGroupAttributes, rg.Spec.RemoteProtectionGroupID, rg.Spec.RemoteProtectionGroupAttributes)
	actionResult.Error = err
	if res != nil {
		actionResult.PGStatus = res.GetStatus()
	}
	actionResult.Time = time.Now()
	if err != nil {
		log.Error(err, "failure encountered in executing action")
		if controllers.IsCSIFinalError(err) {
			log.Info("final error from driver: no more retries")
			actionResult.IsFinalError = true
		} else if rg.Status.LastAction.FirstFailure != nil {
			if time.Since(rg.Status.LastAction.FirstFailure.Time) > r.MaxRetryDurationForActions {
				actionResult.IsFinalError = true
				log.Info("final error: exceeded max retry duration, no more retries")
			}
		}
	}
	return &actionResult
}

// processRG - This function is only invoked when RG is in Ready/Error state
// If action is empty, finish Reconcile
// If action is not empty -
// If it is Invalid       - Reset it to empty, raise an event & finish the Reconcile
// If it is Valid         - Remove any old LastSuccessful annotation
//                        - Add an ActionInProgress annotation
//                        - Update the spec
//                        - Update status.State to <ACTION>_IN_PROGRESS
func (r *ReplicationGroupReconciler) processRG(ctx context.Context, dellCSIReplicationGroup *storagev1alpha1.DellCSIReplicationGroup, log logr.Logger) (ctrl.Result, error) {
	if dellCSIReplicationGroup.Spec.ProtectionGroupID != "" &&
		dellCSIReplicationGroup.Spec.Action != "" {
		// Get the action in progress from annotation
		inProgress, err := getActionInProgress(dellCSIReplicationGroup.Annotations)
		if err != nil {
			// We need to decide what to do here
			// Maybe best effort for what is set in the action field
			// CHANGE THIS
			return ctrl.Result{}, err
		}
		actionType := ActionType(dellCSIReplicationGroup.Spec.Action)
		_, err = r.getAction(actionType)
		// Invalid action type
		if err != nil {
			// Reset the action to empty & raise an event
			// Most importantly finish the reconcile
			log.Error(err, "invalid action type or not supported by driver")
			dellCSIReplicationGroup.Spec.Action = ""
			err1 := r.Update(ctx, dellCSIReplicationGroup)
			if err1 != nil {
				return ctrl.Result{}, err1
			}
			log.Info("Unsupported action was successfully reset to empty")
			r.EventRecorder.Eventf(dellCSIReplicationGroup, v1.EventTypeWarning,
				"Unsupported", "Cannot proceed with action %s. [%s]", actionType.String(), err.Error())
			log.Error(err, "can not proceed with reconcile!")
			return ctrl.Result{}, nil
		}
		// No annotation means we are getting the action call for the first time
		// Annotation with completed set to true means that the older action is complete
		// In both case, we set the Action annotation to "InProgress"
		if inProgress == nil || inProgress.Completed {
			// No annotations applied or old annotation
			actionAnnotation := ActionAnnotation{
				ActionName: actionType.String(),
				Completed:  false,
			}
			bytes, _ := json.Marshal(&actionAnnotation)
			controllers.AddAnnotation(dellCSIReplicationGroup, Action, string(bytes))
			log.Info("Updating", "annotation", string(bytes))
			err := r.Update(ctx, dellCSIReplicationGroup)
			return ctrl.Result{}, err
		}
		// Action is in progress but not completed yet
		// We just update the state to match

		// We need to reset the LastAction results if any
		lastAction := storagev1alpha1.LastAction{
			Condition:    "",
			FirstFailure: nil,
			Time:         nil,
			ErrorMessage: "",
		}
		dellCSIReplicationGroup.Status.LastAction = lastAction
		dellCSIReplicationGroup.Status.State = actionType.getInProgressState()
		log.Info("Updating", "state", actionType.getInProgressState())
		err = r.Status().Update(ctx, dellCSIReplicationGroup.DeepCopy())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ReplicationGroupReconciler) processRGInNoState(ctx context.Context, dellCSIReplicationGroup *storagev1alpha1.DellCSIReplicationGroup, log logr.Logger) (ctrl.Result, error) {
	ok, err := r.addFinalizer(ctx, dellCSIReplicationGroup.DeepCopy(), log)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ok {
		log.Info("Successfully added finalizer. Requesting a requeue")
		return ctrl.Result{Requeue: true}, nil
	}
	if err := r.updateState(ctx, dellCSIReplicationGroup.DeepCopy(), ReadyState, log); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("Successfully updated to", "state", ReadyState)
	return ctrl.Result{}, nil
}

func (r *ReplicationGroupReconciler) processRGForDeletion(ctx context.Context, dellCSIReplicationGroup *storagev1alpha1.DellCSIReplicationGroup, log logr.Logger) (ctrl.Result, error) {
	if dellCSIReplicationGroup.Spec.ProtectionGroupID != "" {
		// Delete the protection-group associated with this replication-group
		if err := r.deleteProtectionGroup(ctx, dellCSIReplicationGroup.DeepCopy(), log); err != nil {
			if !controllers.IsCSIFinalError(err) && dellCSIReplicationGroup.Status.State != DeletingState {
				if err := r.updateState(ctx, dellCSIReplicationGroup.DeepCopy(), DeletingState, log); err != nil {
					return ctrl.Result{}, err
				}
				log.Info("Successfully updated to", "state", DeletingState)
			}
			return ctrl.Result{}, controllers.IgnoreIfFinalError(err)
		}
	} else {
		if dellCSIReplicationGroup.Status.State != DeletingState {
			log.Info("Updating to", "state", DeletingState)
			err := r.updateState(ctx, dellCSIReplicationGroup.DeepCopy(), DeletingState, log)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	log.Info("Removing finalizers")
	err := r.removeFinalizer(ctx, dellCSIReplicationGroup.DeepCopy(), log)
	return ctrl.Result{}, err
}
