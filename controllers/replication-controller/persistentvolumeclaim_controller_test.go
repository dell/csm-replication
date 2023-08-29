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

package replicationcontroller

import (
	"context"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csm-replication/pkg/connection"
	fakeclient "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
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
	driver     utils.Driver
	client     client.Client
	fakeConfig connection.MultiClusterClient
}

// blank assignment to verify client.Client method implementations
var (
	_                 client.Client = &fakeclient.Client{}
	externalReconcile PersistentVolumeClaimReconciler
)

func (suite *PVControllerTestSuite) SetupSuite() {
	suite.Init()
}

func (suite *PVControllerTestSuite) Init() {
	suite.driver = utils.GetDefaultDriver()
	suite.client = utils.GetFakeClient()
	suite.fakeConfig = config.New("sourceCluster", "remote-123")
}

func (suite *PVControllerTestSuite) getPV() corev1.PersistentVolume {
	// creating fake PV to use with our fake PVC
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

	return pvObj
}

func (suite *PVControllerTestSuite) runFakeRemoteReplicationManager(fakeConfig connection.MultiClusterClient, remoteClient connection.RemoteClusterClient) {
	fakeRecorder := record.NewFakeRecorder(100)
	// Initialize the annotations & labels
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)

	pvcObj := utils.GetPVCObj("fake-pvc", suite.driver.Namespace, suite.driver.StorageClass)

	pvcAnnotations := make(map[string]string)
	pvcAnnotations[controllers.RemoteClusterID] = "remote-123"

	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv"
	pvcObj.Annotations = pvcAnnotations
	err := suite.client.Create(context.Background(), pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	pvcReq := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.driver.Namespace,
			Name:      "fake-pvc",
		},
	}

	externalReconcile := PersistentVolumeClaimReconciler{
		Client:        suite.client,
		Log:           ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:        utils.Scheme,
		EventRecorder: fakeRecorder,
		Config:        fakeConfig,
	}

	res, err := externalReconcile.Reconcile(context.Background(), pvcReq)
	assert.NoError(suite.T(), err, "No error on PVC reconcile")
	assert.Equal(suite.T(), res.Requeue, false, "Requeue should be set to false")

	// scenario : idempotency
	res, err = externalReconcile.Reconcile(context.Background(), pvcReq)
	assert.NoError(suite.T(), err, "No error on PVC reconcile")
	assert.Equal(suite.T(), res.Requeue, false, "Requeue should be set to false")

	logger := externalReconcile.Log.WithValues("persistentvolumeclaim")

	remotePVC := &corev1.PersistentVolumeClaim{}
	remotePVCAnnotations := make(map[string]string)
	remotePVCAnnotations[controllers.RemotePVC] = ""
	remotePVCAnnotations[controllers.RemotePVCNamespace] = ""
	remotePVCAnnotations[controllers.RemotePV] = ""
	remotePVC.Annotations = remotePVCAnnotations

	// scenario: process Remote PVC should fail with an error
	_, e := externalReconcile.processRemotePVC(context.WithValue(context.TODO(), constants.LoggerContextKey, logger), remoteClient, remotePVC, "xyz", "xyz", "xyz")
	assert.Error(suite.T(), e, "Process remote PVC failed with an error")

	remotePVC1 := &corev1.PersistentVolumeClaim{}
	remotePVCAnnotations1 := make(map[string]string)
	remotePVCAnnotations1[controllers.PVCSyncComplete] = "no"
	remotePVCAnnotations1[controllers.RemotePVC] = ""
	remotePVCAnnotations1[controllers.RemotePVCNamespace] = ""
	remotePVCAnnotations1[controllers.RemotePV] = ""
	remotePVC1.Annotations = remotePVCAnnotations1

	// scenario: process local PVC should fail with an error
	e = externalReconcile.processLocalPVC(context.WithValue(context.TODO(), constants.LoggerContextKey, logger), remotePVC1, "xyz", "xyz", "xyz", "xyz", true)
	assert.Error(suite.T(), e, "Process remote PVC failed with an error")

	// scenario: remoteClusterId annotation is missing
	annotations := make(map[string]string)
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":8591769600, "volume_id":"csi-UDI-pmax-e0cc37cd2e-000000000002-00766", "volume_context":{"CapacityGB":"8.00","RemoteSYMID":"000000000002","StorageGroup":"csi-no-srp-sg-test-2-ASYNC"}}`
	annotations[controllers.RemoteStorageClassAnnotation] = "xyz"
	annotations[controllers.ReplicationGroup] = "rg-abc"

	pvc := utils.GetPVCObj("fake-pvc", suite.driver.Namespace, suite.driver.StorageClass)
	pvc.Annotations = annotations
	suite.client.Update(context.Background(), pvc)

	res, err = externalReconcile.Reconcile(context.Background(), pvcReq)
	assert.Error(suite.T(), err, "remoteClusterId not set")

	// scenario: Fails to get the remote client
	annotations = make(map[string]string)
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":8591769600, "volume_id":"csi-UDI-pmax-e0cc37cd2e-000000000002-00766", "volume_context":{"CapacityGB":"8.00","RemoteSYMID":"000000000002","StorageGroup":"csi-no-srp-sg-test-2-ASYNC"}}`
	annotations[controllers.RemoteStorageClassAnnotation] = "xyz"
	annotations[controllers.ReplicationGroup] = "rg-abc"
	annotations[controllers.RemoteClusterID] = "invalidId"

	pvc.Annotations = annotations
	suite.client.Update(context.Background(), pvc)

	res, err = externalReconcile.Reconcile(context.Background(), pvcReq)
	assert.Error(suite.T(), err, "error")

	// scenario : Remote PV annotation is not nil
	annotations = make(map[string]string)
	annotations[controllers.RemoteClusterID] = "remote-123"
	annotations[controllers.RemotePV] = "xyz"

	pvc.Annotations = annotations
	suite.client.Update(context.Background(), pvc)

	res, err = externalReconcile.Reconcile(context.Background(), pvcReq)
	assert.Error(suite.T(), err, "PersistentVolume xyz not found")

	fakePVCReq := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.driver.Namespace,
			Name:      "this-pvc-does-not-exist",
		},
	}
	res, err = externalReconcile.Reconcile(context.Background(), fakePVCReq)
	assert.NoError(suite.T(), err, "No error on reconcile")

	// scenario: process local PVC when PVC sync is complete
	remotePVC1.Annotations[controllers.PVCSyncComplete] = "yes"
	e = externalReconcile.processLocalPVC(context.WithValue(context.TODO(), constants.LoggerContextKey, logger), remotePVC1, "xyz", "xyz", "xyz", "xyz", true)
	assert.NoError(suite.T(), e, "No Error while processing local PVC")
}

func (suite *PVControllerTestSuite) TestRemoteReplication() {
	ctx := context.Background()

	remoteClient, err := suite.fakeConfig.GetConnection("remote-123")
	assert.Nil(suite.T(), err)

	pvObj := suite.getPV()
	pvObj.Status.Phase = corev1.VolumeBound
	pvObj.Spec.ClaimRef = &corev1.ObjectReference{
		Kind:            "PersistentVolumeClaim",
		Namespace:       suite.driver.Namespace,
		Name:            "fake-pvc",
		UID:             "18802349-2128-43a8-8169-bbb1ca8a4c67",
		APIVersion:      "v1",
		ResourceVersion: "32776691",
	}
	err = remoteClient.CreatePersistentVolume(context.Background(), &pvObj)
	assert.Nil(suite.T(), err)

	// creating fake storage-class with replication params
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
	scObj := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: suite.driver.StorageClass},
		Provisioner: "csi-fake",
		Parameters:  parameters,
	}
	err = suite.client.Create(ctx, &scObj)
	assert.Nil(suite.T(), err)

	// creating fake PV to use with our fake PVC
	pvObj = suite.getPV()
	err = suite.client.Create(ctx, &pvObj)
	assert.NotNil(suite.T(), pvObj)

	annotations := make(map[string]string)
	annotations[controllers.RGSyncComplete] = "yes"
	annotations[controllers.RemoteReplicationGroup] = "fake-rg"
	annotations[controllers.RemoteClusterID] = "remote-123"
	annotations[controllers.ContextPrefix] = "csi-fake"

	// creating fake resource group
	resourceGroup := repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "fake-rg",
			Annotations: annotations,
			// Namespace: suite.mockUtils.Specs.Namespace,
		},
		Spec: repv1.DellCSIReplicationGroupSpec{
			DriverName:                suite.driver.DriverName,
			RemoteClusterID:           "remote-123",
			ProtectionGroupAttributes: parameters,
			ProtectionGroupID:         "PG-1",
		},
		Status: repv1.DellCSIReplicationGroupStatus{
			State:       "",
			RemoteState: "",
			ReplicationLinkState: repv1.ReplicationLinkState{
				State:                "",
				LastSuccessfulUpdate: &metav1.Time{},
				ErrorMessage:         "",
			},
			LastAction: repv1.LastAction{
				Condition:    "",
				Time:         &metav1.Time{},
				ErrorMessage: "",
			},
		},
	}
	err = suite.client.Create(ctx, &resourceGroup)

	suite.runFakeRemoteReplicationManager(suite.fakeConfig, remoteClient)
}

func (suite *PVControllerTestSuite) TestSetupWithManagerRg() {
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 10*time.Second)
	err := externalReconcile.SetupWithManager(mgr, expRateLimiter, 1)
	assert.Error(suite.T(), err, "Setup should fail when there is no manager")
}

func TestPVControllerTestSuite(t *testing.T) {
	testSuite := new(PVControllerTestSuite)
	suite.Run(t, testSuite)
	testSuite.TearDownTestSuite()
}

func (suite *PVControllerTestSuite) TearDownTestSuite() {
	suite.T().Log("Cleaning up resources...")
}
