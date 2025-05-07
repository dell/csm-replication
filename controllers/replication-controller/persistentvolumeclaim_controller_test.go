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

package replicationcontroller

import (
	"context"
	"strings"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/connection"
	fakeclient "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/dell/csm-replication/test/mocks"
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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
	suite.fakeConfig = mocks.New("sourceCluster", "remote-123")
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

// To create PV obj with PV name
func (suite *PVControllerTestSuite) getPV(name ...string) corev1.PersistentVolume {
	// Use provided name or default "fake-pv"
	pvName := "fake-pv"
	if len(name) > 0 && name[0] != "" {
		pvName = name[0]
	}
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
			Name: pvName,
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

// Modified TestRemoteReplication to use "fake-pv-remote-replication"
func (suite *PVControllerTestSuite) TestRemoteReplication() {
	ctx := context.Background()

	remoteClient, err := suite.fakeConfig.GetConnection("remote-123")
	assert.Nil(suite.T(), err)

	// Create a PV on the remote cluster with a unique name
	pvObj := suite.getPV("fake-pv-remote-replication")
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

	// Create a fake storage class with replication params
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

	// Create a PV in the local cluster for the fake PVC
	pvObj = suite.getPV("fake-pv-remote-replication")
	err = suite.client.Create(ctx, &pvObj)
	assert.NotNil(suite.T(), pvObj)

	annotations := make(map[string]string)
	annotations[controllers.RGSyncComplete] = "yes"
	annotations[controllers.RemoteReplicationGroup] = "fake-rg"
	annotations[controllers.RemoteClusterID] = "remote-123"
	annotations[controllers.ContextPrefix] = "csi-fake"

	// Create a fake replication resource group
	resourceGroup := repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "fake-rg",
			Annotations: annotations,
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

	// Run the fake replication manager; note the PV name parameter
	suite.runFakeRemoteReplicationManager(suite.fakeConfig, remoteClient)
}

func (suite *PVControllerTestSuite) TestSetupWithManagerRg() {
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second)
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

func TestPvcProtectionIsComplete(t *testing.T) {
	originalNewPredicateFuncs := newPredicateFuncs
	originalGetAnnotations := getAnnotations

	defer func() {
		newPredicateFuncs = originalNewPredicateFuncs
		getAnnotations = originalGetAnnotations
	}()

	type testCase struct {
		name        string
		annotations map[string]string
		setupMocks  func()
		expected    bool
	}

	testCases := []testCase{
		{
			name: "PVCProtectionComplete annotation is yes",
			annotations: map[string]string{
				controllers.PVCProtectionComplete: "yes",
			},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{
						controllers.PVCProtectionComplete: "yes",
					}
				}
			},
			expected: true,
		},
		{
			name: "PVCProtectionComplete annotation is no",
			annotations: map[string]string{
				controllers.PVCProtectionComplete: "no",
			},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{
						controllers.PVCProtectionComplete: "no",
					}
				}
			},
			expected: false,
		},
		{
			name:        "No PVCProtectionComplete annotation",
			annotations: map[string]string{},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{}
				}
			},
			expected: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			meta := &fakeClientObject{
				annotations: tt.annotations,
			}
			newPredicateFuncs = func(f func(client.Object) bool) predicate.Funcs {
				return predicate.Funcs{
					GenericFunc: func(e event.GenericEvent) bool {
						return f(e.Object)
					},
				}
			}
			predicateFunc := pvcProtectionIsComplete()
			result := predicateFunc.Generic(event.GenericEvent{Object: meta})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPersistentVolumeClaimReconciler_processRemotePVC(t *testing.T) {
	originalGetPersistentVolumeClaimUpdatePersistentVolumeClaim := getPersistentVolumeClaimUpdatePersistentVolumeClaim

	defer func() {
		getPersistentVolumeClaimUpdatePersistentVolumeClaim = originalGetPersistentVolumeClaimUpdatePersistentVolumeClaim
	}()

	getPersistentVolumeClaimUpdatePersistentVolumeClaim = func(_ connection.RemoteClusterClient, _ context.Context, _ *corev1.PersistentVolumeClaim) error {
		return nil
	}

	tests := []struct {
		name          string
		rClient       connection.RemoteClusterClient
		claim         *corev1.PersistentVolumeClaim
		remotePVCName string
		remotePVCNs   string
		remotePVName  string
		wantError     bool
	}{
		{
			name:    "Remote PVC already has the annotations set",
			rClient: nil,
			claim: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controllers.RemotePVC:          "fake-pvc",
						controllers.RemotePVCNamespace: "fake-ns",
						controllers.RemotePV:           "fake-pv",
					},
				},
			},
			remotePVCName: "fake-pvc",
			remotePVCNs:   "fake-ns",
			remotePVName:  "fake-pv",
			wantError:     false,
		},
		{
			name:    "Successfully updated remote PVC with annotation",
			rClient: nil,
			claim: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controllers.RemotePVC:          "",
						controllers.RemotePVCNamespace: "fake-ns",
						controllers.RemotePV:           "fake-pv",
					},
				},
			},
			remotePVCName: "fake-pvc",
			remotePVCNs:   "fake-ns",
			remotePVName:  "fake-pv",
			wantError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &PersistentVolumeClaimReconciler{}
			_, err := r.processRemotePVC(context.Background(), tt.rClient, tt.claim, tt.remotePVCName, tt.remotePVCNs, tt.remotePVName)
			if (err != nil) != tt.wantError {
				t.Errorf("PersistentVolumeClaim.processRemotePVC() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestPersistentVolumeClaimReconciler_processLocalPVC(t *testing.T) {
	originalGetPersistentVolumeClaimUpdate := getPersistentVolumeClaimUpdate
	originalGetPersistentVolumeClaimReconcilerEventf := getPersistentVolumeClaimReconcilerEventf

	defer func() {
		getPersistentVolumeClaimUpdate = originalGetPersistentVolumeClaimUpdate
		getPersistentVolumeClaimReconcilerEventf = originalGetPersistentVolumeClaimReconcilerEventf
	}()

	getPersistentVolumeClaimUpdate = func(_ *PersistentVolumeClaimReconciler, _ context.Context, _ *corev1.PersistentVolumeClaim) error {
		return nil
	}

	getPersistentVolumeClaimReconcilerEventf = func(_ *PersistentVolumeClaimReconciler, _ *corev1.PersistentVolumeClaim, _ string, _ string, _ string, _ string) {
	}

	type args struct {
		ctx                context.Context
		claim              *corev1.PersistentVolumeClaim
		remotePVCName      string
		remotePVCNs        string
		remotePVName       string
		remoteClusterID    string
		isRemotePVCUpdated bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "PVC sync complete for ClusterId",
			args: args{
				ctx: context.TODO(),
				claim: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							controllers.PVCSyncComplete:    "no",
							controllers.RemotePV:           "fake-pv",
							controllers.RemotePVC:          "fake-pvc",
							controllers.RemotePVCNamespace: "fake-ns",
						},
					},
				},
				remotePVName:       "remote-pv-name",
				remotePVCName:      "remote-pvc-name",
				remotePVCNs:        "remote-pvc-namespace",
				remoteClusterID:    "remote-cluster-id",
				isRemotePVCUpdated: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &PersistentVolumeClaimReconciler{}
			if err := r.processLocalPVC(tt.args.ctx, tt.args.claim, tt.args.remotePVName, tt.args.remotePVCName, tt.args.remotePVCNs, tt.args.remoteClusterID, tt.args.isRemotePVCUpdated); (err != nil) != tt.wantErr {
				t.Errorf("PersistentVolumeClaim.processLocalPVC() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func (suite *PVControllerTestSuite) TestAllowPVCCreationOnTarget_CreatesRemotePVC() {
	ctx := context.Background()
	remoteClient, err := suite.fakeConfig.GetConnection("remote-123")
	assert.Nil(suite.T(), err)
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)

	// Set up a remote PV in the target cluster with Phase=Available and a ClaimRef
	pvObj := suite.getPV("pv-create-target")
	pvObj.Status.Phase = corev1.VolumeAvailable
	pvObj.Spec.ClaimRef = &corev1.ObjectReference{
		Kind:       "PersistentVolumeClaim",
		Namespace:  suite.driver.Namespace,
		Name:       "allow-create-pvc",
		APIVersion: "v1",
	}
	if pvObj.Annotations == nil {
		pvObj.Annotations = make(map[string]string)
	}
	// Add required annotations so updatePVCAnnotations can fill PVC metadata
	pvObj.Annotations[controllers.ContextPrefix] = suite.driver.DriverName
	pvObj.Annotations[controllers.PVCProtectionComplete] = "yes"
	pvObj.Annotations[controllers.RemoteVolumeAnnotation] = pvObj.Name
	pvObj.Annotations[controllers.RemotePVCNamespace] = suite.driver.Namespace
	pvObj.Annotations[controllers.RemotePVC] = "allow-create-pvc-remote"
	// Add required labels for updatePVCLabels
	if pvObj.Labels == nil {
		pvObj.Labels = make(map[string]string)
	}
	pvObj.Labels[controllers.DriverName] = suite.driver.DriverName
	pvObj.Labels[controllers.ReplicationGroup] = "rg0"
	// Create the remote PV in the fake remote cluster
	err = remoteClient.CreatePersistentVolume(ctx, &pvObj)
	assert.Nil(suite.T(), err, "expected no error creating remote PV on target cluster")

	// Set up a local PVC in the source cluster
	pvcObj := utils.GetPVCObj("allow-create-pvc", suite.driver.Namespace, suite.driver.StorageClass)
	pvcAnnotations := make(map[string]string)
	pvcAnnotations[controllers.RemoteClusterID] = "remote-123"
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = pvObj.Name
	pvcObj.Annotations = pvcAnnotations
	// Add labels to local PVC so updatePVCLabels has values
	if pvcObj.Labels == nil {
		pvcObj.Labels = make(map[string]string)
	}
	pvcObj.Labels[controllers.DriverName] = suite.driver.DriverName
	pvcObj.Labels[controllers.ReplicationGroup] = "rg0"
	err = suite.client.Create(ctx, pvcObj)
	assert.Nil(suite.T(), err, "expected no error creating local PVC in source cluster")

	// Perform reconciliation with AllowPVCCreationOnTarget = true
	pvcReq := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: suite.driver.Namespace,
			Name:      "allow-create-pvc",
		},
	}
	fakeRecorder := record.NewFakeRecorder(100)
	externalReconcile := PersistentVolumeClaimReconciler{
		Client:                   suite.client,
		Log:                      ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:                   utils.Scheme,
		EventRecorder:            fakeRecorder,
		Config:                   suite.fakeConfig,
		AllowPVCCreationOnTarget: true,
	}
	res, err := externalReconcile.Reconcile(ctx, pvcReq)
	assert.Nil(suite.T(), err, "expected no error on PVC reconcile")
	assert.False(suite.T(), res.Requeue, "expected no requeue on successful reconcile")

	// Verify that the remote PVC was created in the remote cluster
	remotePVC, err := remoteClient.GetPersistentVolumeClaim(ctx, suite.driver.Namespace, "allow-create-pvc-remote")
	assert.Nil(suite.T(), err, "expected remote PVC to be created on target cluster")
	assert.Equal(suite.T(), "allow-create-pvc-remote", remotePVC.Name, "remote PVC should have correct name")
	assert.Equal(suite.T(), suite.driver.Namespace, remotePVC.Namespace, "remote PVC should have correct namespace")

	// Verify metadata on the created remote PVC
	assert.Equal(suite.T(), constants.DellReplicationController, remotePVC.Annotations[controllers.CreatedBy], "CreatedBy annotation")
	assert.Equal(suite.T(), pvObj.Name, remotePVC.Spec.VolumeName, "PVC Spec.VolumeName should be set to remote volume name")
	assert.Equal(suite.T(), pvObj.Name, remotePVC.Annotations[controllers.RemotePV], "RemotePV annotation")
	assert.Equal(suite.T(), "remote-123", remotePVC.Annotations[controllers.RemoteClusterID], "RemoteClusterID annotation")

	// Verify that StorageClassName is set correctly
	if assert.NotNil(suite.T(), remotePVC.Spec.StorageClassName, "StorageClassName should be set") {
		assert.Equal(suite.T(), suite.driver.StorageClass, *remotePVC.Spec.StorageClassName, "StorageClassName")
	}
	assert.Equal(suite.T(), suite.driver.StorageClass, remotePVC.Annotations[controllers.RemoteStorageClassAnnotation], "RemoteStorageClass annotation")

	// Verify name/namespace annotations
	assert.Equal(suite.T(), suite.driver.Namespace, remotePVC.Annotations[controllers.RemotePVCNamespace], "RemotePVCNamespace annotation")
	assert.Equal(suite.T(), "allow-create-pvc-remote", remotePVC.Annotations[controllers.RemotePVC], "RemotePVC annotation")

	// Verify labels on remote PVC
	assert.Equal(suite.T(), suite.driver.DriverName, remotePVC.Labels[controllers.DriverName], "DriverName label")
	assert.Equal(suite.T(), "remote-123", remotePVC.Labels[controllers.RemoteClusterID], "RemoteClusterID label")
	assert.Equal(suite.T(), "rg0", remotePVC.Labels[controllers.ReplicationGroup], "ReplicationGroup label")

	// Verify finalizer is added
	assert.Contains(suite.T(), remotePVC.Finalizers, controllers.ReplicationFinalizer, "Replication finalizer should be added")
}

func TestUpdatePVCLabels(t *testing.T) {
	// Create a fake PVC representing the original volume (with labels)
	original := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				controllers.DriverName:       "driverX",
				controllers.ReplicationGroup: "repGroup1",
			},
		},
	}
	// Create target PVC to receive labels
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Labels: make(map[string]string),
		},
	}

	// Call the function under test
	updatePVCLabels(pvc, original, "remote-456")

	// Verify that labels were applied correctly
	assert.Equal(t, "driverX", pvc.Labels[controllers.DriverName], "DriverName label")
	assert.Equal(t, "remote-456", pvc.Labels[controllers.RemoteClusterID], "RemoteClusterID label")
	assert.Equal(t, "repGroup1", pvc.Labels[controllers.ReplicationGroup], "ReplicationGroup label")
}

func TestUpdatePVCAnnotations(t *testing.T) {
	// Create a fake PersistentVolume with annotations and labels
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pv1",
			Namespace: "ns1", // namespace is required for name formatting
			Labels: map[string]string{
				controllers.ReplicationGroup: "rg-1",
			},
			Annotations: map[string]string{
				controllers.ContextPrefix:          "ctxPrefixVal",
				controllers.PVCProtectionComplete:  "complete",
				controllers.RemoteVolumeAnnotation: "remote-vol-id",
				controllers.RemotePVC:              "target-pvc",
				controllers.RemotePVCNamespace:     "target-ns",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: "sc1",
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeBound,
		},
	}
	// Create an empty PVC object
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: make(map[string]string),
			Labels:      make(map[string]string),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			// StorageClassName will be set by updatePVCAnnotations
		},
	}

	// Call the function under test
	updatePVCAnnotationsAndSpec(pvc, "remote-123", pv)

	// Verify that annotations and spec fields were set correctly
	assert.Equal(t, "ctxPrefixVal", pvc.Annotations[controllers.ContextPrefix], "ContextPrefix annotation")
	assert.Equal(t, "complete", pvc.Annotations[controllers.PVCProtectionComplete], "PVCProtectionComplete annotation")
	assert.Equal(t, constants.DellReplicationController, pvc.Annotations[controllers.CreatedBy], "CreatedBy annotation")
	assert.Equal(t, "remote-vol-id", pvc.Spec.VolumeName, "PVC Spec.VolumeName should be set to remote volume ID")
	assert.Equal(t, "remote-vol-id", pvc.Annotations[controllers.RemotePV], "RemotePV annotation")
	assert.Equal(t, "remote-123", pvc.Annotations[controllers.RemoteClusterID], "RemoteClusterID annotation")
	assert.Equal(t, "rg-1", pvc.Annotations[controllers.ReplicationGroup], "ReplicationGroup annotation")
	// StorageClassName is a pointer; compare the string value
	if pvc.Spec.StorageClassName == nil {
		t.Error("Expected StorageClassName to be set")
	} else {
		assert.Equal(t, "sc1", *pvc.Spec.StorageClassName, "RemoteStorageClass annotation")
	}
	assert.Equal(t, "sc1", pvc.Annotations[controllers.RemoteStorageClassAnnotation], "RemoteStorageClass annotation")
	// Name and Namespace of PVC should be set from annotations
	assert.Equal(t, "target-pvc", pvc.Name, "PVC Name should be set to RemotePVC")
	assert.Equal(t, "target-ns", pvc.Namespace, "PVC Namespace should be set to RemotePVCNamespace")
	assert.Equal(t, "target-ns", pvc.Annotations[controllers.RemotePVCNamespace], "RemotePVCNamespace annotation")
	assert.Equal(t, "target-pvc", pvc.Annotations[controllers.RemotePVC], "RemotePVC annotation")
	// The RemoteVolumeAnnotation should be overwritten with "ns1/pv1 annotations: ... labels: ..."
	expectedPrefix := "ns1/pv1"
	actualRVAnn := pvc.Annotations[controllers.RemoteVolumeAnnotation]
	if !strings.HasPrefix(actualRVAnn, expectedPrefix) {
		t.Errorf("RemoteVolumeAnnotation should start with \"%s\", got \"%s\"", expectedPrefix, actualRVAnn)
	}
}

func TestVerifyNamespaceExistence_NamespaceAlreadyExists(t *testing.T) {
	// Setup fake multi-cluster config and get remote client
	fakeConfig := mocks.New("sourceCluster", "remote-123")
	remoteClient, err := fakeConfig.GetConnection("remote-123")
	assert.NoError(t, err)

	ctx := context.Background()
	// Create a namespace on remote cluster beforehand
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "existing-ns"}}
	err = remoteClient.CreateNamespace(ctx, ns)
	assert.NoError(t, err)

	// Call VerifyNamespaceExistence should find existing namespace and not error
	err = VerifyAndCreateNamespace(ctx, remoteClient, "existing-ns")
	assert.NoError(t, err)
}

func TestVerifyNamespaceExistence_NamespaceCreated(t *testing.T) {
	ctx := context.Background()
	// Use the mock MultiClusterClient to get a fake remote client
	fakeConfig := mocks.New("sourceCluster", "remote-123")
	remoteClient, err := fakeConfig.GetConnection("remote-123")
	assert.NoError(t, err, "expected no error getting fake remote client")

	namespace := "test-namespace"

	_, err = remoteClient.GetNamespace(ctx, namespace)
	assert.Error(t, err, "expected error when getting non-existent namespace")

	// Call the function under test; it should create the namespace
	err = VerifyAndCreateNamespace(ctx, remoteClient, namespace)
	assert.NoError(t, err, "expected VerifyNamespaceExistence to succeed on create")
}
