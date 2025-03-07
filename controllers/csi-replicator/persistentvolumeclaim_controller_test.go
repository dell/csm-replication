/*
 Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"testing"
	"time"

	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	fakeclient "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	csireplication "github.com/dell/csm-replication/test/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type PVControllerTestSuite struct {
	suite.Suite
	driver    utils.Driver
	mockUtils *utils.MockUtils
}

// blank assignment to verify client.Client method implementations
var (
	_             client.Client = &fakeclient.Client{}
	PVCReconciler PersistentVolumeClaimReconciler
)

func (suite *PVControllerTestSuite) SetupTest() {
	suite.Init()
	suite.runFakeReplicationManager()
}

func (suite *PVControllerTestSuite) Init() {
	suite.driver = utils.Driver{
		DriverName:   "csi-fake",
		StorageClass: "fake-sc",
	}

	suite.mockUtils = &utils.MockUtils{
		Specs:                utils.Common{Namespace: "fake-ns"},
		FakeControllerClient: utils.GetFakeClient(),
	}
	ctx := context.Background()
	scObj := suite.getFakeStorageClass()
	err := suite.mockUtils.FakeControllerClient.Create(ctx, scObj)
	assert.Nil(suite.T(), err)
}

func (suite *PVControllerTestSuite) runFakeReplicationManager() {
	fakeRecorder := record.NewFakeRecorder(100)
	// Initialize the annotations & labels
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	mockReplicationClient := csireplication.NewFakeReplicationClient(utils.ContextPrefix)

	PVCReconciler = PersistentVolumeClaimReconciler{
		Client:            suite.mockUtils.FakeControllerClient,
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:            utils.Scheme,
		EventRecorder:     fakeRecorder,
		DriverName:        suite.driver.DriverName,
		ReplicationClient: &mockReplicationClient,
		Domain:            constants.DefaultDomain,
		ContextPrefix:     utils.ContextPrefix,
	}
}

func (suite *PVControllerTestSuite) TestVolumeCreationFail() {
	// scenario: CreateRemoteVolume should fail with an error
	var dummyBuffer string
	ctx := context.Background()
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv1"

	err := suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc1", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv1"

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	suite.NoError(err, "No error on PVC create")

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc1",
		},
	}
	logger := PVCReconciler.Log.WithValues("persistentvolumeclaim", req.NamespacedName)
	e := PVCReconciler.processClaimForRemoteVolume(context.WithValue(ctx, constants.LoggerContextKey, logger), &corev1.PersistentVolumeClaim{},
		&corev1.PersistentVolume{}, map[string]string{}, dummyBuffer)
	suite.Error(e, "CreateRemoteVolume failed with an error")
}

func (suite *PVControllerTestSuite) TestPVProcessingFailure() {
	// scenario: PV processing should fail with an error
	ctx := context.Background()
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv2"

	err := suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc2", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv2"

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc3",
		},
	}

	logger := PVCReconciler.Log.WithValues("persistentvolumeclaim", req.NamespacedName)
	e := PVCReconciler.processClaimForReplicationGroup(context.WithValue(ctx, constants.LoggerContextKey, logger), &corev1.PersistentVolumeClaim{},
		pvObj)
	suite.Error(e, "CreateRemoteVolume failed with an error")
}

func (suite *PVControllerTestSuite) TestRGCreationFailure() {
	// scenario: CreateRG should fail with an error
	ctx := context.Background()
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv3"

	err := suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc3", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv3"

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc3",
		},
	}

	logger := PVCReconciler.Log.WithValues("persistentvolumeclaim", req.NamespacedName)
	e := PVCReconciler.processClaimForReplicationGroup(context.WithValue(ctx, constants.LoggerContextKey, logger), &corev1.PersistentVolumeClaim{},
		pvObj)
	suite.Error(e, "CreateRemoteVolume failed with an error")
}

func (suite *PVControllerTestSuite) TestWithoutStorageClass() {
	// scenario: When there is no storage class created
	ctx := context.Background()
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv4"

	err := suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	sc := ""
	pvcObj := utils.GetPVCObj("fake-pvc4", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv4"
	pvcObj.Spec.StorageClassName = &sc

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc4",
		},
	}

	res, _ := PVCReconciler.Reconcile(context.Background(), req)
	suite.Equal(false, res.Requeue, "PVC Reconciliation failed when there is no storage class created")
}

func (suite *PVControllerTestSuite) TestWithReplicationDisabledSc() {
	// scenario: When the storage class is not replication enabled
	ctx := context.Background()
	scObj := suite.getFakeStorageClass()
	scObj.Name = "fake-sc-5"
	scObj.Parameters = map[string]string{}

	err := suite.mockUtils.FakeControllerClient.Create(ctx, scObj)
	assert.NoError(suite.T(), err)

	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv5"
	pvObj.Spec.StorageClassName = "fake-sc-5"

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc5", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	scName := "fake-sc-5"
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv5"
	pvcObj.Spec.StorageClassName = &scName

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc5",
		},
	}
	PVCReconciler.DriverName = suite.driver.DriverName
	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NotNil(suite.T(), res, "PVC reconcile should fail when the storage class is not replication enabled")
}

func (suite *PVControllerTestSuite) TestWithInvalidStorageClass() {
	// scenario: When there is no remote-storage class added in the storage class
	ctx := context.Background()
	scObj := suite.getFakeStorageClass()
	scObj.Name = "fake-sc-6"
	scObj.Parameters = map[string]string{
		"RdfGroup":           "2",
		"RdfMode":            "ASYNC",
		"RemoteRDFGroup":     "2",
		"RemoteSYMID":        "000000000002",
		"RemoteServiceLevel": "Bronze",
		"SRP":                "SRP_1",
		"SYMID":              "000000000001",
		"ServiceLevel":       "Bronze",
		"replication.storage.dell.com/isReplicationEnabled": "true",
		"replication.storage.dell.com/remoteClusterID":      "remote-123",
	}

	err := suite.mockUtils.FakeControllerClient.Create(ctx, scObj)
	assert.NoError(suite.T(), err)

	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv6"
	pvObj.Spec.StorageClassName = "fake-sc-6"

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc6", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv6"
	scName := "fake-sc-6"
	pvcObj.Spec.StorageClassName = &scName

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc6",
		},
	}
	PVCReconciler.DriverName = suite.driver.DriverName
	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NotNil(suite.T(), res, "PVC reconcile should fail with invalid storage class")
}

func (suite *PVControllerTestSuite) TestWithInvalidVolumeHandle() {
	// scenario: CreateRemoteVolume should fail with invalid volumeHandle
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv7"
	pvObj.Spec.CSI.VolumeHandle = ""

	err := suite.mockUtils.FakeControllerClient.Create(context.Background(), pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc7", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv7"

	err = suite.mockUtils.FakeControllerClient.Create(context.Background(), pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc7",
		},
	}

	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NotNil(suite.T(), res, "PVC reconcile should fail when the volume_handle is invalid")
}

func (suite *PVControllerTestSuite) TestWhenPVCNotInBound() {
	// scenario: When the PVC is not bound
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv8"

	err := suite.mockUtils.FakeControllerClient.Create(context.Background(), pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc8", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = ""
	pvcObj.Spec.VolumeName = "fake-pv8"

	err = suite.mockUtils.FakeControllerClient.Create(context.Background(), pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc8",
		},
	}
	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NotNil(suite.T(), res, "PVC reconcile should fail when the PVC is not yet bound")
}

func (suite *PVControllerTestSuite) TestInvalidVolumeName() {
	// scenario: When there is no volume name present in the PVC
	pvcObj := utils.GetPVCObj("fake-pvc9", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = ""
	pvcObj.Spec.VolumeName = ""

	err := suite.mockUtils.FakeControllerClient.Create(context.Background(), pvcObj)
	assert.NoError(suite.T(), err)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc9",
		},
	}

	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NotNil(suite.T(), res, "PVC reconcile should fail when there is no volume name")
}

func (suite *PVControllerTestSuite) TestReconcileWithInvalidObjects() {
	// scenario: When the resource names are not valid
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "",
			Name:      "",
		},
	}
	res, _ := PVCReconciler.Reconcile(context.Background(), req)
	suite.Equal(false, res.Requeue, "PVC reconcile should fail with invalid resource names")
}

func (suite *PVControllerTestSuite) TestWithInvalidDriver() {
	// scenario: When the provisioner name and driver name doesn't match
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc9",
		},
	}
	PVCReconciler.DriverName = ""
	res, _ := PVCReconciler.Reconcile(context.Background(), req)
	suite.Equal(false, res.Requeue, "Requeue should be set to false")
}

func (suite *PVControllerTestSuite) TestPVCReconciliation() {
	// scenario: Positive scenario
	ctx := context.Background()
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv10"

	annotations := make(map[string]string)
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":3000023,"volume_id":"pvc-d559bbfa-6612-4b57-a542-5ca64c9625fe","volume_context":{"RdfGroup":"2","RdfMode":"ASYNC","RemoteRDFGroup":"2","RemoteSYMID":"000000000002","RemoteServiceLevel":"Bronze","SRP":"SRP_1","SYMID":"000000000001","ServiceLevel":"Bronze","replication.storage.dell.com/remotePVRetentionPolicy":"delete","storage.dell.com/isReplicationEnabled":"true","storage.dell.com/remoteClusterID":"target","storage.dell.com/remoteStorageClassName":"hostpath-del"}}`
	annotations[controllers.ReplicationGroup] = "fake-rg"

	pvObj.Annotations = annotations

	err := suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc10", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv10"
	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc10",
		},
	}

	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NoError(suite.T(), err, "No error on PVC reconcile")
	suite.Equal(res.Requeue, false, "Requeue should be set to false")

	updatedPVC := new(corev1.PersistentVolumeClaim)
	err = suite.mockUtils.FakeControllerClient.Get(ctx, req.NamespacedName, updatedPVC)
	suite.NoError(err)

	suite.Equal(updatedPVC.Namespace, suite.mockUtils.Specs.Namespace, "They should be equal")
	utils.ValidateAnnotations(updatedPVC.ObjectMeta.Annotations, suite.T())
}

func (suite *PVControllerTestSuite) TestPVCReconciliationWithEmptyRG() {
	// scenario: Negative scenario when RG annotation is empty
	ctx := context.Background()
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv10"

	annotations := make(map[string]string)
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":3000023,"volume_id":"pvc-d559bbfa-6612-4b57-a542-5ca64c9625fe","volume_context":{"RdfGroup":"2","RdfMode":"ASYNC","RemoteRDFGroup":"2","RemoteSYMID":"000000000002","RemoteServiceLevel":"Bronze","SRP":"SRP_1","SYMID":"000000000001","ServiceLevel":"Bronze","replication.storage.dell.com/remotePVRetentionPolicy":"delete","storage.dell.com/isReplicationEnabled":"true","storage.dell.com/remoteClusterID":"target","storage.dell.com/remoteStorageClassName":"hostpath-del"}}`
	annotations[controllers.ReplicationGroup] = ""

	pvObj.Annotations = annotations

	err := suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc10", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv10"
	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc10",
		},
	}

	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NoError(suite.T(), err, "No error on PVC reconcile")
	suite.Equal(res.Requeue, true, "Requeue should be set to true")
}

func (suite *PVControllerTestSuite) TestPVCReconciliationWithMissingRGAnnotation() {
	// scenario: Negative scenario when RG annotation is not set on PV
	ctx := context.Background()
	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv10"

	annotations := make(map[string]string)
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":3000023,"volume_id":"pvc-d559bbfa-6612-4b57-a542-5ca64c9625fe","volume_context":{"RdfGroup":"2","RdfMode":"ASYNC","RemoteRDFGroup":"2","RemoteSYMID":"000000000002","RemoteServiceLevel":"Bronze","SRP":"SRP_1","SYMID":"000000000001","ServiceLevel":"Bronze","replication.storage.dell.com/remotePVRetentionPolicy":"delete","storage.dell.com/isReplicationEnabled":"true","storage.dell.com/remoteClusterID":"target","storage.dell.com/remoteStorageClassName":"hostpath-del"}}`

	pvObj.Annotations = annotations

	err := suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc10", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv10"
	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc10",
		},
	}

	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NoError(suite.T(), err, "No error on PVC reconcile")
	suite.Equal(res.Requeue, true, "Requeue should be set to true")
}

func (suite *PVControllerTestSuite) TestPVRetentionPolicyDelete() {
	// scenario: When the PV Retention policy is set to delete
	ctx := context.Background()
	scObj := suite.getFakeStorageClass()
	scObj.Name = "fake-sc-11"
	scObj.Parameters[controllers.RemotePVRetentionPolicy] = controllers.RemoteRetentionValueDelete

	err := suite.mockUtils.FakeControllerClient.Create(ctx, scObj)
	assert.NoError(suite.T(), err)

	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv11"
	pvObj.Spec.StorageClassName = "fake-sc-11"

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc11", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	scName := "fake-sc-11"
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv11"
	pvcObj.Spec.StorageClassName = &scName

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc11",
		},
	}
	PVCReconciler.DriverName = suite.driver.DriverName
	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NotNil(suite.T(), res, "PVC reconcile should fail when the storage class is not replication enabled")
}

func (suite *PVControllerTestSuite) TestUpdatingTerminatingObjects() {
	// scenario: This should fail to update the PV which is in terminating state
	ctx := context.Background()

	pvObj := suite.getFakePV()
	pvObj.Name = "fake-pv12"
	pvObj.Status.Phase = "Terminating"

	err := suite.mockUtils.FakeControllerClient.Create(ctx, pvObj)
	assert.NoError(suite.T(), err)

	// creating fake PVC with bound state
	pvcObj := utils.GetPVCObj("fake-pvc12", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv12"

	err = suite.mockUtils.FakeControllerClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.mockUtils.Specs.Namespace,
			Name:      "fake-pvc12",
		},
	}
	PVCReconciler.DriverName = suite.driver.DriverName
	res, err := PVCReconciler.Reconcile(context.Background(), req)
	assert.NotNil(suite.T(), res, "PVC reconcile should fail when the PV is in terminating state")
}

func (suite *PVControllerTestSuite) TestSetupWithManager() {
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second)
	err := PVCReconciler.SetupWithManager(mgr, expRateLimiter, 1)
	suite.Error(err, "Setup should fail when there is no manager")
}

func (suite *PVControllerTestSuite) getFakePV() *corev1.PersistentVolume {
	// creating fake PV to use with fake PVC
	volumeAttributes := map[string]string{
		"fake-CapacityGB":     "3.00",
		"RemoteSYMID":         "000000000002",
		"SRP":                 "SRP_1",
		"ServiceLevel":        "Bronze",
		"StorageGroup":        "csi-UDI-Bronze-SRP_1-SG-test-2-ASYNC",
		"VolumeContentSource": "",
		"storage.kubernetes.io/csiProvisionerIdentity": "1611934095007-8081-csi-fake",
	}
	pvObj := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fake-pv",
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           suite.driver.DriverName,
					VolumeHandle:     "csivol-",
					FSType:           "ext4",
					VolumeAttributes: volumeAttributes,
				},
			},
			StorageClassName: suite.driver.StorageClass,
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	return &pvObj
}

func (suite *PVControllerTestSuite) getParams() map[string]string {
	parameters := map[string]string{
		"RdfGroup":           "2",
		"RdfMode":            "ASYNC",
		"RemoteRDFGroup":     "2",
		"RemoteSYMID":        "000000000002",
		"RemoteServiceLevel": "Bronze",
		"SRP":                "SRP_1",
		"SYMID":              "000000000001",
		"ServiceLevel":       "Bronze",
		"replication.storage.dell.com/isReplicationEnabled":   "true",
		"replication.storage.dell.com/remoteClusterID":        "remote-123",
		"replication.storage.dell.com/remoteStorageClassName": suite.driver.StorageClass,
	}
	return parameters
}

func (suite *PVControllerTestSuite) getFakeStorageClass() *storagev1.StorageClass {
	// creating fake storage-class with replication params
	parameters := suite.getParams()
	scObj := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: suite.driver.StorageClass},
		Provisioner: "csi-fake",
		Parameters:  parameters,
	}
	return &scObj
}

func TestPVControllerTestSuite(t *testing.T) {
	testSuite := new(PVControllerTestSuite)
	suite.Run(t, testSuite)
}

func (suite *PVControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}
