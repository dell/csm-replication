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
	controller "github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/connection"
	fakeclient "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/dell/csm-replication/test/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type PVReconcileSuite struct {
	suite.Suite
	driver     utils.Driver
	mockUtils  *utils.MockUtils
	client     client.Client
	fakeConfig connection.MultiClusterClient
	reconciler *PersistentVolumeReconciler
}

// blank assignment to verify client.Client method implementations
var (
	_            client.Client = &fakeclient.Client{}
	PVReconciler PersistentVolumeReconciler
)

func (suite *PVReconcileSuite) SetupSuite() {
	suite.Init()
}

func (suite *PVReconcileSuite) Init() {
	_ = repv1.AddToScheme(scheme.Scheme)

	var obj []runtime.Object
	c, err := fakeclient.NewFakeClient(obj, nil, nil)
	suite.NoError(err)

	suite.mockUtils = &utils.MockUtils{
		FakeClient: c,
		Specs:      utils.Common{Namespace: "fake-ns"},
	}

	suite.driver = utils.GetDefaultDriver()
	suite.client = utils.GetFakeClient()
	suite.fakeConfig = mocks.New("sourceCluster", "remote-123")
	suite.initReconciler(suite.fakeConfig)

	// Initialize the annotations & labels
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
}

func (suite *PVReconcileSuite) runRemoteReplicationManager(fakeConfig connection.MultiClusterClient,
	remoteClient connection.RemoteClusterClient,
) {
	fakeRecorder := record.NewFakeRecorder(100)

	PVReconciler = PersistentVolumeReconciler{
		Client:        suite.mockUtils.FakeClient,
		Log:           ctrl.Log.WithName("controllers").WithName("PersistentVolume"),
		Scheme:        utils.Scheme,
		EventRecorder: fakeRecorder,
		Domain:        constants.DefaultDomain,
		Config:        fakeConfig,
	}

	// Create and add the PersistentVolume to the fake client
	localPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "fake-pv",
			Annotations: map[string]string{},
		},
	}
	suite.mockUtils.FakeClient.Create(context.TODO(), localPV)

	// scenario: Positive scenario
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "fake-pv",
		},
	}
	res, err := PVReconciler.Reconcile(context.Background(), req)
	assert.NoError(suite.T(), err, "No error on PV deletion reconcile")
	assert.Equal(suite.T(), res.Requeue, false, "Requeue should be set to false")

	// scenario: when there is no deletion time-stamp set
	objMap := suite.mockUtils.FakeClient.Objects
	for k, v := range objMap {
		if k.Name == "fake-pv" {
			pv := v.(*corev1.PersistentVolume)
			pv.DeletionTimestamp = nil
			break
		}
	}
	res, err = PVReconciler.Reconcile(context.Background(), req)
	assert.NoError(suite.T(), err, "No error on PV deletion reconcile")
	assert.Equal(suite.T(), res.Requeue, false, "Requeue should be set to false")

	for k, v := range objMap {
		if k.Name == "fake-pv" {
			pv := v.(*corev1.PersistentVolume)
			for _, s := range pv.Finalizers {
				assert.Equal(suite.T(), strings.Compare(s, controllers.ReplicationFinalizer), 0, "Finalizer added successfully")
			}
			break
		}
	}

	// scenario: processLocalPV should fail with an error
	logger := PVReconciler.Log.WithValues("persistentvolumeclaim")

	// Test case where PV has already been synced
	localPV.Annotations[controller.PVSyncComplete] = "yes"
	e := PVReconciler.processLocalPV(context.WithValue(context.TODO(), constants.LoggerContextKey, logger), localPV, "", "")
	assert.NoError(suite.T(), e, "PV has already been synced")

	// Test case where processLocalPV should fail with an error
	e = PVReconciler.processLocalPV(context.WithValue(context.TODO(), constants.LoggerContextKey, logger), &corev1.PersistentVolume{}, "", "")
	assert.Error(suite.T(), e, "Process local PV failed with an error")

	// scenario: process remotePV should fail with an error
	_, e = PVReconciler.processRemotePV(context.WithValue(context.TODO(), constants.LoggerContextKey, logger), remoteClient, &corev1.PersistentVolume{}, "xyz")
	assert.Error(suite.T(), e, "Process remote PV failed with an error")

	// scenario: process remotePV should work with no error
	_, e = PVReconciler.processRemotePV(context.WithValue(context.TODO(), constants.LoggerContextKey, logger), remoteClient, localPV, "")
	assert.NoError(suite.T(), e, "Process remote PV with no error")

	annotations := make(map[string]string)
	annotations[controllers.PVProtectionComplete] = "yes"
	annotations[controllers.RemotePV] = "fake-pv"
	annotations[controllers.RemoteClusterID] = "remote-123"
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":5369364480,"volume_id":"csi-KPC-pmax-a28d2d04ae-00000000000200A40","volume_context":{"CapacityGB":"5.00","RdfGroup":"4","RemoteRDFGroup":"4","ServiceLevel":"Bronze","StorageGroup":"csi-no-srp-sg-test-4-ASYNC","powermax/RdfMode":"ASYNC","powermax/RemoteSYMID":"000000000001","powermax/SYMID":"000000000002"}}`
	annotations[controllers.RemoteStorageClassAnnotation] = "fake-sc"
	annotations[controllers.ReplicationGroup] = "fake-rg"
	annotations[controllers.ContextPrefix] = "powermax"

	labels := make(map[string]string)
	labels[controllers.DriverName] = "fake-provisioner"

	// var finalizers []string
	finalizers := []string{controllers.ReplicationFinalizer}

	objMap = suite.mockUtils.FakeClient.Objects
	for k, v := range objMap {
		if k.Name == "fake-pv" {
			pv := v.(*corev1.PersistentVolume)
			pv.DeletionTimestamp = nil
			pv.Annotations = annotations
			pv.Labels = labels
			pv.Finalizers = finalizers
			pv.Spec.ClaimRef.Namespace = "fake-ns"
			break
		}
	}

	// scenario: remote PV exists
	res, err = PVReconciler.Reconcile(context.Background(), req)
	assert.NoError(suite.T(), err, "No error on PV deletion reconcile")
	assert.Equal(suite.T(), res.Requeue, false, "Requeue should be set to false")

	annotations[controllers.RemotePV] = "doesnotexist"
	objMap = suite.mockUtils.FakeClient.Objects
	for k, v := range objMap {
		if k.Name == "fake-pv" {
			pv := v.(*corev1.PersistentVolume)
			pv.DeletionTimestamp = nil
			pv.Annotations = annotations
			pv.Labels = labels
			pv.Finalizers = finalizers
			pv.Spec.ClaimRef.Namespace = "fake-ns"
			break
		}
	}

	// scenario: remote PV does not exist
	res, err = PVReconciler.Reconcile(context.Background(), req)
	assert.NoError(suite.T(), err, "No error on PV deletion reconcile")
	assert.Equal(suite.T(), res.Requeue, false, "Requeue should be set to false")

	annotations[controllers.RemoteVolumeAnnotation] = "invalidVolume"
	objMap = suite.mockUtils.FakeClient.Objects
	for k, v := range objMap {
		if k.Name == "fake-pv" {
			pv := v.(*corev1.PersistentVolume)
			pv.DeletionTimestamp = nil
			pv.Annotations = annotations
			pv.Labels = labels
			pv.Finalizers = finalizers
			pv.Spec.ClaimRef.Namespace = "fake-ns"
			break
		}
	}

	// scenario: Negative case where failed to marshal json for remote volume
	res, err = PVReconciler.Reconcile(context.Background(), req)
	assert.Error(suite.T(), err, "Failed to unmarshal json for remote volume details")

	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":5369364480,"volume_id":"csi-KPC-pmax-a28d2d04ae-00000000000200A40","volume_context":{"CapacityGB":"5.00","RdfGroup":"4","RemoteRDFGroup":"4","ServiceLevel":"Bronze","StorageGroup":"csi-no-srp-sg-test-4-ASYNC","powermax/RdfMode":"ASYNC","powermax/RemoteSYMID":"000000000001","powermax/SYMID":"000000000002"}}`
	annotations[controllers.RemoteStorageClassAnnotation] = ""
	objMap = suite.mockUtils.FakeClient.Objects
	for k, v := range objMap {
		if k.Name == "fake-pv" {
			pv := v.(*corev1.PersistentVolume)
			pv.DeletionTimestamp = nil
			pv.Annotations = annotations
			pv.Labels = labels
			pv.Finalizers = finalizers
			pv.Spec.ClaimRef.Namespace = "fake-ns"
			break
		}
	}

	// scenario: Negative case failed to fetch remote storage class name
	res, err = PVReconciler.Reconcile(context.Background(), req)
	assert.Error(suite.T(), err, "storage class name missing from the remote volume annotation")

	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":5369364480,"volume_context":{"CapacityGB":"5.00","RdfGroup":"4","RemoteRDFGroup":"4","ServiceLevel":"Bronze","StorageGroup":"csi-no-srp-sg-test-4-ASYNC","powermax/RdfMode":"ASYNC","powermax/RemoteSYMID":"000000000001","powermax/SYMID":"000000000002"}}`
	annotations[controllers.RemoteStorageClassAnnotation] = "fake-sc"
	objMap = suite.mockUtils.FakeClient.Objects
	for k, v := range objMap {
		if k.Name == "fake-pv" {
			pv := v.(*corev1.PersistentVolume)
			pv.DeletionTimestamp = nil
			pv.Annotations = annotations
			pv.Labels = labels
			pv.Finalizers = finalizers
			pv.Spec.ClaimRef.Namespace = "fake-ns"
			pv.Spec.CSI.VolumeHandle = ""
			break
		}
	}

	// scenario: Negative case volume_id missing from the remote volume annotation
	res, err = PVReconciler.Reconcile(context.Background(), req)
	assert.Error(suite.T(), err, "volume_id missing from the remote volume annotation")
}

func (suite *PVReconcileSuite) TestReconcilePV() {
	fakeConfig := mocks.New("sourceCluster", "remote-123")
	remoteClient, err := fakeConfig.GetConnection("remote-123")

	ctx := context.Background()
	// creating fake PV to use with our fake PVC
	volumeAttributes := map[string]string{
		"fake-CapacityGB":     "3.00",
		"RemoteSYMID":         "000000000002",
		"SRP":                 "SRP_1",
		"ServiceLevel":        "Bronze",
		"StorageGroup":        "csi-UDI-Bronze-SRP_1-SG-test-2-ASYNC",
		"VolumeContentSource": "",
	}
	annotations := make(map[string]string)
	annotations[controllers.RGSyncComplete] = "yes"
	annotations[controllers.RemoteReplicationGroup] = "fake-rg"
	annotations[controllers.RemoteClusterID] = "remote-123"
	annotations[controllers.ContextPrefix] = "csi-fake"
	annotations[controllers.RemotePV] = "fake-pv"
	annotations[controllers.RemotePVRetentionPolicy] = "delete"

	pvObj := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "fake-pv",
			Annotations: annotations,
			DeletionTimestamp: &metav1.Time{
				Time: time.Now(),
			},
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
			ClaimRef: &corev1.ObjectReference{
				Kind:            "PersistentVolumeClaim",
				Namespace:       suite.mockUtils.Specs.Namespace,
				Name:            "fake-pvc",
				UID:             "18802349-2128-43a8-8169-bbb1ca8a4c67",
				APIVersion:      "v1",
				ResourceVersion: "32776691",
			},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	err = suite.mockUtils.FakeClient.Create(ctx, &pvObj)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), pvObj)

	// we will also create PV on the remote cluster to simulate deletion
	err = remoteClient.CreatePersistentVolume(ctx, &pvObj)
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

	annotations = make(map[string]string)
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
	err = suite.mockUtils.FakeClient.Create(ctx, &resourceGroup)

	// Creating fake-pvc
	pvcObj := utils.GetPVCObj("fake-pvc", suite.mockUtils.Specs.Namespace, suite.driver.StorageClass)
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "fake-pv"

	err = suite.mockUtils.FakeClient.Create(ctx, pvcObj)
	assert.NotNil(suite.T(), pvcObj)

	suite.runRemoteReplicationManager(fakeConfig, remoteClient)
}

func (suite *PVReconcileSuite) TestRemoteClusterIDNotSet() {
	volAttributes := make(map[string]string)
	pvObj := utils.GetPVObj(suite.driver.PVName, "fakeHandle", suite.driver.DriverName, suite.driver.StorageClass, volAttributes)

	err := suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)

	req := suite.getTypicalRequest(suite.driver.PVName)

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err, "remoteClusterId not set")
}

func (suite *PVReconcileSuite) TestRemoteClusterIDSelfNotFound() {
	volAttributes := make(map[string]string)
	pvObj := utils.GetPVObj("fake-pv02", "fakeHandle", suite.driver.DriverName, suite.driver.StorageClass, volAttributes)

	annotations := make(map[string]string)
	annotations[controllers.RemoteClusterID] = controllers.Self
	pvObj.Annotations = annotations

	err := suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)

	req := suite.getTypicalRequest("fake-pv02")

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err, "clusterId - self not found")
}

func (suite *PVReconcileSuite) TestRGAnnotationNotFound() {
	volAttributes := make(map[string]string)
	pvObj := utils.GetPVObj("fake-pv03", "fakeHandle", suite.driver.DriverName, suite.driver.StorageClass, volAttributes)

	annotations := make(map[string]string)
	annotations[controllers.RemoteClusterID] = "remote-123"
	annotations[controllers.PVProtectionComplete] = "yes"
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":5369364480,"volume_id":"csi-KPC-pmax-a28d2d04ae-000000000001","volume_context":{"CapacityGB":"5.00","RdfGroup":"4","RemoteRDFGroup":"4","ServiceLevel":"Bronze","StorageGroup":"csi-no-srp-sg-test-4-ASYNC","powermax/RdfMode":"ASYNC","powermax/RemoteSYMID":"000000000001","powermax/SYMID":"000000000001"}}`
	annotations[controllers.RemoteStorageClassAnnotation] = suite.driver.RemoteSCName
	pvObj.Annotations = annotations

	var finalizers []string
	finalizers = append(finalizers, controllers.ReplicationFinalizer)
	pvObj.Finalizers = finalizers

	err := suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)

	req := suite.getTypicalRequest("fake-pv03")

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err, "failed to fetch local replication group name")
}

func (suite *PVReconcileSuite) TestRemoteSCDoesNotExist() {
	volAttributes := make(map[string]string)
	pvObj := utils.GetPVObj("fake-pv04", "fakeHandle", suite.driver.DriverName, suite.driver.StorageClass, volAttributes)

	annotations := make(map[string]string)
	annotations[controllers.RemoteClusterID] = "remote-123"
	annotations[controllers.PVProtectionComplete] = "yes"
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":5369364480,"volume_id":"csi-KPC-pmax-a28d2d04ae-000000000001","volume_context":{"CapacityGB":"5.00","RdfGroup":"4","RemoteRDFGroup":"4","ServiceLevel":"Bronze","StorageGroup":"csi-no-srp-sg-test-4-ASYNC","powermax/RdfMode":"ASYNC","powermax/RemoteSYMID":"000000000001","powermax/SYMID":"000000000001"}}`
	annotations[controllers.ReplicationGroup] = suite.driver.RGName
	annotations[controllers.RemoteStorageClassAnnotation] = "doesnotexist"
	pvObj.Annotations = annotations

	var finalizers []string
	finalizers = append(finalizers, controllers.ReplicationFinalizer)
	pvObj.Finalizers = finalizers

	err := suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)

	req := suite.getTypicalRequest("fake-pv04")

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err, "remote storage class doesn't exist")
}

func (suite *PVReconcileSuite) initReconciler(config connection.MultiClusterClient) {
	fakeRecorder := record.NewFakeRecorder(100)
	reconciler := PersistentVolumeReconciler{
		Client:        suite.client,
		Log:           ctrl.Log.WithName("controllers").WithName("PersistentVolume"),
		Scheme:        utils.Scheme,
		EventRecorder: fakeRecorder,
		Domain:        constants.DefaultDomain,
		Config:        config,
	}
	suite.reconciler = &reconciler
}

func (suite *PVReconcileSuite) getTypicalRequest(pvName string) reconcile.Request {
	pvReq := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: pvName,
		},
	}
	return pvReq
}

func TestPVReconcileSuite(t *testing.T) {
	testSuite := new(PVReconcileSuite)
	suite.Run(t, testSuite)
}

func (suite *PVReconcileSuite) TestPVNotFound() {
	req := suite.getTypicalRequest("no-pv")
	_, err := suite.reconciler.Reconcile(context.Background(), req)
	if err == nil {
		suite.NoError(err)
	}
}

func (suite *PVReconcileSuite) TestSetupWithManager() {
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second)
	err := PVReconciler.SetupWithManager(mgr, expRateLimiter, 1)
	assert.Error(suite.T(), err, "Setup should fail when there is no manager")
}

func TestPvProtectionIsComplete(t *testing.T) {
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
			name: "PVProtectionComplete annotation is yes",
			annotations: map[string]string{
				controllers.PVProtectionComplete: "yes",
			},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{
						controllers.PVProtectionComplete: "yes",
					}
				}
			},
			expected: true,
		},
		{
			name: "PVProtectionComplete annotation is no",
			annotations: map[string]string{
				controllers.PVProtectionComplete: "no",
			},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{
						controllers.PVProtectionComplete: "no",
					}
				}
			},
			expected: false,
		},
		{
			name:        "No PVProtectionComplete annotation",
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
			predicateFunc := pvProtectionIsComplete()
			result := predicateFunc.Generic(event.GenericEvent{Object: meta})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDeletionRequested(t *testing.T) {
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
			name: "DeletionRequested annotation is yes",
			annotations: map[string]string{
				controllers.DeletionRequested: "yes",
			},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{
						controllers.DeletionRequested: "yes",
					}
				}
			},
			expected: true,
		},
		{
			name: "DeletionRequested annotation is no",
			annotations: map[string]string{
				controllers.DeletionRequested: "no",
			},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{
						controllers.DeletionRequested: "no",
					}
				}
			},
			expected: false,
		},
		{
			name:        "No DeletionRequested annotation",
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
			predicateFunc := isDeletionRequested()
			result := predicateFunc.Generic(event.GenericEvent{Object: meta})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasDeletionTimestamp(t *testing.T) {
	originalNewPredicateFuncs := newPredicateFuncs
	originalGetDeletionTimestamp := getDeletionTimestamp

	defer func() {
		newPredicateFuncs = originalNewPredicateFuncs
		getDeletionTimestamp = originalGetDeletionTimestamp
	}()

	type testCase struct {
		name              string
		deletionTimestamp *metav1.Time
		setupMocks        func()
		expected          bool
	}

	testCases := []testCase{
		{
			name: "Has deletion timestamp",
			deletionTimestamp: func() *metav1.Time {
				t := metav1.NewTime(time.Now())
				return &t
			}(),
			setupMocks: func() {
				getDeletionTimestamp = func(_ client.Object) *metav1.Time {
					t := metav1.NewTime(time.Now())
					return &t
				}
			},
			expected: true,
		},
		{
			name:              "No deletion timestamp",
			deletionTimestamp: nil,
			setupMocks: func() {
				getDeletionTimestamp = func(_ client.Object) *metav1.Time {
					return nil
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
				deletionTimestamp: tt.deletionTimestamp,
			}
			newPredicateFuncs = func(f func(client.Object) bool) predicate.Funcs {
				return predicate.Funcs{
					GenericFunc: func(e event.GenericEvent) bool {
						return f(e.Object)
					},
				}
			}
			predicateFunc := hasDeletionTimestamp()
			result := predicateFunc.Generic(event.GenericEvent{Object: meta})
			assert.Equal(t, tt.expected, result)
		})
	}
}

type fakeClientObject struct {
	client.Object
	annotations       map[string]string
	deletionTimestamp *metav1.Time
}

func (f *fakeClientObject) GetAnnotations() map[string]string {
	return f.annotations
}

func (f *fakeClientObject) GetDeletionTimestamp() *metav1.Time {
	return f.deletionTimestamp
}

func TestGetAnnotations(t *testing.T) {
	type testCase struct {
		name        string
		annotations map[string]string
		expected    map[string]string
	}

	testCases := []testCase{
		{
			name: "Has annotations",
			annotations: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:        "No annotations",
			annotations: nil,
			expected:    nil,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			meta := &fakeClientObject{
				annotations: tt.annotations,
			}
			result := getAnnotations(meta)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDeletionTimestamp(t *testing.T) {
	type testCase struct {
		name              string
		deletionTimestamp *metav1.Time
		expected          *metav1.Time
	}

	fixedTime := metav1.NewTime(time.Date(2025, time.February, 28, 4, 1, 42, 0, time.UTC))

	testCases := []testCase{
		{
			name:              "Has deletion timestamp",
			deletionTimestamp: &fixedTime,
			expected:          &fixedTime,
		},
		{
			name:              "No deletion timestamp",
			deletionTimestamp: nil,
			expected:          nil,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			meta := &fakeClientObject{
				deletionTimestamp: tt.deletionTimestamp,
			}
			result := getDeletionTimestamp(meta)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func (suite *PVReconcileSuite) TestRemoteClientGetConnectionError() {
	volAttributes := make(map[string]string)
	pvObj := utils.GetPVObj("fake-pv-conn-err", "fakeHandle", suite.driver.DriverName, suite.driver.StorageClass, volAttributes)

	annotations := make(map[string]string)
	annotations[controllers.RemoteClusterID] = "nonexistent-cluster" // Use a cluster ID that will fail
	pvObj.Annotations = annotations

	err := suite.client.Create(context.Background(), pvObj)
	suite.NoError(err)

	req := suite.getTypicalRequest("fake-pv-conn-err")

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err, "should fail to get remote client")
}

func (suite *PVReconcileSuite) TestReconcilePVRemotePVNameEmpty() {
	fakeConfig := mocks.New("sourceCluster", "remote-123")
	_, err := fakeConfig.GetConnection("remote-123")
	suite.NoError(err)

	ctx := context.Background()
	pvObj := utils.GetPVObj("fake-pv-nopv", "fakeHandle", suite.driver.DriverName, suite.driver.StorageClass, map[string]string{})
	annotations := make(map[string]string)
	annotations[controllers.RemoteClusterID] = "remote-123"
	annotations[controllers.PVProtectionComplete] = "yes"
	annotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":5369364480,"volume_id":"csi-KPC-pmax-a28d2d04ae-000000000001","volume_context":{"CapacityGB":"5.00","RdfGroup":"4","RemoteRDFGroup":"4","ServiceLevel":"Bronze","StorageGroup":"csi-no-srp-sg-test-4-ASYNC","powermax/RdfMode":"ASYNC","powermax/RemoteSYMID":"000000000001","powermax/SYMID":"000000000001"}}`
	annotations[controllers.ReplicationGroup] = suite.driver.RGName
	annotations[controllers.RemoteStorageClassAnnotation] = suite.driver.RemoteSCName
	annotations[controllers.RemotePV] = ""

	pvObj.Annotations = annotations

	err = suite.mockUtils.FakeClient.Create(ctx, pvObj)
	suite.NoError(err)
	req := suite.getTypicalRequest("fake-pv-nopv")

	_, err = suite.reconciler.Reconcile(ctx, req)
	suite.NoError(err)
}

func TestPersistentVolumeReconciler_processRemotePV(t *testing.T) {
	type args struct {
		ctx          context.Context
		rClient      connection.RemoteClusterClient
		remotePV     *corev1.PersistentVolume
		remoteRGName string
	}
	tests := []struct {
		name    string
		r       *PersistentVolumeReconciler
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "Remote RG name is empty",
			r:    &PersistentVolumeReconciler{},
			args: args{
				ctx:          context.TODO(),
				rClient:      nil,
				remotePV:     &corev1.PersistentVolume{},
				remoteRGName: "",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Remote RG annotation already set",
			r:    &PersistentVolumeReconciler{},
			args: args{
				ctx:     context.TODO(),
				rClient: nil,
				remotePV: &corev1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							controller.ReplicationGroup: "rg1",
						},
					},
				},
				remoteRGName: "rg1",
			},
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.r.processRemotePV(tt.args.ctx, tt.args.rClient, tt.args.remotePV, tt.args.remoteRGName)
			if (err != nil) != tt.wantErr {
				t.Errorf("PersistentVolumeReconciler.processRemotePV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PersistentVolumeReconciler.processRemotePV() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessLocalPV(t *testing.T) {
	originalGetPersistentVolumeReconcilerUpdate := getPersistentVolumeReconcilerUpdate
	originalGetPersistentVolumeReconcilerEventf := getPersistentVolumeReconcilerEventf

	defer func() {
		getPersistentVolumeReconcilerUpdate = originalGetPersistentVolumeReconcilerUpdate
		getPersistentVolumeReconcilerEventf = originalGetPersistentVolumeReconcilerEventf
	}()

	getPersistentVolumeReconcilerUpdate = func(_ *PersistentVolumeReconciler, _ context.Context, _ client.Object) error {
		// return errors.New("error in getPersistentVolumeReconcilerUpdate")
		return nil
	}

	getPersistentVolumeReconcilerEventf = func(_ *PersistentVolumeReconciler, _ runtime.Object, _ string, _ string, _ string, _ string) {
	}

	tests := []struct {
		name            string
		localPV         *corev1.PersistentVolume
		remotePVName    string
		remoteClusterID string
		wantErr         bool
	}{
		{
			name: "PV sync already complete",
			localPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controller.PVSyncComplete: "no",
						controller.RemotePV:       "",
					},
				},
			},
			remotePVName:    "remote-pv-name",
			remoteClusterID: "remote-cluster-id",
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &PersistentVolumeReconciler{}
			err := r.processLocalPV(context.Background(), tt.localPV, tt.remotePVName, tt.remoteClusterID)

			if (err != nil) != tt.wantErr {
				t.Errorf("PersistentVolumeReconciler.processLocalPV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
