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

package csireplicator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dell/csm-replication/pkg/common/logger"
	v1 "k8s.io/api/core/v1"

	csiext "github.com/dell/dell-csi-extensions/replication"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	repv1 "github.com/dell/csm-replication/api/v1"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dell/csm-replication/controllers"
)

const (
	// ReadyState name of correct state of ReplicationGroup
	ReadyState = "Ready"
	// InvalidState name of invalid state of ReplicationGroup
	InvalidState = "Invalid"
	// ErrorState name of error state of ReplicationGroup
	ErrorState = "Error"
	// NoState empty state of ReplicationGroup
	NoState = ""
	// InProgress name of in progress state of ReplicationGroup
	InProgress = "IN_PROGRESS"
	// DeletingState name deletion state of ReplicationGroup
	DeletingState = "Deleting"
	// Action name of action field
	Action = "Action"
	// MaxRetryDurationForActions maximum amount of time between retries of failed action
	MaxRetryDurationForActions = 1 * time.Hour
	// MaxNumberOfConditions maximum length of conditions list
	MaxNumberOfConditions = 20
)

var getReplicationGroupRecouncilerUpdate = func(r *ReplicationGroupReconciler, ctx context.Context, obj client.Object) error {
	return r.Update(ctx, obj)
}

// ActionType represent replication action (FAILOVER, REPROTECT and etc.)
type ActionType string

func (a ActionType) String() string {
	return strings.ToUpper(string(a))
}

// Equals allows to check if provided string is equal to current action type
func (a ActionType) Equals(ctx context.Context, val string) bool {
	log := logger.GetLoggerFromContext(ctx)
	if strings.ToUpper(string(a)) == strings.ToUpper(val) {
		log.V(logger.DebugLevel).Info("Current action type is equal", "val", val, "a", string(a))
		return true
	}
	log.V(logger.DebugLevel).Info("Current action type is not equal", "val", val, "a", string(a))
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

// ActionResult represents end result of replication action
type ActionResult struct {
	ActionType       ActionType
	Time             time.Time
	Error            error
	IsFinalError     bool
	PGStatus         *csiext.StorageProtectionGroupStatus
	ActionAttributes map[string]string
}

// ActionAnnotation represents annotation that contains information about replication action
type ActionAnnotation struct {
	ActionName            string `json:"name"`
	Completed             bool   `json:"completed"`
	FinalError            string `json:"finalError"`
	FinishTime            string `json:"finishTime"`
	ProtectionGroupStatus string `json:"protectionGroupStatus"`
	SnapshotNamespace     string `json:"snapshotNamespace"`
	SnapshotClass         string `json:"snapshotClass"`
}

func updateRGSpecWithActionResult(ctx context.Context, rg *repv1.DellCSIReplicationGroup, result *ActionResult) bool {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Begin updating RG spec with", "Action Result", result)

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

		if ns, ok := rg.Annotations[controllers.SnapshotNamespace]; ok {
			actionAnnotation.SnapshotNamespace = ns
		} else {
			actionAnnotation.SnapshotNamespace = "default"
		}

		if snClass, ok := rg.Annotations[controllers.SnapshotClass]; ok {
			actionAnnotation.SnapshotClass = snClass
		}

		bytes, _ := json.Marshal(&actionAnnotation)
		controllers.AddAnnotation(rg, Action, string(bytes))

		log.V(logger.InfoLevel).Info("RG was successfully updated with", "Action Result", result)

		log.V(logger.InfoLevel).Info("Resetting the action processed time annotation.")
		// Indicates that the action needs to be processed by the controller.
		controllers.AddAnnotation(rg, controllers.ActionProcessedTime, "")

		isUpdated = true
		return isUpdated
	}

	log.V(logger.InfoLevel).Info("RG was not updated with", "Action Result", result)
	return isUpdated
}

func getActionResultFromActionAnnotation(ctx context.Context, actionAnnotation ActionAnnotation) (*ActionResult, error) {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Getting result from action annotation..")

	var finalErr error
	finalError := false
	if actionAnnotation.FinalError != "" {
		log.V(logger.InfoLevel).Info("There is final error", "actionAnnotation.FinalError", actionAnnotation.FinalError)
		finalErr = fmt.Errorf("%s", actionAnnotation.FinalError)
		finalError = true
	}

	var finishTime metav1.Time
	err := json.Unmarshal([]byte(actionAnnotation.FinishTime), &finishTime)
	if err != nil {
		log.Error(err, "Cannot unmarshall file")
		return nil, err
	}

	pgStatus := new(csiext.StorageProtectionGroupStatus)
	if actionAnnotation.ProtectionGroupStatus != "" {
		err = json.Unmarshal([]byte(actionAnnotation.ProtectionGroupStatus), pgStatus)
		if err != nil {
			log.Error(err, "Protection Group status error", "pgStatus", pgStatus)
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

func updateRGStatusWithActionResult(ctx context.Context, rg *repv1.DellCSIReplicationGroup, actionResult *ActionResult) error {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Begin updating RG status with action result")

	var result *ActionResult
	if actionResult == nil {
		actionAnnotation, err := getActionInProgress(ctx, rg.Annotations)
		if err != nil {
			return err
		}
		result, err = getActionResultFromActionAnnotation(ctx, *actionAnnotation)
		if err != nil {
			return err
		}
	} else {
		result = actionResult
	}
	if result.Error == nil {
		// Update to ReadyState
		log.V(logger.InfoLevel).Info("Update RG status to ReadyState")
		rg.Status.State = ReadyState
	} else {
		if result.IsFinalError {
			log.V(logger.InfoLevel).Info("Update RG status to ErrorState")
			rg.Status.State = ErrorState
		}
	}

	updateConditionsWithActionResult(ctx, rg, result)
	updateLastAction(ctx, rg, result)
	updateActionAttributes(ctx, rg, result)

	// Update the RG link state if we got a status
	if result.PGStatus != nil {
		log.V(logger.InfoLevel).Info("RG link state was updated")
		updateRGLinkState(rg, result.PGStatus.State.String(), result.PGStatus.IsSource, "")

		return nil
	}

	log.V(logger.InfoLevel).Info("RG link state was not updated. There is no PG status")

	return nil
}

func updateConditionsWithActionResult(ctx context.Context, rg *repv1.DellCSIReplicationGroup, result *ActionResult) {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Begin updating condition with action result")

	condition := repv1.LastAction{
		Condition:        result.ActionType.getSuccessfulString(),
		Time:             &metav1.Time{Time: result.Time},
		ActionAttributes: result.ActionAttributes,
	}
	if result.Error != nil && result.IsFinalError {
		condition.Condition = result.ActionType.getErrorString(result.Error.Error())
	}
	if result.Error == nil || (result.Error != nil && result.IsFinalError) {
		controllers.UpdateConditions(rg, condition, MaxNumberOfConditions)
	}

	log.V(logger.InfoLevel).Info("Condition was updated")
}

func updateLastAction(ctx context.Context, rg *repv1.DellCSIReplicationGroup, result *ActionResult) {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Updating last action..")

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

	log.V(logger.InfoLevel).Info("Last action was updated")
}

func updateActionAttributes(ctx context.Context, rg *repv1.DellCSIReplicationGroup, result *ActionResult) {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Updating the action attributes")

	switch result.ActionType {
	case ActionType(csiext.ActionTypes_CREATE_SNAPSHOT.String()):
		log.V(logger.InfoLevel).Info("Finished Create Snapshot, Attributes:")
		for key, val := range result.ActionAttributes {
			log.V(logger.InfoLevel).Info("Key: " + key + " Value: " + val)
		}

		rg.Status.LastAction.ActionAttributes = result.ActionAttributes
	default:
		log.V(logger.InfoLevel).Info("Update Action Attributes to default")
	}
}

// ReplicationGroupReconciler is a structure that watches and reconciles events on ReplicationGroup resources
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

// Reconcile contains reconciliation logic that updates ReplicationGroup depending on it's current state
func (r *ReplicationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("persistentvolumeclaim", req.NamespacedName)
	ctx = context.WithValue(ctx, logger.LoggerContextKey, log)

	rg := new(repv1.DellCSIReplicationGroup)
	err := r.Get(ctx, req.NamespacedName, rg)
	if err != nil {
		log.Error(err, "RG not found", "rg", rg)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	currentState := rg.Status.State

	// Handle deletion by checking for deletion timestamp
	if !rg.DeletionTimestamp.IsZero() && !strings.Contains(currentState, InProgress) {
		return r.processRGForDeletion(ctx, rg.DeepCopy())
	}

	// If no protection group ID set(the only mandatory field), we set it to Invalid state and return
	if rg.Spec.ProtectionGroupID == "" {
		r.EventRecorder.Event(rg, v1.EventTypeWarning, "Invalid", "Missing mandatory value - protectionGroupID")
		log.V(logger.InfoLevel).Info("Missing mandatory value - protectionGroupID", "rg", rg, "v1.EventTypeWarning", v1.EventTypeWarning)
		if rg.Status.State != InvalidState {
			if err := r.updateState(ctx, rg.DeepCopy(), InvalidState); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, fmt.Errorf("missing protection group id")
		}
		return ctrl.Result{}, nil
	}

	switch rg.Status.State {
	case NoState:
		return r.processRGInNoState(ctx, rg.DeepCopy())
	case InvalidState:
		// Update the state to no state
		err := r.updateState(ctx, rg.DeepCopy(), "")
		log.Error(err, "Empty state")
		return ctrl.Result{}, err
	case ErrorState:
		fallthrough
	case ReadyState:
		return r.processRG(ctx, rg.DeepCopy())
	default:
		if strings.Contains(rg.Status.State, InProgress) {
			return r.processRGInActionInProgressState(ctx, rg.DeepCopy())
		}
		return ctrl.Result{}, fmt.Errorf("unknown state")
	}
}

func (r *ReplicationGroupReconciler) getAction(actionType ActionType) (*csiext.ExecuteActionRequest_Action, error) {
	for _, supportedAction := range r.SupportedActions {
		if supportedAction == nil {
			continue
		}
		actionT := supportedAction.GetType()
		if actionT.String() == actionType.String() {
			action := &csiext.ExecuteActionRequest_Action{
				Action: &csiext.Action{},
			}
			action.Action.ActionTypes = actionT
			return action, nil
		}
	}
	return nil, fmt.Errorf("unsupported action")
}

func (r *ReplicationGroupReconciler) deleteProtectionGroup(ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Deleting protection-group")

	log.V(logger.DebugLevel).Info("Making CSI call to delete replication group")

	err := r.ReplicationClient.DeleteStorageProtectionGroup(ctx, rg.Spec.ProtectionGroupID, rg.Spec.ProtectionGroupAttributes)
	if err != nil {
		log.Error(err, "Failed to delete protection-group", "ProtectionGroupID", rg.Spec.ProtectionGroupID)
		return err
	}
	log.V(logger.InfoLevel).Info("Successfully deleted the protection-group")
	return nil
}

func (r *ReplicationGroupReconciler) removeFinalizer(ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Removing finalizer")

	// Remove replication-protection finalizer
	if ok := controllers.RemoveFinalizerIfExists(rg, controllers.ReplicationFinalizer); ok {
		// Adding annotation to mark the removal of protection-group
		controllers.AddAnnotation(rg, controllers.ProtectionGroupRemovedAnnotation, "yes")
		if err := r.Update(ctx, rg); err != nil {
			log.Error(err, "Failed to remove finalizer", "rg", rg, "ProtectionGroupRemovedAnnotation", controllers.ProtectionGroupRemovedAnnotation)
			return err
		}
		log.V(logger.InfoLevel).Info("Finalizer removed successfully")
	}
	return nil
}

func (r *ReplicationGroupReconciler) addFinalizer(ctx context.Context, rg *repv1.DellCSIReplicationGroup) (bool, error) {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Adding finalizer")

	ok := controllers.AddFinalizerIfNotExist(rg, controllers.ReplicationFinalizer)
	if ok {
		if err := getReplicationGroupRecouncilerUpdate(r, ctx, rg); err != nil {
			log.Error(err, "Failed to add finalizer", "rg", rg)
			return ok, err
		}
		log.V(logger.DebugLevel).Info("Successfully add finalizer. Requesting a requeue")
	}
	return ok, nil
}

func (r *ReplicationGroupReconciler) updateState(ctx context.Context, rg *repv1.DellCSIReplicationGroup, state string) error {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Updating to", "state", state)

	rg.Status.State = state
	if err := r.Status().Update(ctx, rg); err != nil {
		log.Error(err, "Failed updating to", "state", state)
		return err
	}
	log.V(logger.InfoLevel).Info("Successfully updated to", "state", state)
	return nil
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *ReplicationGroupReconciler) SetupWithManager(mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
	if r.MaxRetryDurationForActions == 0 {
		r.MaxRetryDurationForActions = MaxRetryDurationForActions
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&repv1.DellCSIReplicationGroup{}).
		WithOptions(reconciler.Options{
			RateLimiter:             limiter,
			MaxConcurrentReconciles: maxReconcilers,
		}).
		Complete(r)
}

func getActionInProgress(ctx context.Context, annotations map[string]string) (*ActionAnnotation, error) {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.DebugLevel).Info("Getting the action in progress from annotation")

	val, ok := annotations[Action]
	if !ok {
		log.V(logger.InfoLevel).Info("No action", "val", val)
		return nil, nil
	}
	var actionAnnotation ActionAnnotation
	err := json.Unmarshal([]byte(val), &actionAnnotation)
	if err != nil {
		log.Error(err, "JSON unmarshal error", "actionAnnotation", actionAnnotation)
		return nil, err
	}
	log.V(logger.InfoLevel).Info("Action was got", "actionAnnotation", actionAnnotation)
	return &actionAnnotation, nil
}

func resetRGSpecForInvalidAction(rg *repv1.DellCSIReplicationGroup) {
	rg.Spec.Action = ""
	delete(rg.Annotations, Action)
}

func (r *ReplicationGroupReconciler) processRGInActionInProgressState(ctx context.Context,
	rg *repv1.DellCSIReplicationGroup,
) (ctrl.Result, error) {
	log := logger.GetLoggerFromContext(ctx)
	// Get action in progress from annotation
	inProgress, err := getActionInProgress(ctx, rg.Annotations)
	if err != nil || inProgress == nil {
		// Either the annotation is not set or not set properly
		// Mostly points to User error
		if rg.Spec.Action != "" {

			log.V(logger.DebugLevel).Info("Action set")

			actionType := ActionType(rg.Spec.Action)
			_, err := r.getAction(actionType)
			if err != nil {
				// Action invalid
				r.EventRecorder.Event(rg, v1.EventTypeWarning, "Invalid",
					"State is InProgress & Invalid action set in spec")
				log.V(logger.InfoLevel).Info("Warning: State is InProgress but invalid action set in spec")
				log.V(logger.InfoLevel).Info("Resetting the CR state to Ready")
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

			log.V(logger.DebugLevel).Info("Action not set")

			// Nothing to do here
			// What should be the final state?
			r.EventRecorder.Event(rg, v1.EventTypeWarning, "Invalid",
				"State is InProgress but action & action annotation not set")
			log.V(logger.InfoLevel).Info("Warning: State is InProgress but action & action annotation not set")
			log.V(logger.InfoLevel).Info("Resetting the CR state to Ready")
			// Delete any incorrect annotation
			resetRGSpecForInvalidAction(rg)
			if err := r.Update(ctx, rg); err != nil {
				return ctrl.Result{}, err
			}
			err = r.updateState(ctx, rg, ReadyState)
			return ctrl.Result{}, err
		}
	}
	if inProgress.Completed {
		// If the action field is still set, then reset it
		if rg.Spec.Action != "" {
			rg.Spec.Action = ""
			if err := r.Update(ctx, rg); err != nil {
				log.Error(err, "Failed to reset action field", "rg.Spec.Action", rg.Spec.Action)
				return ctrl.Result{}, err
			}
		}
		// Update status
		if err := updateRGStatusWithActionResult(ctx, rg, nil); err == nil {
			log.Error(err, "Failed to update status", "rg", rg)
			err1 := r.Status().Update(ctx, rg.DeepCopy())
			return ctrl.Result{}, err1
		}
	}
	actionType := ActionType(inProgress.ActionName)
	if !actionType.Equals(ctx, rg.Spec.Action) {
		r.EventRecorder.Eventf(rg, v1.EventTypeWarning, "InProgress",
			"Action [%s] on DellCSIReplicationGroup [%s] is in Progress, cannot execute [%s] ", actionType.String(), rg.Name, rg.Spec.Action)
	}
	action, err := r.getAction(actionType)
	if err != nil {
		r.EventRecorder.Eventf(rg, v1.EventTypeWarning, "Unsupported",
			"Action changed to an invalid value %s while another action execution was in progress. [%s]",
			actionType.String(), err.Error())
		log.Error(err, "Can not proceed!", "actionType", actionType)
		err = r.updateState(ctx, rg.DeepCopy(), ErrorState)
		return ctrl.Result{}, err
	}
	// Make API call to Execute Action
	actionResult := r.executeAction(ctx, rg.DeepCopy(), actionType, action)
	if actionResult.Error != nil {
		// Raise event in case of an error
		r.EventRecorder.Eventf(rg, v1.EventTypeWarning, "Error",
			"Action [%s] on DellCSIReplicationGroup [%s] failed with error [%s] ",
			actionType.String(), rg.Name, actionResult.Error.Error())
	}
	// Update spec
	isSpecUpdated := updateRGSpecWithActionResult(ctx, rg, actionResult)
	if isSpecUpdated {
		if err := r.Update(ctx, rg); err != nil {
			log.Error(err, "Failed to update spec", "rg", rg, "Action Result", actionResult)
			return ctrl.Result{}, err
		}
		log.V(logger.InfoLevel).Info("Successfully updated spec", "Action Result", actionResult)
	}
	// Update status
	err = updateRGStatusWithActionResult(ctx, rg, actionResult)
	if err != nil {
		r.Log.Error(err, "Failed to update status with action result", "Action Result", actionResult)
		return ctrl.Result{}, err
	}

	err = r.Status().Update(ctx, rg)
	if err != nil {
		r.Log.Error(err, "Failed to update status")
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

func (r *ReplicationGroupReconciler) executeAction(ctx context.Context, rg *repv1.DellCSIReplicationGroup,
	actionType ActionType, action *csiext.ExecuteActionRequest_Action,
) *ActionResult {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Executing action", "actionType", actionType)

	actionResult := ActionResult{
		ActionType: actionType,
		PGStatus:   nil,
	}

	log.V(logger.DebugLevel).Info("Making API call to Execute Action", "actionType", actionType)

	res, err := r.ReplicationClient.ExecuteAction(ctx, rg.Spec.ProtectionGroupID, action,
		rg.Spec.ProtectionGroupAttributes, rg.Spec.RemoteProtectionGroupID, rg.Spec.RemoteProtectionGroupAttributes)
	actionResult.Error = err
	if res != nil {
		actionResult.PGStatus = res.GetStatus()
		actionResult.ActionAttributes = res.ActionAttributes

	}
	actionResult.Time = time.Now()
	if err != nil {
		log.Error(err, "Failure encountered in executing action", "action", action)
		if controllers.IsCSIFinalError(err) {
			log.V(logger.InfoLevel).Info("Final error from driver: no more retries")
			actionResult.IsFinalError = true
		} else if rg.Status.LastAction.FirstFailure != nil {
			if time.Since(rg.Status.LastAction.FirstFailure.Time) > r.MaxRetryDurationForActions {
				actionResult.IsFinalError = true
				log.V(logger.InfoLevel).Info("Final error: exceeded max retry duration, no more retries")
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
//   - Add an ActionInProgress annotation
//   - Update the spec
//   - Update status.State to <ACTION>_IN_PROGRESS
func (r *ReplicationGroupReconciler) processRG(ctx context.Context, dellCSIReplicationGroup *repv1.DellCSIReplicationGroup) (ctrl.Result, error) {
	log := logger.GetLoggerFromContext(ctx)
	log.V(logger.InfoLevel).Info("Start process RG")

	if dellCSIReplicationGroup.Spec.ProtectionGroupID != "" &&
		dellCSIReplicationGroup.Spec.Action != "" {
		// Get action in progress from annotation
		inProgress, err := getActionInProgress(ctx, dellCSIReplicationGroup.Annotations)
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
			log.Error(err, "Invalid action type or not supported by driver", "actionType", actionType)
			dellCSIReplicationGroup.Spec.Action = ""
			err1 := r.Update(ctx, dellCSIReplicationGroup)
			if err1 != nil {
				log.Error(err, "Failed to update", "dellCSIReplicationGroup", dellCSIReplicationGroup)
				return ctrl.Result{}, err1
			}
			log.V(logger.InfoLevel).Info("Unsupported action was successfully reset to empty")
			r.EventRecorder.Eventf(dellCSIReplicationGroup, v1.EventTypeWarning,
				"Unsupported", "Cannot proceed with action %s. [%s]", actionType.String(), err.Error())
			log.Error(err, "Can not proceed with reconcile!", "actionType", actionType)
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
			log.V(logger.InfoLevel).Info("Updating", "annotation", string(bytes))
			err := r.Update(ctx, dellCSIReplicationGroup)
			log.Error(err, "Failed to update", "annotation", string(bytes))
			return ctrl.Result{}, err
		}
		// Action is in progress but not completed yet
		// We just update the state to match

		// We need to reset the LastAction results if any
		lastAction := repv1.LastAction{
			Condition:    "",
			FirstFailure: nil,
			Time:         nil,
			ErrorMessage: "",
		}
		dellCSIReplicationGroup.Status.LastAction = lastAction
		dellCSIReplicationGroup.Status.State = actionType.getInProgressState()
		log.V(logger.InfoLevel).Info("Updating", "state", actionType.getInProgressState())
		err = r.Status().Update(ctx, dellCSIReplicationGroup.DeepCopy())
		log.Error(err, "Failed to update", "state", actionType.getInProgressState())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ReplicationGroupReconciler) processRGInNoState(ctx context.Context, dellCSIReplicationGroup *repv1.DellCSIReplicationGroup) (ctrl.Result, error) {
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

func (r *ReplicationGroupReconciler) processRGForDeletion(ctx context.Context, dellCSIReplicationGroup *repv1.DellCSIReplicationGroup) (ctrl.Result, error) {
	log := logger.GetLoggerFromContext(ctx)

	if dellCSIReplicationGroup.Spec.ProtectionGroupID != "" {
		log.V(logger.DebugLevel).Info("Deleting the protection-group associated with this replication-group")
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
