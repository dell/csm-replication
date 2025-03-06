/*
 Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"errors"
	"fmt"
	"path"
	"testing"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	csireplication "github.com/dell/csm-replication/test/mocks"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type PersistentVolumeControllerTestSuite struct {
	suite.Suite
	driver     utils.Driver
	client     client.Client
	repClient  *csireplication.MockReplication
	reconciler *PersistentVolumeReconciler
}

func (suite *PersistentVolumeControllerTestSuite) SetupTest() {
	suite.Init()
	suite.initReconciler()
}

func (suite *PersistentVolumeControllerTestSuite) Init() {
	suite.driver = utils.GetDefaultDriver()
	utils.InitializeSchemes()
	suite.client = fake.NewClientBuilder().
		WithScheme(utils.Scheme).
		WithIndex(GetProtectionGroupIndexer()).
		WithObjects(suite.getFakeStorageClass()).
		Build()
	mockReplicationClient := csireplication.NewFakeReplicationClient(utils.ContextPrefix)
	suite.repClient = &mockReplicationClient
	var sc storagev1.StorageClass
	err := suite.client.Get(context.Background(), types.NamespacedName{Name: suite.driver.StorageClass}, &sc)
	suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) getTypicalReconcileRequest(name string) reconcile.Request {
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "",
			Name:      name,
		},
	}
	return req
}

func (suite *PersistentVolumeControllerTestSuite) getTypicalManagerManager() manager.Manager {
	scheme := runtime.NewScheme()
	metricsAddr := ""
	enableLeaderElection := false

	mgr, _ := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsServer.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer:              webhook.NewServer(webhook.Options{Port: 9443}),
		LeaderElection:             enableLeaderElection,
		LeaderElectionResourceLock: "leases",
		LeaderElectionID:           fmt.Sprintf("%s-manager", common.DellReplicationController),
	})
	return mgr
}

func (suite *PersistentVolumeControllerTestSuite) getWorkQueueTypeLimiter() workqueue.TypedRateLimiter[reconcile.Request] {
	lim := workqueue.DefaultTypedItemBasedRateLimiter[reconcile.Request]()
	return lim
}

func (suite *PersistentVolumeControllerTestSuite) initReconciler() {
	fakeRecorder := record.NewFakeRecorder(100)
	// Initialize the annotations & labels
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)

	suite.reconciler = &PersistentVolumeReconciler{
		Client:            suite.client,
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:            utils.Scheme,
		EventRecorder:     fakeRecorder,
		DriverName:        suite.driver.DriverName,
		ReplicationClient: suite.repClient,
		Domain:            constants.DefaultDomain,
		ContextPrefix:     utils.ContextPrefix,
		ClusterUID:        "test-clusterid",
	}
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcile() {
	ctx := context.Background()
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)

	err := suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)

	res, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
	suite.Equal(false, res.Requeue, "Requeue should be set to false")
	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, req.NamespacedName, updatedPV)
	suite.NoError(err)
	utils.ValidateAnnotations(updatedPV.ObjectMeta.Annotations, suite.T())
	suite.Equal("yes", updatedPV.Annotations[controllers.PVProtectionComplete],
		"PV protection should be complete")
	rgName := updatedPV.Annotations[controllers.ReplicationGroup]
	// Check if RG was created
	var rg repv1.DellCSIReplicationGroup
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: rgName}, &rg)
	suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileWithTransientError() {
	ctx := context.Background()
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)

	err := suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)
	// Inject temporary error

	errorMsg := "failed to create storage protection group"
	suite.repClient.InjectErrorAutoClear(fmt.Errorf("%s", errorMsg))
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.EqualError(err, errorMsg)

	// Reconcile again
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)

	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, req.NamespacedName, updatedPV)
	suite.NoError(err)
	utils.ValidateAnnotations(updatedPV.ObjectMeta.Annotations, suite.T())
	suite.Equal("yes", updatedPV.Annotations[controllers.PVProtectionComplete],
		"PV protection should be complete")
	rgName := updatedPV.Annotations[controllers.ReplicationGroup]
	// Check if RG was created
	var rg repv1.DellCSIReplicationGroup
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: rgName}, &rg)
	suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestRGAdd() {
	ctx := context.Background()
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)

	err := suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)

	res, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
	suite.Equal(false, res.Requeue, "Requeue should be set to false")
	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, req.NamespacedName, updatedPV)
	suite.NoError(err)
	utils.ValidateAnnotations(updatedPV.ObjectMeta.Annotations, suite.T())
	suite.Equal("yes", updatedPV.Annotations[controllers.PVProtectionComplete],
		"PV protection should be complete")
	rgName := updatedPV.Annotations[controllers.ReplicationGroup]
	// Check if RG was created
	var rg repv1.DellCSIReplicationGroup
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: rgName}, &rg)
	suite.NoError(err)

	// Create another PG with the same SC
	newPVName := "fake-pv-1"
	anotherPVObj := suite.getFakePV(newPVName)
	req = suite.getTypicalReconcileRequest(newPVName)

	err = suite.client.Create(ctx, anotherPVObj)
	suite.NoError(err)

	res, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
	suite.Equal(false, res.Requeue, "Requeue should be set to false")

	newUpdatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, req.NamespacedName, newUpdatedPV)
	suite.NoError(err)
	utils.ValidateAnnotations(newUpdatedPV.ObjectMeta.Annotations, suite.T())
	suite.Equal("yes", newUpdatedPV.Annotations[controllers.PVProtectionComplete],
		"PV protection should be complete")
	newRGName := newUpdatedPV.Annotations[controllers.ReplicationGroup]
	// Check if this is the same as old rg name
	suite.Equal(rgName, newRGName, "PV should be added to same RG")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileDifferentDriver() {
	// Create SC with a different driver
	otherDriver := "some.other.driver"
	scObj := suite.getFakeStorageClass()
	scObj.Name = "different-sc"
	scObj.Provisioner = otherDriver
	err := suite.client.Create(context.Background(), scObj)
	suite.NoError(err)
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)
	pvObj.Spec.PersistentVolumeSource.CSI.Driver = otherDriver
	pvObj.Spec.StorageClassName = "different-sc"

	err = suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)

	res, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
	suite.Equal(false, res.Requeue, "Requeue should be set to false")
	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(context.Background(), req.NamespacedName, updatedPV)
	suite.NoError(err)
	suite.Equal(pvObj.Annotations, updatedPV.Annotations, "No updates to annotation")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileNonReplicationEnabledSC() {
	ctx := context.Background()
	anotherSc := "another-storage-class"
	scObj := utils.GetNonReplicationEnabledSC(suite.driver.DriverName, anotherSc)
	err := suite.client.Create(context.Background(), scObj)
	suite.NoError(err)
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)
	pvObj.Spec.StorageClassName = anotherSc

	err = suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)

	res, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
	suite.Equal(false, res.Requeue, "Requeue should be set to false")
	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, req.NamespacedName, updatedPV)
	suite.NoError(err)
	suite.Equal(pvObj.Annotations, updatedPV.Annotations, "No updates to annotation")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcilePowerMaxMetro() {
	ctx := context.Background()
	anotherSc := "another-storage-class"
	scObj := utils.GetReplicationEnabledSCWithMetroMode(suite.driver.DriverName, anotherSc, "RdfMode")
	err := suite.client.Create(context.Background(), scObj)
	suite.NoError(err)
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)
	pvObj.Spec.StorageClassName = anotherSc

	err = suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)

	res, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
	suite.Equal(false, res.Requeue, "Requeue should be set to false")
	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, req.NamespacedName, updatedPV)
	suite.NoError(err)
	suite.Equal(pvObj.Annotations, updatedPV.Annotations, "No updates to annotation")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcilePowerStoreMetro() {
	ctx := context.Background()
	anotherSc := "another-storage-class"
	scObj := utils.GetReplicationEnabledSCWithMetroMode(suite.driver.DriverName, anotherSc, "mode")
	err := suite.client.Create(context.Background(), scObj)
	suite.NoError(err)
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)
	pvObj.Spec.StorageClassName = anotherSc

	err = suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)

	res, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
	suite.Equal(false, res.Requeue, "Requeue should be set to false")
	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, req.NamespacedName, updatedPV)
	suite.NoError(err)
	suite.Equal(pvObj.Annotations, updatedPV.Annotations, "No updates to annotation")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileInvalidSC() {
	// scenario: Storage class has replication enabled but doesn't have remote storage class name
	ctx := context.Background()
	anotherSc := "another-storage-class"
	scObj := suite.getFakeStorageClass()
	scObj.Name = anotherSc

	delete(scObj.Parameters, controllers.StorageClassRemoteStorageClassParam)
	err := suite.client.Create(context.Background(), scObj)
	suite.NoError(err)
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)
	pvObj.Spec.StorageClassName = anotherSc

	err = suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)

	res, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
	suite.Equal(false, res.Requeue, "Requeue should be set to false")
	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, req.NamespacedName, updatedPV)
	suite.NoError(err)
	suite.Equal(pvObj.Annotations, updatedPV.Annotations, "No updates to annotation")
}

func (suite *PersistentVolumeControllerTestSuite) ValidateAnnotations(pv *corev1.PersistentVolume) {
	suite.Equal("", pv.Annotations[controllers.RemoteStorageClassAnnotation])
}

func (suite *PersistentVolumeControllerTestSuite) TestPVNotFoundScenario() {
	req := suite.getTypicalReconcileRequest("non-existent-pv")
	_, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
}

func (suite *PersistentVolumeControllerTestSuite) TestSCNotFoundScenario() {
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)
	pvObj.Spec.StorageClassName = "non-existent-sc"

	err := suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)
	req := suite.getTypicalReconcileRequest(pvName)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVCExistsScenario() {
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)
	pvObj.Spec.ClaimRef = &corev1.ObjectReference{Name: "fake-pvc12", Namespace: suite.driver.Namespace}

	err := suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc12", suite.driver.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = pvName

	err = suite.client.Create(context.Background(), pvcObj)
	suite.NoError(err)
	req := suite.getTypicalReconcileRequest(pvName)

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVProtectionCompleteAnnotationNotFound() {
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)

	annotations := map[string]string{}
	annotations[controllers.CreatedBy] = "dell-csi-replicator"
	annotations[controllers.ReplicationGroup] = "xyz"

	pvObj.Annotations = annotations

	err := suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pvName)

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error")
}

func (suite *PersistentVolumeControllerTestSuite) getParams() map[string]string {
	// creating fake PV to use with fake PVC
	volumeAttributes := map[string]string{
		"param1":                                 "val1",
		"param2":                                 "val2",
		path.Join(utils.ContextPrefix, "param3"): "val3",
	}
	return volumeAttributes
}

func (suite *PersistentVolumeControllerTestSuite) getFakePV(name string) *corev1.PersistentVolume {
	return utils.GetPVObj(name, "csivol", suite.driver.DriverName, suite.driver.StorageClass, suite.getParams())
}

func (suite *PersistentVolumeControllerTestSuite) getFakeStorageClass() *storagev1.StorageClass {
	// creating fake storage-class with replication params
	sc := utils.GetReplicationEnabledSC(suite.driver.DriverName, suite.driver.StorageClass, suite.driver.RemoteSCName,
		suite.driver.RemoteClusterID)
	return sc
}

func TestPersistentVolumeControllerTestSuite(t *testing.T) {
	testSuite := new(PersistentVolumeControllerTestSuite)
	suite.Run(t, testSuite)
}

func (suite *PersistentVolumeControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVSetupWithManager_Error() {
	ctx := context.Background()
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)

	err := suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	mgr := suite.getTypicalManagerManager()
	limiter := suite.getWorkQueueTypeLimiter()

	defaultGetManagerIndexField := getManagerIndexField
	defer func() {
		getManagerIndexField = defaultGetManagerIndexField
	}()

	getManagerIndexField = func(mgr ctrl.Manager, ctx context.Context) error {
		return errors.New("error in getManagerIndexField")
	}

	err = suite.reconciler.SetupWithManager(context.Background(), mgr, limiter, 1)
	suite.Error(err)
}

// func (suite *PersistentVolumeControllerTestSuite) TestPVSetupWithManager() {
// 	ctx := context.Background()
// 	pvName := utils.FakePVName
// 	pvObj := suite.getFakePV(pvName)

// 	err := suite.client.Create(ctx, pvObj)
// 	suite.NoError(err)

// 	mgr := suite.getTypicalManagerManager()
// 	limiter := suite.getWorkQueueTypeLimiter()

// 	defaultGetManagerIndexField := getManagerIndexField
// 	defer func() {
// 		getManagerIndexField = defaultGetManagerIndexField
// 	}()

// 	getManagerIndexField = func(mgr ctrl.Manager, ctx context.Context) error {
// 		return nil
// 	}

// 	err = suite.reconciler.SetupWithManager(context.Background(), mgr, limiter, 1)
// 	suite.Error(err)
// }

func (suite *PersistentVolumeControllerTestSuite) TestCreateProtectionGroupAndRG_Error() {
	ctx := context.Background()
	pvName := utils.FakePVName
	pvObj := suite.getFakePV(pvName)

	err := suite.client.Create(ctx, pvObj)
	suite.NoError(err)

	_, err = suite.reconciler.createProtectionGroupAndRG(ctx, "", map[string]string{})
	suite.NoError(err)
}
