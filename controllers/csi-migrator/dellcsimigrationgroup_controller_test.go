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
	csimigration "github.com/dell/csm-replication/pkg/csi-clients/migration"
	"testing"
	"time"

	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
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
	fakeClient := errorFakeCtrlRuntimeClient{
		Client: utils.GetFakeClient(),
	}
	suite.client = fakeClient
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

//MG with ReadyState
func (suite *MGControllerTestSuite) TestMGReconcileWithReadyState() {
	mg1 := &storagev1alpha1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1alpha1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1alpha1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      ReadyState,
		},
	}

	ctx := context.Background()
	err := suite.client.Create(ctx, mg1)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(MigratedState, mg.Status.State, "State should be Ready")
	suite.Equal(ReadyState, mg.Status.LastAction, "State should be Ready")
}

//MG with MigratedState
func (suite *MGControllerTestSuite) TestMGReconcileWithMigratedState() {
	mg1 := &storagev1alpha1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1alpha1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1alpha1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      MigratedState,
		},
	}

	ctx := context.Background()
	err := suite.client.Create(ctx, mg1)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(CommitReadyState, mg.Status.State, "State should be Ready")

}

//MG with CommitReadyState
func (suite *MGControllerTestSuite) TestMGReconcileWithCommitReadyState() {
	mg1 := &storagev1alpha1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1alpha1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1alpha1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      CommitReadyState,
		},
	}

	ctx := context.Background()
	err := suite.client.Create(ctx, mg1)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(CommittedState, mg.Status.State, "State should be Ready")
	suite.Equal(CommitReadyState, mg.Status.LastAction, "State should be Ready")

}

//MG with CommittedState
func (suite *MGControllerTestSuite) TestMGReconcileWithCommittedState() {
	mg1 := &storagev1alpha1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1alpha1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1alpha1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      CommittedState,
		},
	}

	ctx := context.Background()
	err := suite.client.Create(ctx, mg1)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(DeletingState, mg.Status.State, "State should be Ready")
	suite.Equal(CommittedState, mg.Status.LastAction, "State should be Ready")
}

//MG with DeletingState
func (suite *MGControllerTestSuite) TestMGReconcileWithDeletingState() {
	mg1 := &storagev1alpha1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1alpha1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1alpha1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      DeletingState,
		},
	}

	ctx := context.Background()
	err := suite.client.Create(ctx, mg1)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
}

//MG with ErrorState
func (suite *MGControllerTestSuite) TestMGReconcileWithErrorState() {
	mg1 := &storagev1alpha1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1alpha1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1alpha1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      ErrorState,
		},
	}

	ctx := context.Background()
	err := suite.client.Create(ctx, mg1)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on MG reconcile")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: mg1.Name}, mg)
	suite.NoError(err, "No error on MG get")
	suite.Equal(MigratedState, mg.Status.State, "State should be Ready")
}

//MG with InvalidState
func (suite *MGControllerTestSuite) TestMGReconcileWithInvalidState() {
	mg1 := &storagev1alpha1.DellCSIMigrationGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "mg1"},
		Spec: storagev1alpha1.DellCSIMigrationGroupSpec{
			DriverName:               "driverName",
			SourceID:                 "001",
			TargetID:                 "0002",
			MigrationGroupAttributes: map[string]string{"test": "test"},
		},
		Status: storagev1alpha1.DellCSIMigrationGroupStatus{
			LastAction: "",
			State:      "Unknown",
		},
	}

	ctx := context.Background()
	err := suite.client.Create(ctx, mg1)
	suite.Nil(err)

	req := suite.getTypicalReconcileRequest(mg1.Name)
	_, err = suite.mgReconcile.Reconcile(context.Background(), req)
	suite.EqualError(err, "Unknown state")

	mg := new(storagev1alpha1.DellCSIMigrationGroup)
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
	expRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 10*time.Second)
	err := suite.mgReconcile.SetupWithManager(mgr, expRateLimiter, 1)
	suite.Error(err, "Setup should fail when there is no manager")
}
