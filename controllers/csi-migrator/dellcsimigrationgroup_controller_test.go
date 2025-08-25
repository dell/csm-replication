/*
 *
 * Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

/*
Copyright © 2023-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"testing"
	"time"

	csimigration "github.com/dell/csm-replication/test/mocks"

	storagev1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common/constants"
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

type MGControllerTestSuite struct {
	suite.Suite
	client          client.Client
	driver          utils.Driver
	migrationClient *csimigration.MockMigration
	mgReconcile     *MigrationGroupReconciler
}

func (suite *MGControllerTestSuite) SetupSuite() {
	// do nothing
}

func (suite *MGControllerTestSuite) SetupTest() {
	suite.Init()
	suite.initReconciler()
}

func (suite *MGControllerTestSuite) Init() {
	suite.driver = utils.GetDefaultDriver()
	migrationClient := csimigration.NewFakeMigrationClient(utils.ContextPrefix)
	suite.migrationClient = &migrationClient
}

func (suite *MGControllerTestSuite) initReconciler() {
	logger := ctrl.Log.WithName("controllers").WithName("DellCSIMigrationGroup")
	fakeRecorder := record.NewFakeRecorder(100)
	// Initialize the annotations & labels
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)

	suite.mgReconcile = &MigrationGroupReconciler{
		Client:                     suite.client,
		Log:                        logger,
		Scheme:                     utils.Scheme,
		DriverName:                 suite.driver.DriverName,
		EventRecorder:              fakeRecorder,
		MigrationClient:            suite.migrationClient,
		MaxRetryDurationForActions: MaxRetryDurationForActions,
	}
}

func TestMGControllerTestSuite(t *testing.T) {
	testSuite := new(MGControllerTestSuite)
	suite.Run(t, testSuite)
}

func (suite *MGControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}

// Return a base migration group
func getTypicalMigrationGroup() *storagev1.DellCSIMigrationGroup {
	return &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      ReadyState,
		},
	}
}

// MG with ReadyState
func (suite *MGControllerTestSuite) TestMGReconcileWithReadyState() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      ReadyState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(MigratedState, mg.Status.State, "State should be Ready")
	suite.Equal(ReadyState, mg.Status.LastAction, "State should be Ready")
}

// MG with MigratedState
func (suite *MGControllerTestSuite) TestMGReconcileWithMigratedState() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      MigratedState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(CommitReadyState, mg.Status.State, "State should be Ready")
}

// MG with CommitReadyState
func (suite *MGControllerTestSuite) TestMGReconcileWithCommitReadyState() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      CommitReadyState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(CommittedState, mg.Status.State, "State should be Ready")
	suite.Equal(CommitReadyState, mg.Status.LastAction, "State should be Ready")
}

// MG with CommittedState
func (suite *MGControllerTestSuite) TestMGReconcileWithCommittedState() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      CommittedState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(DeletingState, mg.Status.State, "State should be Ready")
	suite.Equal(CommittedState, mg.Status.LastAction, "State should be Ready")
}

// MG with DeletingState
func (suite *MGControllerTestSuite) TestMGReconcileWithDeletingState() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      DeletingState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
}

// MG with ErrorState
func (suite *MGControllerTestSuite) TestMGReconcileWithErrorState() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      ErrorState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(MigratedState, mg.Status.State, "State should be Ready")
}

// MG with ErrorState
func (suite *MGControllerTestSuite) TestMGReconcileWithErrorState_fromReady() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "Ready",
			State:      ErrorState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(MigratedState, mg.Status.State, "State should be Ready")
}

// MG with ErrorState
func (suite *MGControllerTestSuite) TestMGReconcileWithErrorState_fromMigrated() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "Migrated",
			State:      ErrorState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(CommitReadyState, mg.Status.State, "State should be Ready")
}

// MG with ErrorState
func (suite *MGControllerTestSuite) TestMGReconcileWithErrorState_fromCommitReady() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "CommitReady",
			State:      ErrorState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(CommittedState, mg.Status.State, "State should be Ready")
}

// MG with ErrorState
func (suite *MGControllerTestSuite) TestMGReconcileWithErrorState_fromCommitted() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "Committed",
			State:      ErrorState,
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()
	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(DeletingState, mg.Status.State, "State should be Ready")
}

// MG with InvalidState
func (suite *MGControllerTestSuite) TestMGReconcileWithInvalidState() {
	mg1 := &storagev1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      "Unknown",
		},
	}

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()

	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, "Unknown state")

	mg := new(storagev1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG reconcile")
}

func (suite *MGControllerTestSuite) getTypicalReconcileRequest(name string) reconcile.Request {
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "",
			Name:      name,
		},
	}
	return req
}

func (suite *MGControllerTestSuite) TestSetupWithManagerMG() {
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second)
	err := suite.mgReconcile.SetupWithManager(mgr, expRateLimiter, 1)
	suite.Error(err, "Setup should fail when there is no manager")
}

func (suite *MGControllerTestSuite) TestIgnoreMGNotFound() {
	// If the MG is not found, return an empty result
	mg1 := getTypicalMigrationGroup()

	// create a fake client, but do not give it info about the mg in order
	// to trigger MG not found error.
	suite.client = utils.GetFakeClient()
	suite.mgReconcile.Client = suite.client

	req := suite.getTypicalReconcileRequest(mg1.Name)
	res, err := suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(ctrl.Result{}, res)
}

func (suite *MGControllerTestSuite) TestUpdateMGSpecWithActionResultSuccess() {
	// successfully update the MG spec with the action result
	mg1 := getTypicalMigrationGroup()

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	isUpdated := suite.mgReconcile.updateMGSpecWithActionResult(context.Background(), mg1, "Delete")
	suite.True(isUpdated, "MG should be updated")
}

func (suite *MGControllerTestSuite) TestAddfinalizer() {
	// Successfully add MG finalizers to a migration group

	mg1 := getTypicalMigrationGroup()

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ok, err := suite.mgReconcile.addFinalizer(context.Background(), mg1)
	suite.NoError(err)
	suite.True(ok)
}

func (suite *MGControllerTestSuite) TestAddfinalizerFailToUpdate() {
	// Add the finalizer to the MG but fail when calling reconciler Update

	mg1 := getTypicalMigrationGroup()

	// create a client without knowledge of the migration group in order
	// to force a failed update
	suite.client = utils.GetFakeClient()
	suite.mgReconcile.Client = suite.client

	ok, err := suite.mgReconcile.addFinalizer(context.Background(), mg1)
	suite.Error(err)
	suite.Contains(err.Error(), "not found")
	suite.True(ok)
}

func (suite *MGControllerTestSuite) TestUpdateMGOnError() {
	// Successfully update the MG on error

	mg1 := getTypicalMigrationGroup()

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	err := suite.mgReconcile.updateMGOnError(context.Background(), mg1, DeletingState, "")
	suite.NoError(err)
}

func (suite *MGControllerTestSuite) TestUpdateMGSpecWithStateFail() {
	// Unsuccessfully attempt to update the MG spec with the given state

	mg1 := getTypicalMigrationGroup()

	// create a client without knowledge of the migration group in order
	// to force a failed update
	suite.client = utils.GetFakeClient()
	suite.mgReconcile.Client = suite.client

	err := suite.mgReconcile.updateMGSpecWithState(context.Background(), mg1, DeletingState)
	suite.Error(err)
	suite.Contains(err.Error(), "not found")
}

func (suite *MGControllerTestSuite) TestProcessMGInNoState() {
	// Process a brand new MG with no state and no finalizers.
	// Should add the MG finalizers and set for requeueing.

	mg := getTypicalMigrationGroup()
	mg.Status.State = NoState

	suite.client = utils.GetFakeClientWithObjects(mg)
	suite.mgReconcile.Client = suite.client

	// Processing should add the finalizers and requeue
	result, err := suite.mgReconcile.processMGInNoState(context.Background(), mg.DeepCopy())
	suite.NoError(err, "Should not return an error")
	suite.True(result.Requeue, "Should be set to requeue")
}

func (suite *MGControllerTestSuite) TestProcessMGWithFinalizersInNoState() {
	// Process a brand new MG that has finalizers, but no state.
	// Should update the MG with 'Ready' state.

	mg1 := getTypicalMigrationGroup()

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()

	// add the finalizers
	ok, err := suite.mgReconcile.addFinalizer(ctx, mg1)
	suite.NoError(err)
	suite.True(ok)

	// Processing should add the finalizers and requeue
	result, err := suite.mgReconcile.processMGInNoState(ctx, mg1.DeepCopy())
	suite.NoError(err, "Should not return an error")
	suite.Equal(ctrl.Result{}, result, "Should be empty result")

	// Get the updated MG to confirm it is in 'Ready' state
	updatedMG := &storagev1.DellCSIMigrationGroup{}
	err = suite.mgReconcile.Client.Get(ctx, types.NamespacedName{Name: mg1.Name}, updatedMG)
	suite.Nil(err, "MG should be retrieved successfully")
	suite.Equal(ReadyState, updatedMG.Status.State, "MG should now be in 'Ready' state")
}

func (suite *MGControllerTestSuite) TestRemoveFinalizer() {
	// Should successfully remove MG finalizer from the MG.
	mg1 := getTypicalMigrationGroup()

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()

	// add the finalizers to be removed
	ok, err := suite.mgReconcile.addFinalizer(ctx, mg1)
	suite.NoError(err)
	suite.True(ok)

	// remove the finalizers
	err = suite.mgReconcile.removeFinalizer(ctx, mg1.DeepCopy())
	suite.NoError(err, "Should remove the finalizers without issues")

	// get the MG to confirm the finalizers have been removed
	updatedMG := &storagev1.DellCSIMigrationGroup{}
	err = suite.mgReconcile.Client.Get(ctx, types.NamespacedName{Name: mg1.Name}, updatedMG)
	suite.Nil(err, "MG should be retrieved successfully")
	suite.Equal(0, len(updatedMG.Finalizers), "MG should have no finalizers")
}

func (suite *MGControllerTestSuite) TestRemoveFinalizerFailure() {
	// Should fail to remove MG finalizer from the MG.
	mg1 := getTypicalMigrationGroup()

	suite.client = utils.GetFakeClientWithObjects(mg1)
	suite.mgReconcile.Client = suite.client

	ctx := context.Background()

	// add the finalizers to be removed
	ok, err := suite.mgReconcile.addFinalizer(ctx, mg1)
	suite.NoError(err)
	suite.True(ok)

	// Replace the client with a new client that has no knowledge
	// of the migration group, mg1, in order to force a failed update.
	suite.client = utils.GetFakeClient()
	suite.mgReconcile.Client = suite.client

	// remove the finalizers
	err = suite.mgReconcile.removeFinalizer(ctx, mg1.DeepCopy())
	suite.Error(err, "Should fail to remove the finalizers")
	suite.Contains(err.Error(), "not found")
}
