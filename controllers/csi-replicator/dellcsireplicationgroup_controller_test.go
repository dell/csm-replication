/*
 Copyright © 2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	csiext "github.com/dell/dell-csi-extensions/replication"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	csiidentity "github.com/dell/csm-replication/test/mocks"
	csireplication "github.com/dell/csm-replication/test/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	protectionGroupID = "l-group-id-1"
)

type RGControllerTestSuite struct {
	suite.Suite
	client      client.Client
	driver      utils.Driver
	repClient   *csireplication.MockReplication
	rgReconcile *ReplicationGroupReconciler
}

func (suite *RGControllerTestSuite) SetupSuite() {
	// do nothing
}

func (suite *RGControllerTestSuite) SetupTest() {
	suite.Init()
	suite.initReconciler()
}

func (suite *RGControllerTestSuite) Init() {
	suite.driver = utils.GetDefaultDriver()
	suite.client = utils.GetFakeClient()
	ctx := context.Background()
	scObj := utils.GetReplicationEnabledSC(suite.driver.DriverName, suite.driver.StorageClass,
		suite.driver.RemoteSCName, suite.driver.RemoteClusterID)
	err := suite.client.Create(ctx, scObj)
	suite.Nil(err)
	repClient := csireplication.NewFakeReplicationClient(utils.ContextPrefix)
	suite.repClient = &repClient
}

func (suite *RGControllerTestSuite) initReconciler() {
	logger := ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup")
	// Get capabilities
	identity := csiidentity.NewFakeIdentityClient(suite.driver.DriverName)
	_, actions, err := identity.GetReplicationCapabilities(context.Background())
	if err != nil {
		logger.Error(err, "failed to get CSI driver capabilities")
		os.Exit(1)
	}
	fakeRecorder := record.NewFakeRecorder(100)
	// Initialize the annotations & labels
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)

	rgReconcile := &ReplicationGroupReconciler{
		Client:                     suite.client,
		Log:                        logger,
		Scheme:                     utils.Scheme,
		DriverName:                 suite.driver.DriverName,
		EventRecorder:              fakeRecorder,
		ReplicationClient:          suite.repClient,
		SupportedActions:           actions,
		MaxRetryDurationForActions: MaxRetryDurationForActions,
	}
	suite.rgReconcile = rgReconcile
}

func TestRGControllerTestSuite(t *testing.T) {
	testSuite := new(RGControllerTestSuite)
	suite.Run(t, testSuite)
	// testSuite.TearDownTestSuite()
}

func (suite *RGControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}

func (suite *RGControllerTestSuite) getLocalRG(name string) repv1.DellCSIReplicationGroup {
	// creating fake replication group
	if name == "" {
		name = suite.driver.RGName
	}
	replicationGroup := utils.GetRGObj(name, suite.driver.DriverName, suite.driver.RemoteClusterID,
		utils.LocalPGID, utils.RemotePGID, suite.getLocalParams(), suite.getRemoteParams())
	return *replicationGroup
}

func (suite *RGControllerTestSuite) getRemoteRG(name string) repv1.DellCSIReplicationGroup {
	// creating fake replication group
	if name == "" {
		name = suite.driver.RGName
	}
	replicationGroup := utils.GetRGObj(name, suite.driver.DriverName, suite.driver.SourceClusterID,
		utils.RemotePGID, utils.LocalPGID, suite.getRemoteParams(), suite.getLocalParams())
	return *replicationGroup
}

func (suite *RGControllerTestSuite) createRGInActionInProgressState(name string, actionName string,
	completed bool, deleteInProgress bool,
) *repv1.DellCSIReplicationGroup {
	rg := suite.getLocalRG(name)
	actionType := ActionType(actionName)
	rg.Spec.Action = actionName
	actionAnnotation := ActionAnnotation{
		ActionName: actionType.String(),
		Completed:  completed,
	}
	bytes, _ := json.Marshal(&actionAnnotation)
	rg.ObjectMeta.Annotations = make(map[string]string)
	rg.ObjectMeta.Annotations[Action] = string(bytes)
	rg.Status.State = actionType.getInProgressState()
	rg.Status.ReplicationLinkState.IsSource = true
	rg.Status.ReplicationLinkState.State = "Synchronized"
	if deleteInProgress {
		rg.DeletionTimestamp = &metav1.Time{
			Time: time.Now(),
		}
		rg.Finalizers = append(rg.Finalizers, controllers.ReplicationFinalizer)
	}

	suite.client = utils.GetFakeClientWithObjects(&rg)
	suite.rgReconcile.Client = suite.client

	return &rg
}

func (suite *RGControllerTestSuite) getLocalParams() map[string]string {
	return utils.GetParams(suite.driver.RemoteClusterID, suite.driver.RemoteSCName)
}

func (suite *RGControllerTestSuite) getRemoteParams() map[string]string {
	return utils.GetParams(suite.driver.SourceClusterID, suite.driver.StorageClass)
}

func (suite *RGControllerTestSuite) getTypicalReconcileRequest(name string) reconcile.Request {
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "",
			Name:      name,
		},
	}
	return req
}

func areAnnotationsEqual(expected, got ActionAnnotation) bool {
	if expected.Completed == got.Completed {
		if expected.FinalError == got.FinalError {
			if expected.ActionName == got.ActionName {
				return true
			}
		}
	}
	return false
}

func (suite *RGControllerTestSuite) TestReconcileWithNoObject() {
	// scenario: Reconcile for an object which doesn't exist (has been deleted)
	req := suite.getTypicalReconcileRequest(suite.driver.RGName)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
}

func (suite *RGControllerTestSuite) TestReconcileWithNoState() {
	// scenario: When there is no state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	resp, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	suite.Equal(true, resp.Requeue, "Requeue is set to true")

	// Reconcile again
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err, "No error on RG get")
	suite.Equal(ReadyState, rg.Status.State, "State should be Ready")
	suite.Contains(rg.Finalizers, controllers.ReplicationFinalizer, "Finalizer should be present")
}

func (suite *RGControllerTestSuite) TestReconcileWithEmptyPGID() {
	// scenario: When there is no protection group ID and no valid state is set
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")

	// unset the protection group
	replicationGroup.Spec.ProtectionGroupID = ""
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, "missing protection group id")

	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err, "No error on RG get")
	suite.Equal(InvalidState, rg.Status.State, "State is set to invalid")
}

func (suite *RGControllerTestSuite) TestDeleteWithInvalidState() {
	// scenario: When there is valid deletion time-stamp and invalid state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = InvalidState
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	res, err := suite.rgReconcile.Reconcile(ctx, req)
	suite.NoError(err, "No error on RG reconcile")
	suite.Equal(res.Requeue, false, "Requeue should be set to false")
}

func (suite *RGControllerTestSuite) TestReconcileWithInvalidStateRepeated() {
	// scenario: When there is invalid state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.Spec.ProtectionGroupID = ""
	replicationGroup.Status.State = InvalidState
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal(InvalidState, rg.Status.State, "State is still Invalid")
}

func (suite *RGControllerTestSuite) TestReconcileWithInvalidStateAfterFix() {
	// scenario: When there is invalid state
	// and PGID has been added back
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.Status.State = InvalidState
	replicationGroup.Spec.ProtectionGroupID = protectionGroupID
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal("", rg.Status.State, "State changed to empty")
}

func (suite *RGControllerTestSuite) TestDeleteWithInvalidStateAndNoPGID() {
	// scenario: When there is valid deletion time-stamp and invalid state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = InvalidState
	replicationGroup.Spec.ProtectionGroupID = ""
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	res, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	suite.Equal(res.Requeue, false, "Requeue should be set to false")

	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	// State should change to deleting
	suite.Equal(DeletingState, rg.Status.State, "State changed to Deleting")
}

func (suite *RGControllerTestSuite) TestDeleteWithNoPGID() {
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Spec.ProtectionGroupID = ""
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on reconcile")

	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err, "no error on RG get")
	suite.Equal(DeletingState, rg.Status.State)

	// Reconcile a few times
	for i := 0; i < 5; i++ {
		resp, err := suite.rgReconcile.Reconcile(context.Background(), req)
		suite.NoError(err, "no error on reconcile")
		suite.Equal(false, resp.Requeue, "No requeue set")
		suite.Equal(DeletingState, rg.Status.State, "State is still deleting")
	}
}

func (suite *RGControllerTestSuite) TestDeleteWithNoState() {
	// scenario: When there is valid deletion time-stamp and no state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	res, err := suite.rgReconcile.Reconcile(ctx, req)
	suite.NoError(err, "No error on RG reconcile")
	suite.Equal(res.Requeue, false, "Requeue should be set to false")
}

func (suite *RGControllerTestSuite) TestDeleteWithReadyState() {
	// scenario: When there is valid deletion time-stamp and Ready state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	// Set Finalizer
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = ReadyState
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.Error(err, "RG object not found")
}

func (suite *RGControllerTestSuite) TestDeleteWithErrors() {
	// scenario: When there is valid deletion time-stamp and Ready state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")

	// Set Finalizer
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = ReadyState
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject temporary error
	errorMsg := fmt.Sprintf("error during delete")
	suite.repClient.InjectErrorClearAfterN(fmt.Errorf("%s", errorMsg), 5)

	for i := 0; i < 5; i++ {
		_, err := suite.rgReconcile.Reconcile(context.Background(), req)
		suite.EqualError(err, errorMsg, "error should match injected error")
	}

	rg := new(repv1.DellCSIReplicationGroup)
	err := suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal(DeletingState, rg.Status.State, "State should be set to Deleting")

	// Reconcile again and this time object should be deleted
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.Error(err, "RG object not found")
}

func (suite *RGControllerTestSuite) TestDeleteWithPersistentError() {
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	// Set Finalizer
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = ReadyState

	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject error which doesn't clear
	errorMsg := fmt.Sprintf("error during delete")
	suite.repClient.InjectError(fmt.Errorf("%s", errorMsg))
	defer suite.repClient.ClearErrorAndCondition(true)

	// Reconcile a few times
	for i := 0; i < 20; i++ {
		_, err := suite.rgReconcile.Reconcile(context.Background(), req)
		suite.EqualError(err, errorMsg, "error should match injected error")
	}

	rg := new(repv1.DellCSIReplicationGroup)
	err := suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal(DeletingState, rg.Status.State, "State should be set to Deleting")
}

func (suite *RGControllerTestSuite) TestDeleteWithFinalError() {
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	// Set Finalizer
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = ReadyState

	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject temporary error
	errorMsg := fmt.Sprintf("error during delete")
	suite.repClient.InjectErrorAutoClear(fmt.Errorf("%s", errorMsg))
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, errorMsg, "error should match injected error")

	// Inject another error which is final
	errorMsg = "failed-pre-condition"
	suite.repClient.InjectErrorAutoClear(status.Error(codes.FailedPrecondition, errorMsg))
	resp, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "no error after a final error")
	suite.Equal(false, resp.Requeue, "Requeue should not be true")

	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal(DeletingState, rg.Status.State, "State should be set to Deleting")

	// Reconcile again without any errors
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)
	// Object should be deleted now
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.Error(err, "RG object not found")
}

func (suite *RGControllerTestSuite) TestDeleteWithActionInProgress() {
	ctx := context.Background()
	// scenario: When there is valid PG-ID and state is in ActionInProgress
	actionName := "Resume"
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, true)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	rg := new(repv1.DellCSIReplicationGroup)
	err := suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)

	// Reconcile
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	var latestRG repv1.DellCSIReplicationGroup
	err = suite.client.Get(context.Background(), req.NamespacedName, &latestRG)
	suite.NoError(err, "No error on RG Get")
	// Verify if status is changed to "Ready"
	suite.Equal(ReadyState, latestRG.Status.State, "State is Ready")
	expectedAnnotation := ActionAnnotation{
		ActionName: ActionType(actionName).String(),
		Completed:  true,
		FinalError: "",
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(latestRG.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal("", latestRG.Spec.Action, "Action field set to empty")

	// Reconcile again
	// this time the object should be deleted
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.Error(err, "RG not found")
}

func (suite *RGControllerTestSuite) prettyPrint(rg repv1.DellCSIReplicationGroup) {
	suite.T().Log("RG Name", rg.Name)
	suite.T().Log("RG Annotations", rg.GetAnnotations())
	suite.T().Log("RG Labels", rg.GetLabels())
	suite.T().Log("RG Spec", rg.Spec)
	suite.T().Log("RG Status", rg.Status)
}

func (suite *RGControllerTestSuite) TestActionInReadyState() {
	// scenario: When there is valid PG-ID and Suspend action
	actionName := "Suspend"
	actionType := ActionType(actionName)

	replicationGroup := suite.getLocalRG("")
	replicationGroup.Status.State = "Ready"
	replicationGroup.Spec.Action = actionName

	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Reconcile twice
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if status is changed to "In progress"
	suite.Equal(ActionType(actionName).getInProgressState(), rg.Status.State,
		"State is in progress")
	expectedAnnotation := ActionAnnotation{
		ActionName: actionType.String(),
		Completed:  false,
		FinalError: "",
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
}

func (suite *RGControllerTestSuite) TestActionInProgressState() {
	// scenario: When there is valid PG-ID and state is in ActionInProgress
	actionName := "Failover_Local"
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	// Inject condition
	suite.repClient.SetCondition(csireplication.ExecuteActionWithSwap)
	// Reconcile
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if status is changed to "Ready"
	suite.Equal(ReadyState, rg.Status.State, "State is Ready")
	expectedAnnotation := ActionAnnotation{
		ActionName: ActionType(actionName).String(),
		Completed:  true,
		FinalError: "",
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Spec.Action, "", "Action field set to empty")
	// Check if condition was updated
	msg := fmt.Sprintf("Replication Link State:IsSource changed")
	suite.Equal(true, conditionPresent(rg.Status.Conditions, msg))
}

func (suite *RGControllerTestSuite) TestActionInProgressStateWithSingleError() {
	// scenario: When there is valid PG-ID and state is in ActionInProgress
	// and driver returns a temporary error
	actionName := "Resume"
	actionType := ActionType(actionName)
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error
	errorMsg := "failed to perform action"
	suite.repClient.InjectErrorAutoClear(fmt.Errorf("%s", errorMsg))
	// Reconcile
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, errorMsg)

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	expectedAnnotation := ActionAnnotation{
		ActionName: actionType.String(),
		Completed:  false,
		FinalError: "",
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Status.State, actionType.getInProgressState(), "State is in progress")
	// Last action set
	suite.Contains(rg.Status.LastAction.Condition, actionType.String(), "Last Action was updated")
	suite.Equal(errorMsg, rg.Status.LastAction.ErrorMessage, "expected error")
	// Also verify if FirstFailure was set
	suite.NotNil(rg.Status.LastAction.FirstFailure)

	// Reconcile again after the error has been cleared
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	expectedAnnotation.Completed = true
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	// Check if condition was updated
	suite.Equal(true, conditionPresent(rg.Status.Conditions, actionType.getSuccessfulString()))
}

func (suite *RGControllerTestSuite) TestActionInProgressStateWithContextDeadlineError() {
	// scenario: When there is valid PG-ID and state is in ActionInProgress
	// and driver returns a temporary error
	actionName := "Resume"
	actionType := ActionType(actionName)
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error
	deadlineExceededError := status.Error(codes.DeadlineExceeded, "context deadline exceeded")
	suite.repClient.InjectErrorAutoClear(deadlineExceededError)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	// Reconcile
	_, err := suite.rgReconcile.Reconcile(ctx, req)
	suite.EqualError(err, deadlineExceededError.Error())

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)

	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	expectedAnnotation := ActionAnnotation{
		ActionName: actionType.String(),
		Completed:  false,
		FinalError: "",
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Status.State, actionType.getInProgressState(), "State is in progress")
	// Last action set
	suite.Contains(rg.Status.LastAction.Condition, actionType.String(), "Last Action was updated")
	suite.Equal(deadlineExceededError.Error(), rg.Status.LastAction.ErrorMessage, "expected error")
	// Also verify if FirstFailure was set
	suite.NotNil(rg.Status.LastAction.FirstFailure)

	// Reconcile again after the error has been cleared
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	expectedAnnotation.Completed = true
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	// Check if condition was updated
	suite.Equal(true, conditionPresent(rg.Status.Conditions, actionType.getSuccessfulString()))
}

func (suite *RGControllerTestSuite) TestActionInProgressWithFinalError() {
	// scenario: When there is valid PG-ID and state is in Error
	// and driver returns a temporary error
	actionName := "Resume"
	actionType := ActionType(actionName)
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error
	errorMsg := "failed to perform action"
	failedPreconditionError := status.Error(codes.FailedPrecondition, errorMsg)
	suite.repClient.InjectErrorAutoClear(failedPreconditionError)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)

	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	expectedAnnotation := ActionAnnotation{
		ActionName: actionType.String(),
		Completed:  true,
		FinalError: failedPreconditionError.Error(),
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Status.State, ErrorState, "State is in error")
	// Last action set
	suite.Contains(rg.Status.LastAction.Condition, actionType.String(), "Last Action was updated")
	suite.Equal(failedPreconditionError.Error(), rg.Status.LastAction.ErrorMessage, "expected error")
	// Also verify if FirstFailure was set
	suite.NotNil(rg.Status.LastAction.FirstFailure)
}

func (suite *RGControllerTestSuite) TestActionInProgressStateWithPersistentError() {
	// scenario: When there is valid PG-ID and state is in ActionInProgress
	// and driver returns errors
	actionName := "Failback_Local"
	actionType := ActionType(actionName)
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	rg := new(repv1.DellCSIReplicationGroup)

	errorMsg := "can't process request"
	suite.repClient.InjectErrorClearAfterN(fmt.Errorf("%s", errorMsg), 10)
	for i := 0; i < 10; i++ {
		_, err := suite.rgReconcile.Reconcile(context.Background(), req)
		suite.EqualError(err, errorMsg)
		// Get the updated RG
		err = suite.client.Get(context.Background(), req.NamespacedName, rg)
		suite.NoError(err, "No error on RG Get")
		suite.Equal(actionName, rg.Spec.Action, "Action field still set")
		suite.Equal(actionType.getInProgressState(), rg.Status.State, "State is in progress")
	}

	// Final reconcile
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on reconcile")
	// Refresh RG
	var latestRG repv1.DellCSIReplicationGroup
	err = suite.client.Get(context.TODO(), req.NamespacedName, &latestRG)
	suite.NoError(err, "No error on RG Get")

	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(latestRG.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	expectedAnnotation := ActionAnnotation{
		ActionName: actionType.String(),
		Completed:  true,
		FinalError: "",
	}
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal("", latestRG.Spec.Action, "Action field set to empty")
	suite.Equal(ReadyState, latestRG.Status.State, "Action field set to empty")
	// Check if condition was updated
	suite.Equal(true, conditionPresent(latestRG.Status.Conditions, actionType.getSuccessfulString()))
}

func (suite *RGControllerTestSuite) TestActionInProgressStateWithTimeout() {
	// scenario: When there is valid PG-ID and state is in ActionInProgress
	// and driver returns errors
	actionName := "Failover_Remote"
	actionType := ActionType(actionName)
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error which doesn't automatically clear
	errorMsg := "failed to perform action"
	suite.repClient.InjectError(fmt.Errorf("%s", errorMsg))
	defer suite.repClient.ClearErrorAndCondition(true)
	suite.rgReconcile.MaxRetryDurationForActions = 3 * time.Second
	defer func() {
		suite.rgReconcile.MaxRetryDurationForActions = MaxRetryDurationForActions
	}()
	// Reconcile in a loop
	for i := 0; i < 4; i++ {
		_, err := suite.rgReconcile.Reconcile(context.Background(), req)
		// Get the updated RG
		rg := new(repv1.DellCSIReplicationGroup)
		err = suite.client.Get(context.Background(), req.NamespacedName, rg)
		suite.NoError(err, "No error on RG Get")
		var gotAnnotation ActionAnnotation
		err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
		suite.NoError(err, "No error on JSON unmarshal of action annotation")
		if gotAnnotation.Completed && gotAnnotation.FinalError == errorMsg {
			suite.Equal(rg.Status.State, ErrorState)
			suite.Equal(fmt.Sprintf("Action %s failed with error %s", actionType.String(),
				errorMsg), rg.Status.Conditions[0].Condition)
			suite.Equal(fmt.Sprintf("Action %s failed with error %s", actionType.String(),
				errorMsg), rg.Status.LastAction.Condition)
			break
		}
		time.Sleep(1 * time.Second)
	}
}

func (suite *RGControllerTestSuite) TestActionInProgressWithSuccessfulAction() {
	actionName := "Failback_Local"
	replicationGroup := suite.getLocalRG(suite.driver.RGName)
	actionType := ActionType(actionName)
	replicationGroup.Spec.Action = actionName
	replicationGroup.Status.State = actionType.getInProgressState()
	finishTime := &metav1.Time{
		Time: time.Now(),
	}
	buff, _ := json.Marshal(finishTime)
	pgStatus := csiext.StorageProtectionGroupStatus{
		State:    csiext.StorageProtectionGroupStatus_SYNCHRONIZED,
		IsSource: true,
	}
	protectionGroupStatus, _ := json.Marshal(&pgStatus)
	actionAnnotation := ActionAnnotation{
		ActionName:            actionType.String(),
		Completed:             true,
		FinalError:            "",
		FinishTime:            string(buff),
		ProtectionGroupStatus: string(protectionGroupStatus),
	}
	bytes, _ := json.Marshal(&actionAnnotation)
	replicationGroup.ObjectMeta.Annotations = make(map[string]string)
	replicationGroup.ObjectMeta.Annotations[Action] = string(bytes)
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if state is changed to "Ready"
	suite.Equal(ReadyState, rg.Status.State, "State is Ready")
	expectedAnnotation := ActionAnnotation{
		ActionName: ActionType(actionName).String(),
		Completed:  true,
		FinalError: "",
	}

	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Spec.Action, "", "Action field set to empty")
	// Check if condition was updated
	suite.Equal(true, conditionPresent(rg.Status.Conditions, actionType.getSuccessfulString()))
}

func conditionPresent(conditions []repv1.LastAction, msg string) bool {
	found := false
	for _, condition := range conditions {
		if strings.Contains(condition.Condition, msg) {
			found = true
			break
		}
	}
	return found
}

func (suite *RGControllerTestSuite) TestActionInProgressWithFailedAction() {
	actionName := "Failback_Local"
	replicationGroup := suite.getLocalRG(suite.driver.RGName)
	actionType := ActionType(actionName)
	replicationGroup.Spec.Action = actionName
	replicationGroup.Status.State = actionType.getInProgressState()
	finishTime := &metav1.Time{
		Time: time.Now(),
	}
	buff, _ := json.Marshal(finishTime)
	errorMsg := "failed to execute action"
	actionAnnotation := ActionAnnotation{
		ActionName:            actionType.String(),
		Completed:             true,
		FinalError:            errorMsg,
		FinishTime:            string(buff),
		ProtectionGroupStatus: "",
	}
	bytes, _ := json.Marshal(&actionAnnotation)
	replicationGroup.ObjectMeta.Annotations = make(map[string]string)
	replicationGroup.ObjectMeta.Annotations[Action] = string(bytes)
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if state is changed to "Error"
	suite.Equal(ErrorState, rg.Status.State, "State is Error")
	suite.Equal(rg.Spec.Action, "", "Action field set to empty")
	// Check if condition was updated
	suite.Equal(fmt.Sprintf("Action %s failed with error %s", actionType.String(),
		errorMsg), rg.Status.Conditions[0].Condition)
}

func (suite *RGControllerTestSuite) TestActionInProgressWithMissingAnnotation() {
	actionName := "Failback_Local"
	replicationGroup := suite.getLocalRG(suite.driver.RGName)
	actionType := ActionType(actionName)
	replicationGroup.Spec.Action = actionName
	replicationGroup.Status.State = actionType.getInProgressState()
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if state is changed to "Ready"
	suite.Equal(ReadyState, rg.Status.State, "State is Ready")
	expectedAnnotation := ActionAnnotation{
		ActionName: ActionType(actionName).String(),
		Completed:  true,
		FinalError: "",
	}

	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Spec.Action, "", "Action field set to empty")
}

func (suite *RGControllerTestSuite) TestActionInProgressWithMissingEverything() {
	actionName := "Failback_Local"
	replicationGroup := suite.getLocalRG(suite.driver.RGName)
	actionType := ActionType(actionName)
	replicationGroup.Status.State = actionType.getInProgressState()
	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if state is changed to "Ready"
	suite.Equal(ReadyState, rg.Status.State, "State is Ready")
	suite.Equal(rg.Spec.Action, "", "Action field set to empty")
}

func (suite *RGControllerTestSuite) TestActionInProgressWithIncorrectAnnotation() {
	actionName := "Failover_Local"

	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if state is changed to "Ready"
	suite.Equal(ReadyState, rg.Status.State, "State is Ready")
	expectedAnnotation := ActionAnnotation{
		ActionName: ActionType(actionName).String(),
		Completed:  true,
		FinalError: "",
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Spec.Action, "", "Action field set to empty")
}

func (suite *RGControllerTestSuite) TestActionInProgressWithMismatchingAction() {
	// scenario: State is ActionInProgress, correct ActionInProgress annotation & user modifies the action field
	// expectation: controller ignores this & finishes the current action
	actionName := "Sync"
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	// Reconcile
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if state is changed to "Ready"
	suite.Equal(ReadyState, rg.Status.State, "State is Ready")
	expectedAnnotation := ActionAnnotation{
		ActionName: ActionType(actionName).String(),
		Completed:  true,
		FinalError: "",
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Spec.Action, "", "Action field set to empty")
}

func (suite *RGControllerTestSuite) TestActionInErrorStateWithContextDeadlineError() {
	// scenario: When there is valid PG-ID and state is in Error
	// because of a previosuly failed action
	// and driver returns a temporary error
	actionName := "Resume"
	actionType := ActionType(actionName)
	replicationGroup := suite.createRGInActionInProgressState("", actionName, false, false)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error
	errorMsg := "failed to perform action"
	failedPreconditionError := status.Error(codes.FailedPrecondition, errorMsg)
	suite.repClient.InjectErrorAutoClear(failedPreconditionError)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)

	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	expectedAnnotation := ActionAnnotation{
		ActionName: actionType.String(),
		Completed:  true,
		FinalError: failedPreconditionError.Error(),
	}
	var gotAnnotation ActionAnnotation
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(rg.Status.State, ErrorState, "State is in error")
	// Last action set
	suite.Contains(rg.Status.LastAction.Condition, actionType.String(), "Last Action was updated")
	suite.Equal(failedPreconditionError.Error(), rg.Status.LastAction.ErrorMessage, "expected error")
	// Also verify if FirstFailure was set
	suite.NotNil(rg.Status.LastAction.FirstFailure)

	// Now we are in error state
	// Lets start another action
	actionName = "Failover_remote"
	actionType = ActionType(actionName)
	rg.Spec.Action = actionName

	err = suite.client.Update(context.Background(), rg)
	suite.NoError(err)

	// Reconcile a few times
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)

	deadlineExceededError := status.Error(codes.DeadlineExceeded, "context deadline exceeded")
	suite.repClient.InjectErrorAutoClear(deadlineExceededError)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)

	//_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, deadlineExceededError.Error())
	// Get the updated RG
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")
	expectedAnnotation = ActionAnnotation{
		ActionName: actionType.String(),
		Completed:  false,
		FinalError: "",
	}
	err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
	suite.NoError(err, "No error on JSON unmarshal of action annotation")
	suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
	suite.Equal(actionType.getInProgressState(), rg.Status.State, "State is in progress")
	// Last action set
	suite.Contains(rg.Status.LastAction.Condition, actionType.String(), "Last Action was updated")

	/*
		// Reconcile again after the error has been cleared
		_, err = suite.rgReconcile.Reconcile(context.Background(), req)
		suite.NoError(err, "No error on RG reconcile")
		err = suite.client.Get(context.Background(), req.NamespacedName, rg)
		suite.NoError(err, "No error on RG Get")
		err = json.Unmarshal([]byte(rg.GetAnnotations()[Action]), &gotAnnotation)
		suite.NoError(err, "No error on JSON unmarshal of action annotation")
		expectedAnnotation.Completed = true
		suite.Equal(true, areAnnotationsEqual(expectedAnnotation, gotAnnotation), "Action completed")
		// Check if condition was updated
		suite.Equal(fmt.Sprintf("Action %s succeeded", actionType.String()),
			rg.Status.Conditions[0].Condition)

	*/
}

func (suite *RGControllerTestSuite) TestInvalidAction() {
	// Scenario: RG - Ready State, Action - Invalid
	actionName := "fail0ver_local"

	replicationGroup := suite.getLocalRG("")
	replicationGroup.Status.State = ReadyState
	replicationGroup.Spec.Action = actionName

	err := suite.client.Create(context.Background(), &replicationGroup)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.Nil(err, "RG Reconcile shouldn't fail")

	// Get the updated RG
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")
	suite.Equal(ReadyState, rg.Status.State, "State is still Ready")
	suite.Equal("", rg.Spec.Action, "Action is set to empty")
}

func (suite *RGControllerTestSuite) TestNoDeletionTimeStamp() {
	// scenario: When there is no deletion time stamp
	replicationGroup := suite.getLocalRG("")
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)

	suite.client = utils.GetFakeClientWithObjects(&replicationGroup)
	suite.rgReconcile.Client = suite.client
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.Nil(err, "This should reconcile successfully")
}

func TestSetupWithManagerRg(t *testing.T) {
	tests := []struct {
		name                       string
		MaxRetryDurationForActions time.Duration
		manager                    ctrl.Manager
		limiter                    workqueue.TypedRateLimiter[reconcile.Request]
		maxReconcilers             int
		wantError                  bool
	}{
		{
			name:                       "Manager is nil AND MaxRetryDurationForActions is zero",
			MaxRetryDurationForActions: 0,
			manager:                    nil,
			limiter:                    nil,
			maxReconcilers:             0,
			wantError:                  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReplicationGroupReconciler{
				MaxRetryDurationForActions: tt.MaxRetryDurationForActions,
			}
			err := r.SetupWithManager(tt.manager, tt.limiter, tt.maxReconcilers)
			if (err != nil) != tt.wantError {
				t.Errorf("SetupWithManager() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestReplicationGroupReconciler_getAction(t *testing.T) {
	tests := []struct {
		name       string
		actionType ActionType
		wantAction *csiext.ExecuteActionRequest_Action
		wantErr    bool
	}{
		{
			name:       "supportedAction is nil",
			actionType: ActionType("InvalidAction"),
			wantAction: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReplicationGroupReconciler{
				SupportedActions: []*csiext.SupportedActions{
					nil,
				},
			}

			gotAction, err := r.getAction(tt.actionType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReplicationGroupReconciler.getAction() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotAction, tt.wantAction) {
				t.Errorf("ReplicationGroupReconciler.getAction() = %v, want %v", gotAction, tt.wantAction)
			}
		})
	}
}

func TestReplicationGroupReconciler_addFinalizer(t *testing.T) {
	originalGetReplicationGroupRecouncilerUpdate := getReplicationGroupRecouncilerUpdate

	after := func() {
		getReplicationGroupRecouncilerUpdate = originalGetReplicationGroupRecouncilerUpdate
	}

	tests := []struct {
		name    string
		setup   func()
		rg      *repv1.DellCSIReplicationGroup
		wantErr bool
	}{
		{
			name: "Add finalizer with error",
			setup: func() {
				getReplicationGroupRecouncilerUpdate = func(_ *ReplicationGroupReconciler, _ context.Context, _ client.Object) error {
					return errors.New("Failed to add finalizer")
				}
			},
			rg:      &repv1.DellCSIReplicationGroup{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			tt.setup()
			ctx := context.Background()
			r := &ReplicationGroupReconciler{}

			ok, err := r.addFinalizer(ctx, tt.rg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReplicationGroupReconciler.addFinalizer() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				assert.True(t, ok, "Finalizer should be added")
			}
		})
	}
}

func TestActionType_Equals(t *testing.T) {
	type args struct {
		ctx context.Context
		val string
	}
	tests := []struct {
		name string
		a    ActionType
		args args
		want bool
	}{
		{
			name: "Equal action types",
			a:    "Action1",
			args: args{
				ctx: context.Background(),
				val: "action1",
			},
			want: true,
		},
		{
			name: "Different action types",
			a:    "Action1",
			args: args{
				ctx: context.Background(),
				val: "action2",
			},
			want: false,
		},
		{
			name: "Empty action type",
			a:    "",
			args: args{
				ctx: context.Background(),
				val: "action1",
			},
			want: false,
		},
		{
			name: "Empty value",
			a:    "Action1",
			args: args{
				ctx: context.Background(),
				val: "",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equals(tt.args.ctx, tt.args.val); got != tt.want {
				t.Errorf("ActionType.Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetActionResultFromActionAnnotation(t *testing.T) {
	testTime, _ := json.Marshal(metav1.Time{Time: time.Now()})

	tests := []struct {
		name          string
		actionAnnot   ActionAnnotation
		expectedError error
	}{
		{
			name: "Action annotation with invalid finish time",
			actionAnnot: ActionAnnotation{
				ActionName: "TestAction",
				FinishTime: "invalid time",
			},
			expectedError: errors.New(""),
		},
		{
			name: "Action annotation with invalid protection group status",
			actionAnnot: ActionAnnotation{
				ActionName:            "TestAction",
				FinishTime:            string(testTime),
				ProtectionGroupStatus: "invalid",
			},
			expectedError: errors.New(""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := getActionResultFromActionAnnotation(ctx, test.actionAnnot)
			if err == nil && test.expectedError != nil {
				t.Errorf("Expected error, but got nil")
			}
			if err != nil && test.expectedError == nil {
				t.Errorf("Expected nil error, but got %v", err)
			}
		})
	}
}

func TestUpdateRGStatusWithActionResult(t *testing.T) {
	dummyActionAnnotation, _ := json.Marshal(ActionAnnotation{
		FinalError: "",
		FinishTime: "invalid time",
	})
	tests := []struct {
		name         string
		rg           *repv1.DellCSIReplicationGroup
		actionResult *ActionResult
		expectedErr  error
	}{
		{
			name: "ActionResult is nil AND failed in getActionInProgress",
			rg: &repv1.DellCSIReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						Action:                        "action",
						controllers.SnapshotNamespace: "default",
						controllers.SnapshotClass:     "test",
					},
				},
			},
			actionResult: nil,
			expectedErr:  errors.New(""),
		},
		{
			name: "ActionResult is nil AND failed in getActionResultFromActionAnnotation",
			rg: &repv1.DellCSIReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						Action:                        string(dummyActionAnnotation),
						controllers.SnapshotNamespace: "default",
						controllers.SnapshotClass:     "test",
					},
				},
			},
			actionResult: nil,
			expectedErr:  errors.New(""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			err := updateRGStatusWithActionResult(ctx, test.rg, test.actionResult)
			if err == nil && test.expectedErr != nil {
				t.Errorf("Expected error, but got nil")
			}
			if err != nil && test.expectedErr == nil {
				t.Errorf("Expected nil error, but got %v", err)
			}
		})
	}
}

func TestUpdateActionAttributes(t *testing.T) {
	tests := []struct {
		name          string
		actionType    ActionType
		actionAttrs   map[string]string
		expectedAttrs map[string]string
	}{
		{
			name:          "ActionType is CREATE_SNAPSHOT and result.ActionAttributes is not empty",
			actionType:    ActionType(csiext.ActionTypes_CREATE_SNAPSHOT.String()),
			actionAttrs:   map[string]string{"key1": "value1", "key2": "value2"},
			expectedAttrs: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name:          "ActionType is not CREATE_SNAPSHOT",
			actionType:    ActionType(csiext.ActionTypes_ABORT_SNAPSHOT.String()),
			actionAttrs:   map[string]string{"key1": "value1", "key2": "value2"},
			expectedAttrs: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			rg := &repv1.DellCSIReplicationGroup{}
			result := &ActionResult{
				ActionType:       test.actionType,
				ActionAttributes: test.actionAttrs,
			}

			updateActionAttributes(ctx, rg, result)

			if test.expectedAttrs == nil {
				if rg.Status.LastAction.ActionAttributes != nil {
					t.Errorf("Expected ActionAttributes to be nil, but it was not")
				}
			} else {
				if rg.Status.LastAction.ActionAttributes == nil {
					t.Errorf("Expected ActionAttributes to be set, but it was nil")
				}

				for key, val := range test.expectedAttrs {
					if rg.Status.LastAction.ActionAttributes[key] != val {
						t.Errorf("Expected ActionAttributes to be set correctly, but it was not")
					}
				}
			}
		})
	}
}
