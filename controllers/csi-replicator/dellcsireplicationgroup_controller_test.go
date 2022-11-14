/*
 Copyright © 2021-2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"os"
	"strings"
	"testing"
	"time"

	csiext "github.com/dell/dell-csi-extensions/replication"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
	//_ = storagev1alpha1.AddToScheme(scheme.Scheme)
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
	//testSuite.TearDownTestSuite()
}

func (suite *RGControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}

func (suite *RGControllerTestSuite) getLocalRG(name string) storagev1alpha1.DellCSIReplicationGroup {
	//creating fake replication group
	if name == "" {
		name = suite.driver.RGName
	}
	replicationGroup := utils.GetRGObj(name, suite.driver.DriverName, suite.driver.RemoteClusterID,
		utils.LocalPGID, utils.RemotePGID, suite.getLocalParams(), suite.getRemoteParams())
	return *replicationGroup
}

func (suite *RGControllerTestSuite) getRemoteRG(name string) storagev1alpha1.DellCSIReplicationGroup {
	//creating fake replication group
	if name == "" {
		name = suite.driver.RGName
	}
	replicationGroup := utils.GetRGObj(name, suite.driver.DriverName, suite.driver.SourceClusterID,
		utils.RemotePGID, utils.LocalPGID, suite.getRemoteParams(), suite.getLocalParams())
	return *replicationGroup
}

func (suite *RGControllerTestSuite) createRGInActionInProgressState(name string, actionName string,
	completed bool, deleteInProgress bool) (*storagev1alpha1.DellCSIReplicationGroup, error) {

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
	err := suite.client.Create(context.Background(), &rg)
	if err != nil {
		return nil, err
	}
	return &rg, nil
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
	//scenario: Reconcile for an object which doesn't exist (has been deleted)
	req := suite.getTypicalReconcileRequest(suite.driver.RGName)
	_, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
}

func (suite *RGControllerTestSuite) TestReconcileWithNoState() {
	//scenario: When there is no state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	resp, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	suite.Equal(true, resp.Requeue, "Requeue is set to true")

	//Reconcile again
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err, "No error on RG get")
	suite.Equal(ReadyState, rg.Status.State, "State should be Ready")
	suite.Contains(rg.Finalizers, controllers.ReplicationFinalizer, "Finalizer should be present")

}

func (suite *RGControllerTestSuite) TestReconcileWithEmptyPGID() {
	//scenario: When there is no protection group ID and no valid state is set
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.Spec.ProtectionGroupID = ""
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, "missing protection group id")
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err, "No error on RG get")
	suite.Equal(InvalidState, rg.Status.State, "State is set to invalid")
}

func (suite *RGControllerTestSuite) TestDeleteWithInvalidState() {
	//scenario: When there is valid deletion time-stamp and invalid state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = InvalidState
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	res, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	suite.Equal(res.Requeue, false, "Requeue should be set to false")
}

func (suite *RGControllerTestSuite) TestReconcileWithInvalidStateRepeated() {
	//scenario: When there is invalid state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.Spec.ProtectionGroupID = ""
	replicationGroup.Status.State = InvalidState
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal(InvalidState, rg.Status.State, "State is still Invalid")
}

func (suite *RGControllerTestSuite) TestReconcileWithInvalidStateAfterFix() {
	//scenario: When there is invalid state
	// and PGID has been added back
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.Status.State = InvalidState
	replicationGroup.Spec.ProtectionGroupID = protectionGroupID
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal("", rg.Status.State, "State changed to empty")
}

func (suite *RGControllerTestSuite) TestDeleteWithInvalidStateAndNoPGID() {
	//scenario: When there is valid deletion time-stamp and invalid state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = InvalidState
	replicationGroup.Spec.ProtectionGroupID = ""
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	res, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	suite.Equal(res.Requeue, false, "Requeue should be set to false")

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on reconcile")

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	//scenario: When there is valid deletion time-stamp and no state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	res, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	suite.Equal(res.Requeue, false, "Requeue should be set to false")
}

func (suite *RGControllerTestSuite) TestDeleteWithReadyState() {
	//scenario: When there is valid deletion time-stamp and Ready state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	// Set Finalizer
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = ReadyState
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.Error(err, "RG object not found")
}

func (suite *RGControllerTestSuite) TestDeleteWithErrors() {
	//scenario: When there is valid deletion time-stamp and Ready state
	ctx := context.Background()
	replicationGroup := suite.getLocalRG("")
	// Set Finalizer
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)
	replicationGroup.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	replicationGroup.Status.State = ReadyState
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject temporary error
	errorMsg := fmt.Sprintf("error during delete")
	suite.repClient.InjectErrorClearAfterN(fmt.Errorf(errorMsg), 5)

	for i := 0; i < 5; i++ {
		_, err := suite.rgReconcile.Reconcile(context.Background(), req)
		suite.EqualError(err, errorMsg, "error should match injected error")
	}

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
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
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject error which doesn't clear
	errorMsg := fmt.Sprintf("error during delete")
	suite.repClient.InjectError(fmt.Errorf(errorMsg))
	defer suite.repClient.ClearErrorAndCondition(true)

	// Reconcile a few times
	for i := 0; i < 20; i++ {
		_, err := suite.rgReconcile.Reconcile(context.Background(), req)
		suite.EqualError(err, errorMsg, "error should match injected error")
	}

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
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
	err := suite.client.Create(ctx, &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject temporary error
	errorMsg := fmt.Sprintf("error during delete")
	suite.repClient.InjectErrorAutoClear(fmt.Errorf(errorMsg))
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, errorMsg, "error should match injected error")

	// Inject another error which is final
	errorMsg = "failed-pre-condition"
	suite.repClient.InjectErrorAutoClear(status.Error(codes.FailedPrecondition, errorMsg))
	resp, err := suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "no error after a final error")
	suite.Equal(false, resp.Requeue, "Requeue should not be true")

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	//scenario: When there is valid PG-ID and state is in ActionInProgress
	actionName := "Resume"
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, true)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(ctx, req.NamespacedName, rg)
	suite.NoError(err)

	// Reconcile
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	var latestRG storagev1alpha1.DellCSIReplicationGroup
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

func (suite *RGControllerTestSuite) prettyPrint(rg storagev1alpha1.DellCSIReplicationGroup) {
	suite.T().Log("RG Name", rg.Name)
	suite.T().Log("RG Annotations", rg.GetAnnotations())
	suite.T().Log("RG Labels", rg.GetLabels())
	suite.T().Log("RG Spec", rg.Spec)
	suite.T().Log("RG Status", rg.Status)
}

func (suite *RGControllerTestSuite) TestActionInReadyState() {
	//scenario: When there is valid PG-ID and Suspend action
	actionName := "Suspend"
	actionType := ActionType(actionName)

	replicationGroup := suite.getLocalRG("")
	replicationGroup.Status.State = "Ready"
	replicationGroup.Spec.Action = actionName

	err := suite.client.Create(context.Background(), &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Reconcile twice
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	//scenario: When there is valid PG-ID and state is in ActionInProgress
	actionName := "Failover_Local"
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	// Inject condition
	suite.repClient.SetCondition(csireplication.ExecuteActionWithSwap)
	// Reconcile
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error
	errorMsg := "failed to perform action"
	suite.repClient.InjectErrorAutoClear(fmt.Errorf("%s", errorMsg))
	// Reconcile
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, errorMsg)

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error
	deadlineExceededError := status.Error(codes.DeadlineExceeded, "context deadline exceeded")
	suite.repClient.InjectErrorAutoClear(deadlineExceededError)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	// Reconcile
	_, err = suite.rgReconcile.Reconcile(ctx, req)
	suite.EqualError(err, deadlineExceededError.Error())

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)

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
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error
	errorMsg := "failed to perform action"
	failedPreconditionError := status.Error(codes.FailedPrecondition, errorMsg)
	suite.repClient.InjectErrorAutoClear(failedPreconditionError)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)

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
	//scenario: When there is valid PG-ID and state is in ActionInProgress
	// and driver returns errors
	actionName := "Failback_Local"
	actionType := ActionType(actionName)
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	rg := new(storagev1alpha1.DellCSIReplicationGroup)

	errorMsg := "can't process request"
	suite.repClient.InjectErrorClearAfterN(fmt.Errorf("%s", errorMsg), 10)
	for i := 0; i < 10; i++ {
		_, err = suite.rgReconcile.Reconcile(context.Background(), req)
		suite.EqualError(err, errorMsg)
		// Get the updated RG
		err = suite.client.Get(context.Background(), req.NamespacedName, rg)
		suite.NoError(err, "No error on RG Get")
		suite.Equal(actionName, rg.Spec.Action, "Action field still set")
		suite.Equal(actionType.getInProgressState(), rg.Status.State, "State is in progress")
	}

	// Final reconcile
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on reconcile")
	// Refresh RG
	var latestRG storagev1alpha1.DellCSIReplicationGroup
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
	//scenario: When there is valid PG-ID and state is in ActionInProgress
	// and driver returns errors
	actionName := "Failover_Remote"
	actionType := ActionType(actionName)
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.NoError(err, "No error on create")
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
		_, err = suite.rgReconcile.Reconcile(context.Background(), req)
		// Get the updated RG
		rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	err := suite.client.Create(context.Background(), &replicationGroup)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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

func conditionPresent(conditions []storagev1alpha1.LastAction, msg string) bool {
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
	err := suite.client.Create(context.Background(), &replicationGroup)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	err := suite.client.Create(context.Background(), &replicationGroup)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	err := suite.client.Create(context.Background(), &replicationGroup)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")

	// Verify if state is changed to "Ready"
	suite.Equal(ReadyState, rg.Status.State, "State is Ready")
	suite.Equal(rg.Spec.Action, "", "Action field set to empty")
}

func (suite *RGControllerTestSuite) TestActionInProgressWithIncorrectAnnotation() {
	actionName := "Failover_Local"

	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	//scenario: State is ActionInProgress, correct ActionInProgress annotation & user modifies the action field
	//expectation: controller ignores this & finishes the current action
	actionName := "Sync"
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	// Reconcile
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on RG reconcile")

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	replicationGroup, err := suite.createRGInActionInProgressState("", actionName, false, false)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)

	// Inject an error
	errorMsg := "failed to perform action"
	failedPreconditionError := status.Error(codes.FailedPrecondition, errorMsg)
	suite.repClient.InjectErrorAutoClear(failedPreconditionError)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)

	// Get the updated RG
	rg := new(storagev1alpha1.DellCSIReplicationGroup)

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
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err, "No error on RG Get")
	suite.Equal(ReadyState, rg.Status.State, "State is still Ready")
	suite.Equal("", rg.Spec.Action, "Action is set to empty")
}

func (suite *RGControllerTestSuite) TestNoDeletionTimeStamp() {
	//scenario: When there is no deletion time stamp
	replicationGroup := suite.getLocalRG("")
	replicationGroup.Finalizers = append(replicationGroup.Finalizers, controllers.ReplicationFinalizer)

	err := suite.client.Create(context.Background(), &replicationGroup)
	suite.Nil(err)
	req := suite.getTypicalReconcileRequest(replicationGroup.Name)
	_, err = suite.rgReconcile.Reconcile(context.Background(), req)
	suite.Nil(err, "This should reconcile successfully")
}

func (suite *RGControllerTestSuite) TestSetupWithManagerRg() {
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 10*time.Second)
	err := suite.rgReconcile.SetupWithManager(mgr, expRateLimiter, 1)
	suite.Error(err, "Setup should fail when there is no manager")
}
