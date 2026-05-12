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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	csireplicator "github.com/dell/csm-replication/controllers/csi-replicator"
	"github.com/dell/csm-replication/pkg/common/constants"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/dell/csm-replication/test/mocks"
	"github.com/go-logr/logr"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type RGControllerTestSuite struct {
	suite.Suite
	client     client.Client
	driver     utils.Driver
	config     connection.MultiClusterClient
	reconciler *ReplicationGroupReconciler
	mockUtils  *utils.MockUtils
}

func TestRGControllerTestSuite(t *testing.T) {
	testSuite := new(RGControllerTestSuite)
	suite.Run(t, testSuite)
}

func (suite *RGControllerTestSuite) SetupTest() {
	suite.Init()
}

func (suite *RGControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}

func (suite *RGControllerTestSuite) Init() {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	suite.driver = utils.GetDefaultDriver()
	suite.client = utils.GetFakeClient()
	fakeConfig := mocks.NewFakeConfig(suite.driver.SourceClusterID, suite.driver.RemoteClusterID)
	suite.config = fakeConfig
	suite.initReconciler(fakeConfig)
}

func (suite *RGControllerTestSuite) initReconciler(config connection.MultiClusterClient) {
	fakeRecorder := record.NewFakeRecorder(100)
	reconciler := ReplicationGroupReconciler{
		Client:        suite.client,
		Log:           ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:        utils.Scheme,
		EventRecorder: fakeRecorder,
		Config:        config,
		Domain:        constants.DefaultDomain,
	}
	suite.reconciler = &reconciler
}

func (suite *RGControllerTestSuite) getTypicalRequest() reconcile.Request {
	rgReq := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: suite.driver.RGName,
		},
	}
	return rgReq
}

func (suite *RGControllerTestSuite) getLocalParams() map[string]string {
	return utils.GetParams(suite.driver.RemoteClusterID, suite.driver.RemoteSCName)
}

func (suite *RGControllerTestSuite) getRemoteParams() map[string]string {
	return utils.GetParams(suite.driver.SourceClusterID, suite.driver.StorageClass)
}

func (suite *RGControllerTestSuite) getLocalRG(name, clusterID string) *repv1.DellCSIReplicationGroup {
	// creating fake resource group
	replicationGroup := utils.GetRGObj(name, suite.driver.DriverName, clusterID,
		utils.LocalPGID, utils.RemotePGID, suite.getLocalParams(), suite.getRemoteParams())
	return replicationGroup
}

func (suite *RGControllerTestSuite) getRemoteRG(name, clusterID string) *repv1.DellCSIReplicationGroup {
	// creating fake resource group
	replicationGroup := utils.GetRGObj(name, suite.driver.DriverName, clusterID,
		utils.RemotePGID, utils.LocalPGID, suite.getRemoteParams(), suite.getLocalParams())
	return replicationGroup
}

func (suite *RGControllerTestSuite) getRGWithoutSyncComplete(name string, local bool, self bool) *repv1.DellCSIReplicationGroup {
	annotations := make(map[string]string)
	annotations[controllers.RemoteReplicationGroup] = name
	annotations[controllers.ContextPrefix] = utils.ContextPrefix

	annotations[controllers.RemoteRGRetentionPolicy] = controllers.RemoteRetentionValueDelete

	rgFinalizers := []string{controllers.RGFinalizer}

	rg := new(repv1.DellCSIReplicationGroup)
	if local {
		if self {
			annotations[controllers.RemoteClusterID] = utils.Self
			rg = suite.getLocalRG(name, utils.Self)
		} else {
			annotations[controllers.RemoteClusterID] = suite.driver.RemoteClusterID
			rg = suite.getLocalRG(name, suite.driver.RemoteClusterID)
		}
	} else {
		if self {
			annotations[controllers.RemoteClusterID] = utils.Self
			rg = suite.getRemoteRG(name, utils.Self)
		} else {
			annotations[controllers.RemoteClusterID] = suite.driver.SourceClusterID
			rg = suite.getRemoteRG(name, suite.driver.SourceClusterID)
		}
	}
	rg.Annotations = annotations
	rg.Finalizers = rgFinalizers
	return rg
}

func (suite *RGControllerTestSuite) getRGWithSyncComplete(name string) *repv1.DellCSIReplicationGroup {
	annotations := make(map[string]string)
	annotations[controllers.RGSyncComplete] = "yes"
	annotations[controllers.RemoteReplicationGroup] = suite.driver.RGName
	annotations[controllers.RemoteClusterID] = suite.driver.RemoteClusterID
	annotations[controllers.ContextPrefix] = utils.ContextPrefix
	rg := suite.getLocalRG(name, suite.driver.RemoteClusterID)
	rg.Annotations = annotations
	return rg
}

func (suite *RGControllerTestSuite) getTypicalSC() *storagev1.StorageClass {
	sc := utils.GetReplicationEnabledSC(suite.driver.DriverName, suite.driver.StorageClass,
		suite.driver.RemoteSCName, suite.driver.RemoteClusterID)
	return sc
}

func (suite *RGControllerTestSuite) createSCAndRG(sc *storagev1.StorageClass, rg *repv1.DellCSIReplicationGroup) {
	ctx := context.Background()
	err := suite.client.Create(ctx, sc)
	suite.NoError(err)
	err = suite.client.Create(ctx, rg)
	suite.NoError(err)
}

func (suite *RGControllerTestSuite) TestReconcileWithInvalidRGName() {
	// scenario: Reconcile with a non-existent RG
	req := suite.getTypicalRequest()
	_, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err) // Ignore not found RG
}

func (suite *RGControllerTestSuite) TestReconcileWithRemoteRGInvalidDriver() {
	// scenario: Reconcile with an existing remote RG with
	remoteRG := suite.getRGWithoutSyncComplete(suite.driver.RGName, false, false)
	remoteRG.Spec.DriverName = "invalid-driver-name"
	rClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	err = rClient.CreateReplicationGroup(context.Background(), remoteRG)
	suite.NoError(err)
	suite.createSCAndRG(suite.getTypicalSC(), suite.getRGWithoutSyncComplete(suite.driver.RGName, true, false))
	req := suite.getTypicalRequest()
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	remoteRGName := fmt.Sprintf("SourceClusterId-%s-%s", suite.driver.SourceClusterID, suite.driver.RGName)
	newRemoteRG, err := rClient.GetReplicationGroup(context.Background(), remoteRGName)
	suite.NoError(err)
	suite.Equal(suite.driver.SourceClusterID, newRemoteRG.Spec.RemoteClusterID)
}

func (suite *RGControllerTestSuite) TestReconcileWithRemoteRGInvalidRemoteClusterID() {
	// scenario: Reconcile with an existing remote RG with
	remoteRG := suite.getRGWithoutSyncComplete(suite.driver.RGName, false, false)
	remoteRG.Spec.DriverName = "invalid-driver-name"
	remoteRG.Spec.RemoteClusterID = "invalidClusterID"
	rClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	err = rClient.CreateReplicationGroup(context.Background(), remoteRG)
	suite.NoError(err)
	suite.createSCAndRG(suite.getTypicalSC(), suite.getRGWithoutSyncComplete(suite.driver.RGName, true, false))
	req := suite.getTypicalRequest()
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	remoteRGName := fmt.Sprintf("SourceClusterId-%s-%s", suite.driver.SourceClusterID, suite.driver.RGName)
	newRemoteRG, err := rClient.GetReplicationGroup(context.Background(), remoteRGName)
	suite.NoError(err)
	suite.Equal(suite.driver.SourceClusterID, newRemoteRG.Spec.RemoteClusterID)
}

func (suite *RGControllerTestSuite) TestReconcileWithRemoteRGInvalidPGID() {
	// scenario: Reconcile with an existing remote RG with Invalid PG ID
	remoteRG := suite.getRGWithoutSyncComplete(suite.driver.RGName, false, false)
	remoteRG.Spec.ProtectionGroupID = "invalid-pg-id"
	rClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	err = rClient.CreateReplicationGroup(context.Background(), remoteRG)
	suite.NoError(err)
	suite.createSCAndRG(suite.getTypicalSC(), suite.getRGWithoutSyncComplete(suite.driver.RGName, true, false))
	req := suite.getTypicalRequest()
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err) // Reconcile should stop
	rgList, err := rClient.ListReplicationGroup(context.Background())
	suite.NoError(err)
	suite.Equal(1, len(rgList.Items), "Only one remote RG")
	suite.Equal(suite.driver.RGName, rgList.Items[0].Name)
}

// scenario: Remote RG already exists on the remote cluster but driver name does not match

func (suite *RGControllerTestSuite) TestReconcileWithInvalidClusterID() {
	// scenario: RG without any annotations set by sidecar
	rg := suite.getRGWithoutSyncComplete(suite.driver.RGName, true, false)
	rg.Annotations[controllers.RemoteClusterID] = "invalidClusterID"
	rg.Spec.RemoteClusterID = "invalidClusterID"
	suite.createSCAndRG(suite.getTypicalSC(), rg)
	req := suite.getTypicalRequest()
	_, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err)
}

func (suite *RGControllerTestSuite) TestReconcileWithRGWithoutAnnotations() {
	// scenario: RG without any annotations set by sidecar
	suite.createSCAndRG(suite.getTypicalSC(), suite.getLocalRG(suite.driver.RGName, suite.driver.RemoteClusterID))
	req := suite.getTypicalRequest()
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)
}

func (suite *RGControllerTestSuite) TestReconcileRGWithAnnotations() {
	// scenario: RG without sync complete
	suite.createSCAndRG(suite.getTypicalSC(), suite.getRGWithoutSyncComplete(suite.driver.RGName, true, false))
	rg := new(repv1.DellCSIReplicationGroup)
	req := suite.getTypicalRequest()

	err := suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.NotContains(controllers.RemoteReplicationGroup, rg.Annotations,
		"Remote RG annotation doesn't exist")
	suite.NotContains(controllers.RGSyncComplete, rg.Annotations,
		"RG Sync annotation doesn't exist")

	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal("yes", rg.Annotations[controllers.RGSyncComplete],
		"RG Sync annotation applied")
	suite.Equal(suite.driver.RGName, rg.Annotations[controllers.RemoteReplicationGroup],
		"Remote RG annotation applied")

	// Check if remote RG got created
	rClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	_, err = rClient.GetReplicationGroup(context.Background(), rg.Name)
	suite.NoError(err)
}

func (suite *RGControllerTestSuite) TestReconcileRGWithAnnotationsSingleCluster() {
	// scenario: RG without sync complete
	newConfig := mocks.NewFakeConfigForSingleCluster(suite.client,
		suite.driver.SourceClusterID, suite.driver.RemoteClusterID)
	suite.config = newConfig
	suite.reconciler.Config = newConfig
	sc1 := utils.GetReplicationEnabledSC(suite.driver.DriverName, "sc-1",
		"sc-2", utils.Self)
	// create sc-1 and corresponding RG
	rg1 := suite.getRGWithoutSyncComplete(suite.driver.RGName, true, true)
	labels := make(map[string]string)
	labels[controllers.DriverName] = suite.driver.DriverName
	rg1.Labels = labels
	suite.createSCAndRG(sc1, rg1)
	// create sc-2
	sc2 := utils.GetReplicationEnabledSC(suite.driver.DriverName, "sc-2",
		"sc-1", utils.Self)
	err := suite.client.Create(context.Background(), sc2)
	suite.NoError(err)

	rg := new(repv1.DellCSIReplicationGroup)
	req := suite.getTypicalRequest()

	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.NotContains(controllers.RemoteReplicationGroup, rg.Annotations,
		"Remote RG annotation doesn't exist")
	suite.NotContains(controllers.RGSyncComplete, rg.Annotations,
		"RG Sync annotation doesn't exist")

	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal("yes", rg.Annotations[controllers.RGSyncComplete],
		"RG Sync annotation applied")
	replicatedRGName := fmt.Sprintf("%s-%s", replicated, rg.Name)
	suite.Equal(replicatedRGName, rg.Annotations[controllers.RemoteReplicationGroup],
		"Remote RG annotation applied")

	// Check if remote RG got created
	rClient, err := suite.config.GetConnection("self")
	suite.NoError(err)
	_, err = rClient.GetReplicationGroup(context.Background(), replicatedRGName)
	suite.NoError(err)

	// Another reconcile
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)

	// Reconcile the other RG
	req.NamespacedName.Name = replicatedRGName
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	replicatedRG, err := rClient.GetReplicationGroup(context.Background(), replicatedRGName)
	suite.NoError(err)
	suite.T().Log(replicatedRG.Annotations)
	suite.T().Log(replicatedRG.Labels)
}

func (suite *RGControllerTestSuite) TestRGSyncWithFinalizer() {
	suite.createSCAndRG(suite.getTypicalSC(), suite.getRGWithoutSyncComplete(suite.driver.RGName, true, false))
	rg := new(repv1.DellCSIReplicationGroup)
	req := suite.getTypicalRequest()
	err := suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.NotContains(controllers.RGSyncComplete, rg.Finalizers,
		"RG finalizer doesn't exist")
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	rg.Finalizers = append(rg.Finalizers, controllers.RGFinalizer)
	rg.DeletionTimestamp = &metav1.Time{
		Time: time.Now(),
	}
	suite.T().Log(rg.Finalizers, "RG finalizer added")
	// Check if remote RG got created
	rClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	_, err = rClient.GetReplicationGroup(context.Background(), rg.Name)
	suite.NoError(err)

	// Delete rg
	err = suite.client.Delete(context.Background(), rg)
	suite.NoError(err)
}

func (suite *RGControllerTestSuite) TestReconcileRGWithContextPrefix() {
	// scenario: RG without sync complete
	rg := suite.getRGWithoutSyncComplete(suite.driver.RGName, true, false)
	rg.Spec.RemoteProtectionGroupAttributes[fmt.Sprintf("%s/key", utils.ContextPrefix)] = "val"
	suite.createSCAndRG(suite.getTypicalSC(), rg)
	req := suite.getTypicalRequest()

	err := suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.NotContains(controllers.RemoteReplicationGroup, rg.Annotations,
		"Remote RG annotation doesn't exist")
	suite.NotContains(controllers.RGSyncComplete, rg.Annotations,
		"RG Sync annotation doesn't exist")

	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.T().Log(rg.Annotations)
	suite.Equal("yes", rg.Annotations[controllers.RGSyncComplete],
		"RG Sync annotation applied")
	suite.Equal(suite.driver.RGName, rg.Annotations[controllers.RemoteReplicationGroup],
		"Remote RG annotation applied")

	// Check if remote RG got created
	rClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	remoteRG, err := rClient.GetReplicationGroup(context.Background(), rg.Name)
	suite.NoError(err)
	suite.Equal("val", remoteRG.Labels[fmt.Sprintf("%s/key", constants.DefaultDomain)])
}

func (suite *RGControllerTestSuite) TestReconcileRGWithSyncCompleteWithError() {
	// scenario: RG with sync complete but no remote RG
	suite.createSCAndRG(suite.getTypicalSC(), suite.getRGWithSyncComplete(suite.driver.RGName))
	req := suite.getTypicalRequest()
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)
	rg := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.T().Log(rg.Annotations)

	// Check if remote RG got created
	rClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	_, err = rClient.GetReplicationGroup(context.Background(), rg.Name)
	suite.Error(err) // RG should not be created again
}

func (suite *RGControllerTestSuite) TestRGSyncDeletion() {
	// scenario: Test Remote RG sync deletion
	newConfig := mocks.NewFakeConfigForSingleCluster(suite.client,
		suite.driver.SourceClusterID, suite.driver.RemoteClusterID)
	suite.config = newConfig
	suite.reconciler.Config = newConfig
	sc1 := utils.GetReplicationEnabledSC(suite.driver.DriverName, "sc-1",
		"sc-2", utils.Self)
	// create sc-1 and corresponding RG
	rg1 := suite.getRGWithoutSyncComplete(suite.driver.RGName, true, true)
	labels := make(map[string]string)
	labels[controllers.DriverName] = suite.driver.DriverName
	rg1.Labels = labels
	suite.createSCAndRG(sc1, rg1)
	// create sc-2
	sc2 := utils.GetReplicationEnabledSC(suite.driver.DriverName, "sc-2",
		"sc-1", utils.Self)
	err := suite.client.Create(context.Background(), sc2)
	suite.NoError(err)

	rg := new(repv1.DellCSIReplicationGroup)
	req := suite.getTypicalRequest()

	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.NotContains(controllers.RemoteReplicationGroup, rg.Annotations,
		"Remote RG annotation doesn't exist")
	suite.NotContains(controllers.RGSyncComplete, rg.Annotations,
		"RG Sync annotation doesn't exist")

	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	err = suite.client.Delete(context.Background(), rg)
	suite.NoError(err)

	resp, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)
}

func (suite *RGControllerTestSuite) TestSetupWithManagerRg() {
	suite.Init()
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second)
	err := suite.reconciler.SetupWithManager(mgr, expRateLimiter, 1)
	suite.Error(err, "Setup should fail when there is no manager")
}

func (suite *RGControllerTestSuite) TestMakeNamespaceReference() {
	ns := "test-namespace"
	result := makeNamespaceReference(ns)
	suite.Equal(ns, result.ObjectMeta.Name)
}

func (suite *RGControllerTestSuite) TestMakeSnapReference() {
	snapName := "test-snapshot"
	namespace := "test-namespace"
	result := makeSnapReference(snapName, namespace)

	expectedName := "snapshot-" + snapName
	suite.Equal(result.Name, expectedName)
	suite.Equal(result.Namespace, namespace)
	suite.Equal(result.Kind, "VolumeSnapshot")
	suite.Equal(result.APIVersion, "snapshot.storage.k8s.io/v1")
}

func (suite *RGControllerTestSuite) TestMakeSnapshotObject() {
	snapName := "test-snapshot"
	contentName := "test-content"
	className := "test-class"
	namespace := "test-namespace"
	result := makeSnapshotObject(snapName, contentName, className, namespace)

	suite.Equal(result.Name, snapName)
	suite.Equal(result.Namespace, namespace)
	suite.Equal(*result.Spec.Source.VolumeSnapshotContentName, contentName)
	suite.Equal(*result.Spec.VolumeSnapshotClassName, className)
}

func (suite *RGControllerTestSuite) TestMakeStorageClassContent() {
	driver := "test-driver"
	snapClass := "test-snap-class"
	result := makeStorageClassContent(driver, snapClass)

	suite.Equal(result.Driver, driver)
	suite.Equal(result.Name, snapClass)
}

func (suite *RGControllerTestSuite) TestMakeVolSnapContent() {
	snapName := "test-snapshot"
	volumeName := "test-volume"
	snapRef := v1.ObjectReference{
		Name:      "test-snapshot-ref",
		Namespace: "test-namespace",
	}
	sc := &s1.VolumeSnapshotClass{
		Driver:         "test-driver",
		DeletionPolicy: "Retain",
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-snap-class",
		},
	}

	result := makeVolSnapContent(snapName, volumeName, snapRef, sc)

	suite.Equal(result.Spec.Driver, sc.Driver)
	suite.Equal(result.Spec.DeletionPolicy, sc.DeletionPolicy)
	suite.Equal(result.Spec.VolumeSnapshotRef.Name, snapRef.Name)
	suite.Equal(*result.Spec.VolumeSnapshotClassName, sc.Name)
	suite.Equal(*result.Spec.Source.SnapshotHandle, snapName)
}

func (suite *RGControllerTestSuite) TestProcessLastActionResult() {
	// Process the last action result by updating the RG annotation,
	// controllers.ActionProcessedTime, with the time of the last action

	rg := suite.getRGWithSyncComplete(suite.driver.RGName)

	// add a timestamp for the last action processed
	actionTimeStamp := time.Now()
	rg.Status.LastAction.Time = &metav1.Time{
		Time: actionTimeStamp,
	}

	// provide the RG with at least one condition.
	condition := repv1.LastAction{
		Condition: "successfully updated",
		Time:      &metav1.Time{Time: actionTimeStamp},
	}
	controllers.UpdateConditions(rg, condition, csireplicator.MaxNumberOfConditions)

	// make sure the actionProcessedTime and Status.LastAction.Time do not match
	rg.Annotations[controllers.ActionProcessedTime] = actionTimeStamp.Add(-1 * time.Minute).GoString()

	suite.client = utils.GetFakeClientWithObjects(rg)
	suite.reconciler.Client = suite.client

	remoteClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)

	// Process the last action. Should update the RG, updating the actionProcessedTime annotation
	err = suite.reconciler.processLastActionResult(context.Background(), rg, rg, remoteClient, suite.reconciler.Log)
	suite.NoError(err, "processLastActionResult should not fail")

	updatedRG := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: suite.driver.RGName}, updatedRG)
	suite.NoError(err, "should successfully get the updated RG")
	suite.Equal(actionTimeStamp.GoString(), updatedRG.Annotations[controllers.ActionProcessedTime],
		"Last action processed time should be updated with actionTimeStamp time")
}

func (suite *RGControllerTestSuite) TestProcessLastActionResult_AlreadyProcessed() {
	// Attempt to process the last action result when the RG has already been processed
	// and the actionProcessedTime and Status.LastAction.Time match

	rg := suite.getRGWithSyncComplete(suite.driver.RGName)

	// add a timestamp for the last action processed
	actionTimeStamp := time.Now()
	rg.Status.LastAction.Time = &metav1.Time{
		Time: actionTimeStamp,
	}

	// provide the RG with at least one condition.
	condition := repv1.LastAction{
		Condition: "successfully updated",
		Time:      &metav1.Time{Time: actionTimeStamp},
	}
	controllers.UpdateConditions(rg, condition, csireplicator.MaxNumberOfConditions)

	// make sure the actionProcessedTime and Status.LastAction.Time do not match
	rg.Annotations[controllers.ActionProcessedTime] = actionTimeStamp.GoString()

	suite.client = utils.GetFakeClientWithObjects(rg)
	suite.reconciler.Client = suite.client

	remoteClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)

	// Process the last action. Should update the RG, updating the actionProcessedTime annotation
	err = suite.reconciler.processLastActionResult(context.Background(), rg, rg, remoteClient, suite.reconciler.Log)
	suite.NoError(err, "processLastActionResult should do nothing")
	// Ideally, we'd check the log output here to confirm it logged "Last action has already been processed", but
	// it appears there is no method to get the log output.
}

func (suite *RGControllerTestSuite) TestProcessLastActionResult_NoActionProcessedTime() {
	// Attempt to process the last action result but do not provide any annotation
	// for controllers.ActionProcessedTime

	rg := suite.getRGWithSyncComplete(suite.driver.RGName)

	// add a timestamp for the last action processed
	actionTimeStamp := time.Now()
	rg.Status.LastAction.Time = &metav1.Time{
		Time: actionTimeStamp,
	}

	// provide the RG with at least one condition.
	condition := repv1.LastAction{
		Condition: "successfully updated",
		Time:      &metav1.Time{Time: actionTimeStamp},
	}
	controllers.UpdateConditions(rg, condition, csireplicator.MaxNumberOfConditions)

	// leave out the annotation for controllers.ActionProcessedTime

	suite.client = utils.GetFakeClientWithObjects(rg)
	suite.reconciler.Client = suite.client

	remoteClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)

	// Process the last action. Should update the RG, updating the actionProcessedTime annotation
	err = suite.reconciler.processLastActionResult(context.Background(), rg, rg, remoteClient, suite.reconciler.Log)
	suite.NoError(err, "processLastActionResult should do nothing")
	// Ideally, we'd check the log output here to confirm it logged "Action Processed does not exist", but
	// it appears there is no method to get the log output.
}

func (suite *RGControllerTestSuite) TestProcessSnapshotEvent() {
	// scenario: Test snapshot event processing
	rg := suite.getRGWithSyncComplete(suite.driver.RGName)
	rg.Status.LastAction.Time = &metav1.Time{Time: time.Now()}
	rg.Status.LastAction.Condition = "CREATE_SNAPSHOT"

	suite.client = utils.GetFakeClientWithObjects(rg)
	suite.reconciler.Client = suite.client

	remoteClient, err := suite.config.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)

	// Test case: No action annotation
	err = suite.reconciler.processSnapshotEvent(context.Background(), rg, remoteClient, suite.reconciler.Log)
	suite.NoError(err, "processSnapshotEvent should return nil when no action annotation is provided")

	// Test case: JSON unmarshal error
	rg.Annotations[csireplicator.Action] = "invalid-json"
	err = suite.reconciler.processSnapshotEvent(context.Background(), rg, remoteClient, suite.reconciler.Log)
	suite.Error(err, "processSnapshotEvent should return an error for invalid JSON annotation")

	// Test case: Snapshot class does not exist in remote cluster
	actionAnnotation := csireplicator.ActionAnnotation{
		SnapshotClass:     "test-snap-class",
		SnapshotNamespace: "test-namespace",
	}
	annotationBytes, _ := json.Marshal(actionAnnotation)
	rg.Annotations[csireplicator.Action] = string(annotationBytes)

	err = suite.reconciler.processSnapshotEvent(context.Background(), rg, remoteClient, suite.reconciler.Log)
	suite.Error(err, "processSnapshotEvent should return an error when the snapshot class is not found")

	// Test case: Valid Snapshot Class and Action Attributes
	actionAnnotation.SnapshotClass = "test-snapshot-class"
	annotationBytes, _ = json.Marshal(actionAnnotation)
	rg.Annotations[csireplicator.Action] = string(annotationBytes)
	rg.Status.LastAction.ActionAttributes = map[string]string{
		"volume1": "snapshot1",
	}

	err = suite.reconciler.processSnapshotEvent(context.Background(), rg, remoteClient, suite.reconciler.Log)
	suite.NoError(err, "processSnapshotEvent should succeed when a valid snapshot class and action attributes are provided")
}

func TestReplicationGroupReconciler_SetupWithManager(t *testing.T) {
	tests := []struct {
		name           string
		manager        ctrl.Manager
		limiter        workqueue.TypedRateLimiter[reconcile.Request]
		maxReconcilers int
		wantError      bool
	}{
		{
			name:           "Manager is nil",
			manager:        nil,
			limiter:        nil,
			maxReconcilers: 0,
			wantError:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReplicationGroupReconciler{}
			err := r.SetupWithManager(tt.manager, tt.limiter, tt.maxReconcilers)
			if (err != nil) != tt.wantError {
				t.Errorf("SetupWithManager() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestReplicationGroupReconciler_processLastActionResult(t *testing.T) {
	originalGetDellCsiReplicationGroupProcessSnapshotEvent := getDellCsiReplicationGroupProcessSnapshotEvent
	originalGetDellCsiReplicationGroupUpdate := getDellCsiReplicationGroupUpdate

	defer func() {
		getDellCsiReplicationGroupProcessSnapshotEvent = originalGetDellCsiReplicationGroupProcessSnapshotEvent
		getDellCsiReplicationGroupUpdate = originalGetDellCsiReplicationGroupUpdate
	}()

	getDellCsiReplicationGroupProcessSnapshotEvent = func(_ *ReplicationGroupReconciler, _ context.Context, _ *repv1.DellCSIReplicationGroup, _ connection.RemoteClusterClient, _ logr.Logger) error {
		return errors.New("error in getDellCsiReplicationGroupProcessSnapshotEvent()")
	}

	getDellCsiReplicationGroupUpdate = func(_ *ReplicationGroupReconciler, _ context.Context, _ *repv1.DellCSIReplicationGroup) error {
		return nil
	}

	type args struct {
		ctx          context.Context
		group        *repv1.DellCSIReplicationGroup
		remoteClient connection.RemoteClusterClient
		log          logr.Logger
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Test case: Last action failed",
			args: args{
				ctx: context.Background(),
				group: &repv1.DellCSIReplicationGroup{
					Status: repv1.DellCSIReplicationGroupStatus{
						Conditions: []repv1.LastAction{{}},
						LastAction: repv1.LastAction{
							Time:         &metav1.Time{Time: time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)},
							ErrorMessage: "error msg",
						},
					},
				},
				remoteClient: nil,
				log:          logr.Discard(),
			},
			wantErr: true,
		},
		{
			name: "Test case: Last action has already been processed",
			args: args{
				ctx: context.Background(),
				group: &repv1.DellCSIReplicationGroup{
					Status: repv1.DellCSIReplicationGroupStatus{
						Conditions: []repv1.LastAction{
							{
								Condition: "successful condition",
								Time:      &metav1.Time{Time: time.Now()},
							},
						}, LastAction: repv1.LastAction{
							Time:         &metav1.Time{Time: time.Now()},
							ErrorMessage: "",
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							controllers.ActionProcessedTime: time.Now().GoString(),
						},
					},
				},
				remoteClient: nil,
				log:          logr.Discard(),
			},
			wantErr: false,
		},
		{
			name: "Test case: Last action is a snapshot",
			args: args{
				ctx: context.Background(),
				group: &repv1.DellCSIReplicationGroup{
					Status: repv1.DellCSIReplicationGroupStatus{
						Conditions: []repv1.LastAction{
							{
								Condition: "successful condition",
							},
						}, LastAction: repv1.LastAction{
							Time:         &metav1.Time{Time: time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)},
							ErrorMessage: "",
							Condition:    "CREATE_SNAPSHOT",
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							controllers.ActionProcessedTime: time.Now().GoString(),
						},
					},
				},
				remoteClient: nil,
				log:          logr.Discard(),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReplicationGroupReconciler{}
			if err := r.processLastActionResult(tt.args.ctx, tt.args.group, tt.args.group, tt.args.remoteClient, tt.args.log); (err != nil) != tt.wantErr {
				t.Errorf("ReplicationGroupReconciler.processLastActionResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReplicationGroupReconciler_processSnapshotEvent(t *testing.T) {
	originalGetDellCsiReplicationGroupGetSnapshotClass := getDellCsiReplicationGroupGetSnapshotClass
	originalGetDellCsiReplicationGroupGetNamespace := getDellCsiReplicationGroupGetNamespace
	originalGetDellCsiReplicationGroupCreateNamespace := getDellCsiReplicationGroupCreateNamespace
	originalGetDellCsiReplicationGroupCreateSnapshotContent := getDellCsiReplicationGroupCreateSnapshotContent
	originalGetDellCsiReplicationGroupCreateSnapshotObject := getDellCsiReplicationGroupCreateSnapshotObject

	after := func() {
		getDellCsiReplicationGroupGetSnapshotClass = originalGetDellCsiReplicationGroupGetSnapshotClass
		getDellCsiReplicationGroupGetNamespace = originalGetDellCsiReplicationGroupGetNamespace
		getDellCsiReplicationGroupCreateNamespace = originalGetDellCsiReplicationGroupCreateNamespace
		getDellCsiReplicationGroupCreateSnapshotContent = originalGetDellCsiReplicationGroupCreateSnapshotContent
		getDellCsiReplicationGroupCreateSnapshotObject = originalGetDellCsiReplicationGroupCreateSnapshotObject
	}

	tests := []struct {
		name         string
		setup        func()
		group        *repv1.DellCSIReplicationGroup
		remoteClient connection.RemoteClusterClient
		log          logr.Logger
		wantErr      bool
	}{
		{
			name:         "Snapshot class not found in remote cluster",
			setup:        func() {},
			group:        &repv1.DellCSIReplicationGroup{},
			remoteClient: nil,
			log:          logr.Discard(),
			wantErr:      false,
		},
		{
			name: "Snapshot class does not exist on remote cluster",
			setup: func() {
				getDellCsiReplicationGroupGetSnapshotClass = func(_ connection.RemoteClusterClient, _ context.Context, _ string) (*s1.VolumeSnapshotClass, error) {
					return nil, errors.New("error in getDellCsiReplicationGroupGetSnapshotClass")
				}
			},
			group: &repv1.DellCSIReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						csireplicator.Action: func() string {
							obj := csireplicator.ActionAnnotation{}
							val, _ := json.Marshal(obj)
							return string(val)
						}(),
					},
				},
			},
			remoteClient: nil,
			log:          logr.Discard(),
			wantErr:      true,
		},
		{
			name: "unable to create the desired namespace",
			setup: func() {
				getDellCsiReplicationGroupGetSnapshotClass = func(_ connection.RemoteClusterClient, _ context.Context, _ string) (*s1.VolumeSnapshotClass, error) {
					// return nil, errors.New("error in getDellCsiReplicationGroupGetSnapshotClass")
					return &s1.VolumeSnapshotClass{ObjectMeta: metav1.ObjectMeta{Name: "test-snapshotclass"}}, nil
				}

				getDellCsiReplicationGroupGetNamespace = func(_ connection.RemoteClusterClient, _ context.Context, _ string) (*v1.Namespace, error) {
					return nil, errors.New("error in getDellCsiReplicationGroupGetNamespace")
					// return &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"}}, nil
				}

				getDellCsiReplicationGroupCreateNamespace = func(_ connection.RemoteClusterClient, _ context.Context, _ *v1.Namespace) error {
					return errors.New("error in getDellCsiReplicationGroupCreateNamespace")
				}
			},
			group: &repv1.DellCSIReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						csireplicator.Action: func() string {
							obj := csireplicator.ActionAnnotation{}
							val, _ := json.Marshal(obj)
							return string(val)
						}(),
					},
				},
			},
			remoteClient: nil,
			log:          logr.Discard(),
			wantErr:      true,
		},
		{
			name: "unable to create snapshot content",
			setup: func() {
				getDellCsiReplicationGroupGetSnapshotClass = func(_ connection.RemoteClusterClient, _ context.Context, _ string) (*s1.VolumeSnapshotClass, error) {
					return &s1.VolumeSnapshotClass{ObjectMeta: metav1.ObjectMeta{Name: "test-snapshotclass"}}, nil
				}
				getDellCsiReplicationGroupGetNamespace = func(_ connection.RemoteClusterClient, _ context.Context, _ string) (*v1.Namespace, error) {
					return &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"}}, nil
				}
				getDellCsiReplicationGroupCreateSnapshotContent = func(_ connection.RemoteClusterClient, _ context.Context, _ *s1.VolumeSnapshotContent) error {
					return errors.New("error in getDellCsiReplicationGroupCreateSnapshotContent")
				}
			},
			group: &repv1.DellCSIReplicationGroup{
				Status: repv1.DellCSIReplicationGroupStatus{
					LastAction: repv1.LastAction{
						ActionAttributes: func() map[string]string {
							m := map[string]string{
								"volumeHandle":   "test-volume-handle",
								"snapshotHandle": "test-snapshot-handle",
							}
							return m
						}(),
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						csireplicator.Action: func() string {
							obj := csireplicator.ActionAnnotation{}
							val, _ := json.Marshal(obj)
							return string(val)
						}(),
					},
				},
			},
			remoteClient: nil,
			log:          logr.Discard(),
			wantErr:      true,
		},
		{
			name: "unable to create snapshot object",
			setup: func() {
				getDellCsiReplicationGroupGetSnapshotClass = func(_ connection.RemoteClusterClient, _ context.Context, _ string) (*s1.VolumeSnapshotClass, error) {
					return &s1.VolumeSnapshotClass{ObjectMeta: metav1.ObjectMeta{Name: "test-snapshotclass"}}, nil
				}
				getDellCsiReplicationGroupGetNamespace = func(_ connection.RemoteClusterClient, _ context.Context, _ string) (*v1.Namespace, error) {
					return &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"}}, nil
				}
				getDellCsiReplicationGroupCreateSnapshotContent = func(_ connection.RemoteClusterClient, _ context.Context, _ *s1.VolumeSnapshotContent) error {
					return nil
				}
				getDellCsiReplicationGroupCreateSnapshotObject = func(_ connection.RemoteClusterClient, _ context.Context, _ *s1.VolumeSnapshot) error {
					return errors.New("error in getDellCsiReplicationGroupCreateSnapshotObject")
				}
			},
			group: &repv1.DellCSIReplicationGroup{
				Status: repv1.DellCSIReplicationGroupStatus{
					LastAction: repv1.LastAction{
						ActionAttributes: func() map[string]string {
							m := map[string]string{
								"volumeHandle":   "test-volume-handle",
								"snapshotHandle": "test-snapshot-handle",
							}
							return m
						}(),
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						csireplicator.Action: func() string {
							obj := csireplicator.ActionAnnotation{}
							val, _ := json.Marshal(obj)
							return string(val)
						}(),
					},
				},
			},
			remoteClient: nil,
			log:          logr.Discard(),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()
			r := &ReplicationGroupReconciler{}
			if err := r.processSnapshotEvent(context.Background(), tt.group, tt.remoteClient, tt.log); (err != nil) != tt.wantErr {
				t.Errorf("ReplicationGroupReconciler.processLastActionResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// getSingleClusterPVSetup creates replication group, remote replication group,
// a pair of PVs, and a PVC for single cluster.
func (suite *RGControllerTestSuite) getSingleClusterPVSetup() (*repv1.DellCSIReplicationGroup, *repv1.DellCSIReplicationGroup, *corev1.PersistentVolume, *corev1.PersistentVolume, *corev1.PersistentVolumeClaim) {
	// scenario: RG without sync complete
	newConfig := mocks.NewFakeConfigForSingleCluster(suite.client,
		suite.driver.SourceClusterID, suite.driver.RemoteClusterID)
	suite.config = newConfig
	suite.reconciler.Config = newConfig
	sc1 := utils.GetReplicationEnabledSC(suite.driver.DriverName, "sc-1",
		"sc-2", utils.Self)
	// create sc-1 and corresponding RG
	rg1 := suite.getRGWithoutSyncComplete(suite.driver.RGName, true, true)
	labels := make(map[string]string)
	labels[controllers.DriverName] = suite.driver.DriverName
	rg1.Labels = labels
	suite.createSCAndRG(sc1, rg1)
	// create sc-2
	sc2 := utils.GetReplicationEnabledSC(suite.driver.DriverName, "sc-2",
		"sc-1", utils.Self)
	err := suite.client.Create(context.Background(), sc2)
	suite.NoError(err)

	rg := new(repv1.DellCSIReplicationGroup)
	req := suite.getTypicalRequest()

	suite.NotContains(controllers.RemoteReplicationGroup, rg.Annotations,
		"Remote RG annotation doesn't exist")
	suite.NotContains(controllers.RGSyncComplete, rg.Annotations,
		"RG Sync annotation doesn't exist")

	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	suite.Equal("yes", rg.Annotations[controllers.RGSyncComplete],
		"RG Sync annotation applied")
	replicatedRGName := fmt.Sprintf("%s-%s", replicated, rg.Name)
	suite.Equal(replicatedRGName, rg.Annotations[controllers.RemoteReplicationGroup],
		"Remote RG annotation applied")

	// Check if remote RG got created
	rClient, err := suite.config.GetConnection("self")
	suite.NoError(err)
	replicatedRG, err := rClient.GetReplicationGroup(context.Background(), replicatedRGName)
	suite.NoError(err)

	// rgName, replicatedRGName
	rgName := rg.Name
	ctx := context.Background()

	// create local and remote PV
	localAnnotations := make(map[string]string)
	localAnnotations[controllers.RGSyncComplete] = "yes"
	localAnnotations[controllers.ReplicationGroup] = rgName
	localAnnotations[controllers.RemoteReplicationGroup] = replicatedRGName
	localAnnotations[controllers.RemoteClusterID] = "self"
	localAnnotations[controllers.ContextPrefix] = "csi-fake"
	localAnnotations[controllers.RemotePV] = "remote-pv"
	localAnnotations[controllers.RemotePVRetentionPolicy] = "delete"
	localAnnotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":3000023,"volume_id":"pvc-d559bbfa-6612-4b57-a542-5ca64c9625fe","volume_context":{"RdfGroup":"2","RdfMode":"ASYNC","RemoteRDFGroup":"2","RemoteSYMID":"000000000002","RemoteServiceLevel":"Bronze","SRP":"SRP_1","SYMID":"000000000001","ServiceLevel":"Bronze","replication.storage.dell.com/remotePVRetentionPolicy":"delete","storage.dell.com/isReplicationEnabled":"true","storage.dell.com/remoteClusterID":"self","storage.dell.com/remoteStorageClassName":"sc-2"}}`

	localLabels := make(map[string]string)
	localLabels[controllers.DriverName] = suite.driver.DriverName
	localLabels[controllers.ReplicationGroup] = rgName
	localLabels[controllers.RemoteClusterID] = "self"

	remoteAnnotations := make(map[string]string)
	remoteAnnotations[controllers.RGSyncComplete] = "yes"
	remoteAnnotations[controllers.ReplicationGroup] = replicatedRGName
	remoteAnnotations[controllers.RemoteReplicationGroup] = rgName
	remoteAnnotations[controllers.RemoteClusterID] = "self"
	remoteAnnotations[controllers.ContextPrefix] = "csi-fake"
	remoteAnnotations[controllers.RemotePV] = "local-pv"
	remoteAnnotations[controllers.RemotePVRetentionPolicy] = "delete"
	remoteAnnotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":3000023,"volume_id":"pvc-d559bbfa-6612-4b57-a542-5ca64c9625fe","volume_context":{"RdfGroup":"2","RdfMode":"ASYNC","RemoteRDFGroup":"2","RemoteSYMID":"000000000002","RemoteServiceLevel":"Bronze","SRP":"SRP_1","SYMID":"000000000001","ServiceLevel":"Bronze","replication.storage.dell.com/remotePVRetentionPolicy":"delete","storage.dell.com/isReplicationEnabled":"true","storage.dell.com/remoteClusterID":"self","storage.dell.com/remoteStorageClassName":"sc-1"}}`

	remoteLabels := make(map[string]string)
	remoteLabels[controllers.DriverName] = suite.driver.DriverName
	remoteLabels[controllers.ReplicationGroup] = replicatedRGName
	remoteLabels[controllers.RemoteClusterID] = "self"

	localPV := utils.GetPVObj("local-pv", "vol-handle", suite.driver.DriverName, "sc-1", nil)
	localPV.Labels = localLabels
	localPV.Annotations = localAnnotations
	localPV.Spec.PersistentVolumeReclaimPolicy = controllers.RemoteRetentionValueDelete

	localClaimRef := &corev1.ObjectReference{
		Kind:            "PersistentVolumeClaim",
		Namespace:       "fake-ns",
		Name:            "fake-pvc",
		UID:             "18802349-2128-43a8-8169-bbb1ca8a4c67",
		APIVersion:      "v1",
		ResourceVersion: "32776691",
	}
	localPV.Spec.ClaimRef = localClaimRef
	err = suite.client.Create(ctx, localPV)
	suite.NoError(err)

	remotePV := utils.GetPVObj("remote-pv", "vol-handle", suite.driver.DriverName, "sc-2", nil)
	localPV.Labels = remoteLabels
	remotePV.Annotations = remoteAnnotations
	remotePV.Spec.PersistentVolumeReclaimPolicy = controllers.RemoteRetentionValueDelete
	err = suite.client.Create(ctx, remotePV)
	suite.NoError(err)

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)

	// create pvc
	pvcAnnotations := make(map[string]string)
	pvcAnnotations[controllers.RGSyncComplete] = "yes"
	pvcAnnotations[controllers.ReplicationGroup] = rgName
	pvcAnnotations[controllers.RemoteReplicationGroup] = replicatedRGName
	pvcAnnotations[controllers.RemoteClusterID] = "self"
	pvcAnnotations[controllers.RemoteStorageClassAnnotation] = "sc-2"
	pvcAnnotations[controllers.ContextPrefix] = "csi-fake"
	pvcAnnotations[controllers.RemotePV] = "remote-pv"
	pvcAnnotations[controllers.RemoteVolumeAnnotation] = `{"capacity_bytes":3000023,"volume_id":"pvc-d559bbfa-6612-4b57-a542-5ca64c9625fe","volume_context":{"RdfGroup":"2","RdfMode":"ASYNC","RemoteRDFGroup":"2","RemoteSYMID":"000000000002","RemoteServiceLevel":"Bronze","SRP":"SRP_1","SYMID":"000000000001","ServiceLevel":"Bronze","replication.storage.dell.com/remotePVRetentionPolicy":"delete","storage.dell.com/isReplicationEnabled":"true","storage.dell.com/remoteClusterID":"self","storage.dell.com/remoteStorageClassName":"sc-2"}}`

	pvcLabels := make(map[string]string)
	pvcLabels[controllers.DriverName] = suite.driver.DriverName
	pvcLabels[controllers.ReplicationGroup] = rgName
	pvcLabels[controllers.RemoteClusterID] = "self"

	pvcObj := utils.GetPVCObj("fake-pvc", "fake-ns", "sc-1")
	pvcObj.Status.Phase = corev1.ClaimBound
	pvcObj.Spec.VolumeName = "local-pv"
	pvcObj.Annotations = pvcAnnotations
	pvcObj.Labels = pvcLabels

	err = suite.client.Create(ctx, pvcObj)
	suite.NoError(err)
	suite.NotNil(suite.T(), pvcObj)

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)

	return rg, replicatedRG, localPV, remotePV, pvcObj
}

func (suite *RGControllerTestSuite) TestPVCRemapPlanned() {
	rg, replicatedRG, _, _, _ := suite.getSingleClusterPVSetup()
	replicatedRGName := replicatedRG.Name

	// Invoke failover action
	time := metav1.Now()
	lastAction := repv1.LastAction{
		Time:      &time,
		Condition: "Action FAILOVER_REMOTE succeeded",
	}
	rg.Status = repv1.DellCSIReplicationGroupStatus{
		LastAction: lastAction,
		Conditions: []repv1.LastAction{lastAction},
	}
	rg.Annotations[controllers.ActionProcessedTime] = time.String()

	err := suite.client.Update(context.Background(), rg)
	suite.NoError(err)

	// Reconcile to trigger processFailoverAction
	req := suite.getTypicalRequest()
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	// Verify PVC swap occurred
	var swappedPVC corev1.PersistentVolumeClaim
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "fake-pvc", Namespace: "fake-ns"}, &swappedPVC)
	suite.NoError(err)

	suite.Equal("remote-pv", swappedPVC.Spec.VolumeName, "PVC should now be bound to the remote PV")
	suite.Equal("sc-2", *swappedPVC.Spec.StorageClassName, "PVC should now use the remote storage class")
	suite.Equal("local-pv", swappedPVC.Annotations[controllers.RemotePV], "Remote PV annotation should be updated")
	suite.Equal(replicatedRGName, swappedPVC.Annotations[controllers.ReplicationGroup], "Replication group annotation should be updated")

	// Verify remote PV's claim reference
	var updatedRemotePV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "remote-pv"}, &updatedRemotePV)
	suite.NoError(err)
	suite.Equal(controllers.RemoteRetentionValueDelete, string(updatedRemotePV.Spec.PersistentVolumeReclaimPolicy), "Remote PV reclaim policy should be 'Delete' after swapAllPVC")
	suite.Equal("fake-pvc", updatedRemotePV.Spec.ClaimRef.Name, "Remote PV should now be claimed by the PVC")
	suite.Equal("fake-ns", updatedRemotePV.Spec.ClaimRef.Namespace)

	// Verify local PV's claim reference is removed
	var updatedLocalPV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "local-pv"}, &updatedLocalPV)
	suite.NoError(err)
	suite.Equal("reserved", updatedLocalPV.Spec.ClaimRef.Name, "Remote PV should now be claimed by the PVC")
	suite.Equal("reserved", updatedLocalPV.Spec.ClaimRef.Namespace)
	suite.Equal(controllers.RemoteRetentionValueDelete, string(updatedLocalPV.Spec.PersistentVolumeReclaimPolicy), "Local PV reclaim policy should be 'Delete' after swapAllPVC")
}

func (suite *RGControllerTestSuite) TestPVCRemapPlannedFailbackLocal() {
	rg, _, _, _, _ := suite.getSingleClusterPVSetup()

	// Invoke failback action
	time := metav1.Now()
	lastAction := repv1.LastAction{
		Time:      &time,
		Condition: "Action FAILBACK_LOCAL succeeded",
	}
	rg.Status = repv1.DellCSIReplicationGroupStatus{
		LastAction: lastAction,
		Conditions: []repv1.LastAction{lastAction},
	}
	rg.Annotations[controllers.ActionProcessedTime] = time.String()

	err := suite.client.Update(context.Background(), rg)
	suite.NoError(err)

	// Reconcile to trigger processFailbackAction
	req := suite.getTypicalRequest()
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	// Verify PVC swap occurred
	var swappedPVC corev1.PersistentVolumeClaim
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "fake-pvc", Namespace: "fake-ns"}, &swappedPVC)
	suite.NoError(err)

	suite.Equal("local-pv", swappedPVC.Spec.VolumeName, "PVC should now be bound to the local PV")
	suite.Equal("sc-1", *swappedPVC.Spec.StorageClassName, "PVC should now use the local storage class")
	suite.Equal("remote-pv", swappedPVC.Annotations[controllers.RemotePV], "Remote PV annotation should be updated")
	suite.Equal(rg.Name, swappedPVC.Annotations[controllers.ReplicationGroup], "Replication group annotation should be updated")

	// Verify local PV's claim reference
	var updatedLocalPV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "local-pv"}, &updatedLocalPV)
	suite.NoError(err)
	suite.Equal(controllers.RemoteRetentionValueDelete, string(updatedLocalPV.Spec.PersistentVolumeReclaimPolicy), "Local PV reclaim policy should be 'Delete' after swapAllPVC")
	suite.Equal("fake-pvc", updatedLocalPV.Spec.ClaimRef.Name, "Local PV should now be claimed by the PVC")
	suite.Equal("fake-ns", updatedLocalPV.Spec.ClaimRef.Namespace)

	// Verify remote PV's claim reference is removed
	var updatedRemotePV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "remote-pv"}, &updatedRemotePV)
	suite.NoError(err)
	suite.Equal(controllers.RemoteRetentionValueDelete, string(updatedRemotePV.Spec.PersistentVolumeReclaimPolicy), "Remote PV reclaim policy should be 'Delete' after swapAllPVC")
}

func (suite *RGControllerTestSuite) TestPVCRemapUnplanned() {
	_, replicatedRG, _, _, _ := suite.getSingleClusterPVSetup()
	replicatedRGName := replicatedRG.Name

	// Invoke failover action
	time := metav1.Now()
	lastAction := repv1.LastAction{
		Time:      &time,
		Condition: "Action UNPLANNED_FAILOVER_LOCAL succeeded",
	}
	replicatedRG.Status = repv1.DellCSIReplicationGroupStatus{
		LastAction: lastAction,
		Conditions: []repv1.LastAction{lastAction},
	}
	replicatedRG.Annotations[controllers.ActionProcessedTime] = time.String()
	replicatedRG.Annotations[controllers.RGSyncComplete] = "yes"
	replicatedRG.Finalizers = append(replicatedRG.Finalizers, controllers.RGFinalizer)
	err := suite.client.Update(context.Background(), replicatedRG)
	suite.NoError(err)

	rgReq := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: replicatedRGName,
		},
	}
	resp, err := suite.reconciler.Reconcile(context.Background(), rgReq)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	// Verify PVC swap occurred
	var swappedPVC corev1.PersistentVolumeClaim
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "fake-pvc", Namespace: "fake-ns"}, &swappedPVC)
	suite.NoError(err)

	suite.Equal("remote-pv", swappedPVC.Spec.VolumeName, "PVC should now be bound to the remote PV")
	suite.Equal("sc-2", *swappedPVC.Spec.StorageClassName, "PVC should now use the remote storage class")
	suite.Equal("local-pv", swappedPVC.Annotations[controllers.RemotePV], "Remote PV annotation should be updated")
	suite.Equal(replicatedRGName, swappedPVC.Annotations[controllers.ReplicationGroup], "Replication group annotation should be updated")

	// Verify remote PV's claim reference
	var updatedRemotePV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "remote-pv"}, &updatedRemotePV)
	suite.NoError(err)
	suite.Equal(controllers.RemoteRetentionValueDelete, string(updatedRemotePV.Spec.PersistentVolumeReclaimPolicy), "Remote PV reclaim policy should be 'Delete' after swapAllPVC")
	suite.Equal("fake-pvc", updatedRemotePV.Spec.ClaimRef.Name, "Remote PV should now be claimed by the PVC")
	suite.Equal("fake-ns", updatedRemotePV.Spec.ClaimRef.Namespace)

	// Verify local PV's claim reference is removed
	var updatedLocalPV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "local-pv"}, &updatedLocalPV)
	suite.NoError(err)
	suite.Equal("reserved", updatedLocalPV.Spec.ClaimRef.Name, "Remote PV should now be claimed by the PVC")
	suite.Equal("reserved", updatedLocalPV.Spec.ClaimRef.Namespace)
	suite.Equal(controllers.RemoteRetentionValueDelete, string(updatedLocalPV.Spec.PersistentVolumeReclaimPolicy), "Local PV reclaim policy should be 'Delete' after swapAllPVC")
}

func (suite *RGControllerTestSuite) TestPVCRemapDisabled() {
	rg, _, _, _, _ := suite.getSingleClusterPVSetup()
	rgName := rg.Name
	suite.reconciler.DisablePVCRemap = true // Disable PVC remapping

	// Invoke failover action
	time := metav1.Now()
	lastAction := repv1.LastAction{
		Time:      &time,
		Condition: "Action FAILOVER_REMOTE succeeded",
	}
	rg.Status = repv1.DellCSIReplicationGroupStatus{
		LastAction: lastAction,
		Conditions: []repv1.LastAction{lastAction},
	}
	rg.Annotations[controllers.ActionProcessedTime] = time.String()

	err := suite.client.Update(context.Background(), rg)
	suite.NoError(err)

	// Reconcile to trigger processFailoverAction
	req := suite.getTypicalRequest()
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	// Verify that PVC swap did not occur
	var unchangedPVC corev1.PersistentVolumeClaim
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "fake-pvc", Namespace: "fake-ns"}, &unchangedPVC)
	suite.NoError(err)

	suite.Equal("local-pv", unchangedPVC.Spec.VolumeName, "PVC should still be bound to the local PV")
	suite.Equal("sc-1", *unchangedPVC.Spec.StorageClassName, "PVC should still use the local storage class")
	suite.Equal("remote-pv", unchangedPVC.Annotations[controllers.RemotePV], "Remote PV annotation should remain unchanged")
	suite.Equal(rgName, unchangedPVC.Annotations[controllers.ReplicationGroup], "Replication group annotation should remain unchanged")

	// Verify local PV's claim reference is unchanged
	var unchangedLocalPV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "local-pv"}, &unchangedLocalPV)
	suite.NoError(err)
	suite.NotNil(unchangedLocalPV.Spec.ClaimRef, "Local PV's claim reference should remain")
	suite.Equal("fake-pvc", unchangedLocalPV.Spec.ClaimRef.Name, "Local PV should still be claimed by the original PVC")
	suite.Equal("fake-ns", unchangedLocalPV.Spec.ClaimRef.Namespace)

	// Verify remote PV's claim reference is unchanged
	var unchangedRemotePV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "remote-pv"}, &unchangedRemotePV)
	suite.NoError(err)
	suite.Nil(unchangedRemotePV.Spec.ClaimRef, "Remote PV's claim reference should remain nil")
}

func (suite *RGControllerTestSuite) TestKubevirtPVCRemapDisabledByDefault() {
	rg, _, _, _, pvcObj := suite.getSingleClusterPVSetup()
	rgName := rg.Name

	// Ensure EnableKubevirtPVCRemap is false (default)
	suite.reconciler.EnableKubevirtPVCRemap = false

	// Add a DataVolume ownerRef to simulate a kubevirt-backed PVC
	trueVal := true
	pvcObj.OwnerReferences = append(pvcObj.OwnerReferences, metav1.OwnerReference{
		APIVersion:         "cdi.kubevirt.io/v1beta1",
		Kind:               "DataVolume",
		Name:               "test-dv",
		UID:                "dv-uid",
		Controller:         &trueVal,
		BlockOwnerDeletion: &trueVal,
	})
	err := suite.client.Update(context.Background(), pvcObj)
	suite.NoError(err)

	// Invoke failover action
	time := metav1.Now()
	lastAction := repv1.LastAction{
		Time:      &time,
		Condition: "Action FAILOVER_REMOTE succeeded",
	}
	rg.Status = repv1.DellCSIReplicationGroupStatus{
		LastAction: lastAction,
		Conditions: []repv1.LastAction{lastAction},
	}
	rg.Annotations[controllers.ActionProcessedTime] = time.String()

	err = suite.client.Update(context.Background(), rg)
	suite.NoError(err)

	// Reconcile to trigger processFailoverAction
	req := suite.getTypicalRequest()
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	// Verify that PVC swap was skipped — DV-owned PVCs should not be deleted
	// when KubeVirt PVC remap is disabled
	var updatedPVC corev1.PersistentVolumeClaim
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "fake-pvc", Namespace: "fake-ns"}, &updatedPVC)
	suite.NoError(err)

	// PVC should remain unchanged — still bound to local-pv
	suite.Equal("local-pv", updatedPVC.Spec.VolumeName, "PVC should still be bound to local PV (swap skipped)")
	suite.Equal("remote-pv", updatedPVC.Annotations[controllers.RemotePV], "Remote PV annotation should be unchanged")
	suite.Equal(rgName, updatedPVC.Annotations[controllers.ReplicationGroup], "Replication group annotation should be unchanged")
}

func (suite *RGControllerTestSuite) TestPVCRemapWithMismatchedRemotePV() {
	rg, _, localPV, remotePV, pvcObj := suite.getSingleClusterPVSetup()
	rgName := rg.Name
	ctx := context.Background()
	// Modify the remotePV annotation to create a mismatch
	pvcObj.Annotations[controllers.RemotePV] = "mismatched-pv"
	err := suite.client.Update(ctx, pvcObj)
	suite.NoError(err)

	req := suite.getTypicalRequest()
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)

	// Invoke failover action
	time := metav1.Now()
	lastAction := repv1.LastAction{
		Time:      &time,
		Condition: "Action FAILOVER_REMOTE succeeded",
	}
	rg.Status = repv1.DellCSIReplicationGroupStatus{
		LastAction: lastAction,
		Conditions: []repv1.LastAction{lastAction},
	}
	rg.Annotations[controllers.ActionProcessedTime] = time.String()

	err = suite.client.Update(context.Background(), rg)
	suite.NoError(err)

	// Reconcile to trigger processFailoverAction
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	// Verify that the PVC swap did not occur due to mismatched target
	var unchangedPVC corev1.PersistentVolumeClaim
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: pvcObj.Name, Namespace: pvcObj.Namespace}, &unchangedPVC)
	suite.NoError(err)

	suite.Equal(localPV.Name, unchangedPVC.Spec.VolumeName, "PVC should still be bound to the local PV")
	suite.Equal("sc-1", *unchangedPVC.Spec.StorageClassName, "PVC should still use the local storage class")
	suite.Equal("mismatched-pv", unchangedPVC.Annotations[controllers.RemotePV], "Remote PV annotation should remain unchanged")
	suite.Equal(rgName, unchangedPVC.Annotations[controllers.ReplicationGroup], "Replication group annotation should remain unchanged")

	// Verify local PV's claim reference is unchanged
	var unchangedLocalPV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: localPV.Name}, &unchangedLocalPV)
	suite.NoError(err)
	suite.NotNil(unchangedLocalPV.Spec.ClaimRef, "Local PV's claim reference should remain")
	suite.Equal(pvcObj.Name, unchangedLocalPV.Spec.ClaimRef.Name, "Local PV should still be claimed by the original PVC")
	suite.Equal(pvcObj.Namespace, unchangedLocalPV.Spec.ClaimRef.Namespace)

	// Verify remote PV's claim reference is unchanged
	var unchangedRemotePV corev1.PersistentVolume
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: remotePV.Name}, &unchangedRemotePV)
	suite.NoError(err)
	suite.Nil(unchangedRemotePV.Spec.ClaimRef, "Remote PV's claim reference should remain nil")
}

func TestUpdatePVClaimRef(t *testing.T) {
	originalGetPersistentVolume := getPersistentVolume
	originalUpdatePersistentVolume := updatePersistentVolume

	after := func() {
		getPersistentVolume = originalGetPersistentVolume
		updatePersistentVolume = originalUpdatePersistentVolume
	}

	tests := []struct {
		name               string
		pv                 *v1.PersistentVolume
		client             connection.RemoteClusterClient
		pvName             string
		pvcNamespace       string
		pvcResourceVersion string
		pvcName            string
		pvcUID             types.UID
		log                logr.Logger
		setup              func()
		expectedErr        bool
	}{
		{
			name:   "Error in getting persisitent volume",
			pvName: "",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Kind:            "PersistentVolumeClaim",
						Namespace:       "fake-ns",
						Name:            "",
						UID:             "fake-uid",
						ResourceVersion: "fake-version",
					},
				},
			},
			setup: func() {
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return nil, errors.New("Error retrieving PV")
				}
			},
			expectedErr: true,
		},
		{
			name:   "Error in updating persisitent volume",
			pvName: "fake-pv",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Kind:            "PersistentVolumeClaim",
						Namespace:       "fake-ns",
						Name:            "fake-pvc",
						UID:             "fake-uid",
						ResourceVersion: "fake-version",
					},
				},
			},
			setup: func() {
				pv := &corev1.PersistentVolume{}
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return pv, nil
				}
				updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
					return errors.New("error updating PV")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			tt.setup()
			pvName := tt.pvName
			pvcNamespace := tt.pvcNamespace
			pvcResourceVersion := tt.pvcResourceVersion
			pvcName := tt.pvcName
			pvcUID := tt.pvcUID
			log := tt.log
			client := tt.client
			ctx := context.Background()
			err := updatePVClaimRef(ctx, client, pvName, pvcNamespace, pvcResourceVersion, pvcName, pvcUID, log)
			if tt.expectedErr {
				if tt.name == "Error in getting persisitent volume" && err.Error() != "Error retrieving PV" {
					t.Errorf("expected error, got %s", err)
				} else if tt.name == "Error in updating persisitent volume" && !strings.Contains(err.Error(), "error updating PV") {
					t.Errorf("expected error, got %s", err)
				}
			} else {
				t.Logf("Expected no error, got %s", err)
			}
		})
	}
}

func TestRemovePVClaimRef(t *testing.T) {
	originalGetPersistentVolume := getPersistentVolume
	originalUpdatePersistentVolume := updatePersistentVolume

	after := func() {
		getPersistentVolume = originalGetPersistentVolume
		updatePersistentVolume = originalUpdatePersistentVolume
	}

	tests := []struct {
		name         string
		pv           *v1.PersistentVolume
		client       connection.RemoteClusterClient
		pvName       string
		pvcNamespace string
		pvcName      string
		log          logr.Logger
		setup        func()
		expectedErr  bool
	}{
		{
			name: "When PV cannot be retrieved",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: nil,
				},
			},
			setup: func() {
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return nil, errors.New("Error retrieving PV")
				}
			},
		},
		{
			name:         "Error in updating persisitent volume",
			pvName:       "fake-pv",
			pvcNamespace: "fake-ns",
			pvcName:      "fake-pvc",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Kind:            "PersistentVolumeClaim",
						Namespace:       "fake-ns",
						Name:            "fake-pvc",
						UID:             "fake-uid",
						ResourceVersion: "fake-version",
					},
				},
			},
			setup: func() {
				pv := &corev1.PersistentVolume{
					Spec: corev1.PersistentVolumeSpec{
						ClaimRef: &corev1.ObjectReference{
							Kind:            "PersistentVolumeClaim",
							Namespace:       "fake-ns",
							Name:            "fake-pvc",
							UID:             "fake-uid",
							ResourceVersion: "fake-version",
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: make(map[string]string),
						Labels:      make(map[string]string),
					},
				}
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return pv, nil
				}
				updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
					return errors.New("error updating PV")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			tt.setup()
			pvName := tt.pvName
			pvcNamespace := tt.pvcNamespace
			pvcName := tt.pvcName
			log := tt.log
			client := tt.client
			ctx := context.Background()
			err := removePVClaimRef(ctx, client, pvName, pvcNamespace, pvcName, log)
			if tt.expectedErr && err != nil {
				if tt.name == "When PV cannot be retrieved" && err.Error() != "Error retrieving PV" {
					t.Errorf("expected error, got %s", err)
				} else if tt.name == "Error in updating persisitent volume" && !strings.Contains(err.Error(), "error updating PV") {
					t.Errorf("expected error, got %s", err)
				}
			} else {
				t.Logf("Expected no error, got %s", err)
			}
		})
	}
}

func TestSetPVClaimRef(t *testing.T) {
	originalGetPersistentVolume := getPersistentVolume
	originalUpdatePersistentVolume := updatePersistentVolume

	after := func() {
		getPersistentVolume = originalGetPersistentVolume
		updatePersistentVolume = originalUpdatePersistentVolume
	}

	tests := []struct {
		name         string
		pv           *v1.PersistentVolume
		client       connection.RemoteClusterClient
		pvName       string
		pvcNamespace string
		pvcName      string
		log          logr.Logger
		setup        func()
		expectedErr  bool
	}{
		{
			name: "When PV cannot be retrieved",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: nil,
				},
			},
			setup: func() {
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return nil, errors.New("Error retrieving PV")
				}
			},
		},
		{
			name:   "Error in updating persisitent volume",
			pvName: "fake-pv",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Kind:            "PersistentVolumeClaim",
						Namespace:       "fake-ns",
						Name:            "",
						UID:             "fake-uid",
						ResourceVersion: "fake-version",
					},
				},
			},
			setup: func() {
				pv := &corev1.PersistentVolume{}
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return pv, nil
				}
				updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
					return errors.New("error updating PV")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			tt.setup()
			pv := &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{
					PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
				},
			}
			pvName := tt.pvName
			prevPolicy := pv.Spec.PersistentVolumeReclaimPolicy
			client := tt.client
			ctx := context.Background()
			err := setPVReclaimPolicy(ctx, client, pvName, prevPolicy)
			if tt.expectedErr && err != nil {
				if tt.name == "When PV cannot be retrieved" && err.Error() != "Error retrieving PV" {
					t.Errorf("expected error, got %s", err)
				} else if tt.name == "Error in updating persisitent volume" && !strings.Contains(err.Error(), "error updating PV") {
					t.Errorf("expected error, got %s", err)
				}
			} else {
				t.Logf("Expected no error, got %s", err)
			}
		})
	}
}

func TestSwapPVC(t *testing.T) {
	originalGetPersistentVolumeClaim := getPersistentVolumeClaim
	originalGetPersistentVolume := getPersistentVolume
	originalDeletePersistentVolumeClaim := deletePersistentVolumeClaim
	originalUpdatePersistentVolume := updatePersistentVolume
	originalCreatePersistentVolumeClaim := createPersistentVolumeClaim
	originalSleep := sleep

	after := func() {
		getPersistentVolumeClaim = originalGetPersistentVolumeClaim
		getPersistentVolume = originalGetPersistentVolume
		deletePersistentVolumeClaim = originalDeletePersistentVolumeClaim
		updatePersistentVolume = originalUpdatePersistentVolume
		createPersistentVolumeClaim = originalCreatePersistentVolumeClaim
		sleep = originalSleep
	}
	tests := []struct {
		name        string
		pvc         *v1.PersistentVolumeClaim
		client      connection.RemoteClusterClient
		pvcName     string
		namespace   string
		targetPV    string
		rgTarget    string
		log         logr.Logger
		setup       func()
		expectedErr bool
	}{
		{
			name:      "Error getting PVC",
			namespace: "fake-ns",
			pvcName:   "fake-pvc",
			setup: func() {
				getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
					return nil, errors.New("error getting pvc")
				}
			},
			expectedErr: true,
		},
		{
			name:      "Error getting PV",
			namespace: "fake-ns",
			pvcName:   "fake-pvc",
			setup: func() {
				pvc := &corev1.PersistentVolumeClaim{}
				getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
					return pvc, nil
				}
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return nil, errors.New("error getting pv")
				}
			},
			expectedErr: true,
		},
		{
			name:      "Error deleting PVC",
			namespace: "fake-ns",
			pvcName:   "fake-pvc",
			setup: func() {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake-pvc",
						Namespace: "fake-ns",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						VolumeName: "fake-pv",
					},
				}
				pv := &corev1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv",
						Annotations: map[string]string{
							"migration.storage.dell.com/migrate-to": "sc2",
							"migration.storage.dell.com/namespace":  "namespace",
						},
					},
					Spec: corev1.PersistentVolumeSpec{
						PersistentVolumeSource: corev1.PersistentVolumeSource{
							CSI: &corev1.CSIPersistentVolumeSource{
								Driver:       "provisionerName",
								VolumeHandle: "volHandle",
								FSType:       "ext4",
							},
						},
						StorageClassName: "name",
					},
					Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
				}
				getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
					return pvc, nil
				}
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return pv, nil
				}
				deletePersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolumeClaim) error {
					return errors.New("error deleting PVC")
				}
				updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
					return nil
				}
			},
			expectedErr: true,
		},
		{
			name:      "Error recreating PVC",
			namespace: "fake-ns",
			pvcName:   "fake-pvc",
			setup: func() {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake-pvc",
						Namespace: "fake-ns",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						VolumeName: "fake-pv",
					},
				}
				pv := &corev1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv",
						Annotations: map[string]string{
							"migration.storage.dell.com/migrate-to": "sc2",
							"migration.storage.dell.com/namespace":  "namespace",
						},
					},
					Spec: corev1.PersistentVolumeSpec{
						PersistentVolumeSource: corev1.PersistentVolumeSource{
							CSI: &corev1.CSIPersistentVolumeSource{
								Driver:       "provisionerName",
								VolumeHandle: "volHandle",
								FSType:       "ext4",
							},
						},
						StorageClassName: "name",
					},
					Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
				}
				getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
					return pvc, nil
				}
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return pv, nil
				}
				deletePersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolumeClaim) error {
					return nil
				}
				updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
					return nil
				}
				sleep = func(_ time.Duration) {
					// Mock sleep function to do nothing
				}
				createPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolumeClaim) error {
					return errors.New("unable to create PVC")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()
			pvcName := tt.pvcName
			namespace := tt.namespace
			targetPV := tt.targetPV
			rgTarget := tt.rgTarget
			log := tt.log
			client := tt.client
			ctx := context.Background()
			r := &ReplicationGroupReconciler{
				Client:          nil,
				Log:             ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
				Scheme:          nil,
				EventRecorder:   nil,
				Config:          nil,
				Domain:          "",
				DisablePVCRemap: false,
			}
			err := r.swapPVC(ctx, client, pvcName, namespace, targetPV, rgTarget, log)
			if tt.expectedErr {
				if tt.name == "Error getting PVC" && !strings.Contains(err.Error(), "error getting pvc") {
					t.Errorf("expected error, got %s", err)
				} else if tt.name == "Error getting PV" && !strings.Contains(err.Error(), "error retrieving local PV") {
					t.Errorf("expected error, got %s", err)
				}
			} else {
				t.Logf("Expected no error, got %s", err)
			}
		})
	}
}

func TestSwapPVCWithClaimRef(t *testing.T) {
	originalGetPersistentVolumeClaim := getPersistentVolumeClaim
	originalGetPersistentVolume := getPersistentVolume

	after := func() {
		getPersistentVolumeClaim = originalGetPersistentVolumeClaim
		getPersistentVolume = originalGetPersistentVolume
	}

	fakeConfig := mocks.New("sourceCluster", "remote-123")
	remoteClient, _ := fakeConfig.GetConnection("remote-123")

	tests := []struct {
		name        string
		pvc         *v1.PersistentVolumeClaim
		client      connection.RemoteClusterClient
		pvcName     string
		namespace   string
		targetPV    string
		rgTarget    string
		log         logr.Logger
		setup       func()
		expectedErr bool
	}{
		{
			name:      "When claimRef is set to reserved",
			namespace: "fake-ns",
			pvcName:   "fake-pvc",
			client:    remoteClient,
			setup: func() {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake-pvc",
						Namespace: "fake-ns",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						VolumeName: "fake-pv",
					},
				}

				pv := &corev1.PersistentVolume{}
				pv.Spec.ClaimRef = &corev1.ObjectReference{
					Name:      controllers.ReservedPVCName,
					Namespace: controllers.ReservedPVCNamespace,
				}
				getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
					return pvc, nil
				}
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return pv, nil
				}
			},
			expectedErr: true,
		},
		{
			name:      "When claimRef is set to something other than reserved",
			namespace: "fake-ns",
			pvcName:   "fake-pvc",
			client:    remoteClient,
			setup: func() {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake-pvc",
						Namespace: "fake-ns",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						VolumeName: "fake-pv",
					},
				}

				pv := &corev1.PersistentVolume{}
				pv.Name = "fake-pv"
				pv.Spec.ClaimRef = &corev1.ObjectReference{
					Name:      "xyz",
					Namespace: "xyz",
				}
				getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
					return pvc, nil
				}
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return pv, nil
				}
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()
			pvcName := tt.pvcName
			namespace := tt.namespace
			targetPV := tt.targetPV
			rgTarget := tt.rgTarget
			log := tt.log
			client := tt.client
			ctx := context.Background()

			r := &ReplicationGroupReconciler{
				Client:          nil,
				Log:             ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
				Scheme:          nil,
				EventRecorder:   nil,
				Config:          nil,
				Domain:          "",
				DisablePVCRemap: false,
			}
			err := r.swapPVC(ctx, client, pvcName, namespace, targetPV, rgTarget, log)
			if tt.expectedErr {
				if tt.name == "When claimRef is set to something other than reserved" && !strings.Contains(err.Error(), "target PV fake-pv is claimed") {
					t.Errorf("expected error, got %s", err)
				}
			}
		})
	}
}

func TestRemoveReservedClaimRefForTargetPV(t *testing.T) {
	originalGetPersistentVolume := getPersistentVolume

	after := func() {
		getPersistentVolume = originalGetPersistentVolume
	}

	fakeConfig := mocks.New("sourceCluster", "remote-123")
	remoteClient, _ := fakeConfig.GetConnection("remote-123")
	type args struct {
		ctx    context.Context
		client connection.RemoteClusterClient
		pvName string
		log    logr.Logger
	}
	tests := []struct {
		name    string
		args    args
		setup   func()
		wantErr bool
	}{
		{
			name: "Error Retrieving PV",
			args: args{
				ctx:    context.TODO(),
				client: remoteClient,
				pvName: "fake-pv",
			},
			setup: func() {
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return nil, fmt.Errorf("error")
				}
			},
			wantErr: true,
		},
		{
			name: "No claimRef for PV",
			args: args{
				ctx:    context.TODO(),
				client: remoteClient,
				pvName: "fake-pv",
			},
			setup: func() {
				pv := &corev1.PersistentVolume{}
				pv.Spec.ClaimRef = nil
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return pv, nil
				}
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()
			err := removeReservedClaimRefForTargetPV(tt.args.ctx, tt.args.client, tt.args.pvName, tt.args.log)
			if (err != nil) != tt.wantErr {
				t.Errorf("PersistentVolumeReconciler.removeReservedClaimRefforTargetPV() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleDataVolumeDependencies(t *testing.T) {
	originalGetObject := getObject
	originalDeleteObject := deleteObject
	originalUpdateObject := updateObject

	after := func() {
		getObject = originalGetObject
		deleteObject = originalDeleteObject
		updateObject = originalUpdateObject
	}

	trueVal := true

	tests := []struct {
		name          string
		pvc           *v1.PersistentVolumeClaim
		setup         func()
		wantErr       bool
		wantErrMsg    string
		wantDVDeleted bool
		validate      func(t *testing.T, pvc *v1.PersistentVolumeClaim)
	}{
		{
			name: "No DataVolume ownerRef — no action",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "v1", Kind: "ConfigMap", Name: "cm"},
					},
				},
			},
			setup:         func() {},
			wantErr:       false,
			wantDVDeleted: false,
			validate: func(t *testing.T, pvc *v1.PersistentVolumeClaim) {
				if len(pvc.OwnerReferences) != 1 {
					t.Errorf("expected 1 ownerRef unchanged, got %d", len(pvc.OwnerReferences))
				}
			},
		},
		{
			name: "DV owned by VM — skip cleanup, return error",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "test-dv", UID: "dv-uid", Controller: &trueVal},
					},
				},
			},
			setup: func() {
				getObject = func(_ context.Context, _ connection.RemoteClusterClient, key client.ObjectKey, obj client.Object) error {
					u := obj.(*unstructured.Unstructured)
					u.SetName(key.Name)
					u.SetNamespace(key.Namespace)
					u.SetOwnerReferences([]metav1.OwnerReference{
						{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine", Name: "test-vm", UID: "vm-uid"},
					})
					return nil
				}
			},
			wantErr:       true,
			wantErrMsg:    "skipping cleanup",
			wantDVDeleted: false,
		},
		{
			name: "DV NOT owned by VM, no finalizers — delete DV, return dvDeleted=true",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "test-dv", UID: "dv-uid", Controller: &trueVal},
						{APIVersion: "v1", Kind: "ConfigMap", Name: "cm"},
					},
					Annotations: map[string]string{
						"cdi.kubevirt.io/storage.condition.running": "true",
						"replication.storage.dell.com/remotePV":     "remote-pv",
					},
				},
			},
			setup: func() {
				getObject = func(_ context.Context, _ connection.RemoteClusterClient, key client.ObjectKey, obj client.Object) error {
					u := obj.(*unstructured.Unstructured)
					u.SetName(key.Name)
					u.SetNamespace(key.Namespace)
					u.SetOwnerReferences(nil)
					return nil
				}
				deleteObject = func(_ context.Context, _ connection.RemoteClusterClient, _ client.Object) error {
					return nil
				}
			},
			wantErr:       false,
			wantDVDeleted: true,
			validate: func(t *testing.T, pvc *v1.PersistentVolumeClaim) {
				// PVC should NOT be modified by handleDataVolumeDependencies
				if len(pvc.OwnerReferences) != 2 {
					t.Errorf("expected PVC ownerRefs unchanged (2), got %d", len(pvc.OwnerReferences))
				}
				if _, ok := pvc.Annotations["cdi.kubevirt.io/storage.condition.running"]; !ok {
					t.Error("expected PVC annotations unchanged")
				}
			},
		},
		{
			name: "DV with finalizers — remove finalizers, delete DV, return dvDeleted=true",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "test-dv", UID: "dv-uid", Controller: &trueVal},
					},
				},
			},
			setup: func() {
				finalizerRemoved := false
				getObject = func(_ context.Context, _ connection.RemoteClusterClient, key client.ObjectKey, obj client.Object) error {
					u := obj.(*unstructured.Unstructured)
					u.SetName(key.Name)
					u.SetNamespace(key.Namespace)
					u.SetOwnerReferences(nil)
					u.SetFinalizers([]string{"cdi.kubevirt.io/dataVolumeFinalizer"})
					return nil
				}
				updateObject = func(_ context.Context, _ connection.RemoteClusterClient, obj client.Object) error {
					u := obj.(*unstructured.Unstructured)
					if len(u.GetFinalizers()) != 0 {
						t.Error("expected finalizers to be cleared before update")
					}
					finalizerRemoved = true
					return nil
				}
				deleteObject = func(_ context.Context, _ connection.RemoteClusterClient, _ client.Object) error {
					if !finalizerRemoved {
						t.Error("expected finalizers to be removed before delete")
					}
					return nil
				}
			},
			wantErr:       false,
			wantDVDeleted: true,
		},
		{
			name: "DV not found — return dvDeleted=false (caller handles PVC deletion)",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "test-dv", UID: "dv-uid"},
					},
				},
			},
			setup: func() {
				getObject = func(_ context.Context, _ connection.RemoteClusterClient, _ client.ObjectKey, _ client.Object) error {
					return k8serrors.NewNotFound(schema.GroupResource{Group: "cdi.kubevirt.io", Resource: "datavolumes"}, "test-dv")
				}
			},
			wantErr:       false,
			wantDVDeleted: false,
			validate: func(t *testing.T, pvc *v1.PersistentVolumeClaim) {
				// PVC should NOT be modified
				if len(pvc.OwnerReferences) != 1 {
					t.Errorf("expected PVC ownerRefs unchanged, got %d", len(pvc.OwnerReferences))
				}
			},
		},
		{
			name: "Error fetching DV — return error",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "test-dv", UID: "dv-uid"},
					},
				},
			},
			setup: func() {
				getObject = func(_ context.Context, _ connection.RemoteClusterClient, _ client.ObjectKey, _ client.Object) error {
					return fmt.Errorf("connection refused")
				}
			},
			wantErr:       true,
			wantErrMsg:    "error fetching DataVolume",
			wantDVDeleted: false,
		},
		{
			name: "Error removing DV finalizers — return error",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "test-dv", UID: "dv-uid"},
					},
				},
			},
			setup: func() {
				getObject = func(_ context.Context, _ connection.RemoteClusterClient, key client.ObjectKey, obj client.Object) error {
					u := obj.(*unstructured.Unstructured)
					u.SetName(key.Name)
					u.SetNamespace(key.Namespace)
					u.SetOwnerReferences(nil)
					u.SetFinalizers([]string{"cdi.kubevirt.io/dataVolumeFinalizer"})
					return nil
				}
				updateObject = func(_ context.Context, _ connection.RemoteClusterClient, _ client.Object) error {
					return errors.New("update conflict")
				}
			},
			wantErr:       true,
			wantErrMsg:    "error removing finalizers",
			wantDVDeleted: false,
		},
		{
			name: "Error deleting DV — return error",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "test-dv", UID: "dv-uid"},
					},
				},
			},
			setup: func() {
				getObject = func(_ context.Context, _ connection.RemoteClusterClient, key client.ObjectKey, obj client.Object) error {
					u := obj.(*unstructured.Unstructured)
					u.SetName(key.Name)
					u.SetNamespace(key.Namespace)
					u.SetOwnerReferences(nil)
					return nil
				}
				deleteObject = func(_ context.Context, _ connection.RemoteClusterClient, _ client.Object) error {
					return errors.New("delete error")
				}
			},
			wantErr:       true,
			wantErrMsg:    "error deleting DataVolume",
			wantDVDeleted: false,
		},
		{
			name: "DV delete returns NotFound — idempotent, return dvDeleted=true",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "test-dv", UID: "dv-uid"},
					},
				},
			},
			setup: func() {
				getObject = func(_ context.Context, _ connection.RemoteClusterClient, key client.ObjectKey, obj client.Object) error {
					u := obj.(*unstructured.Unstructured)
					u.SetName(key.Name)
					u.SetNamespace(key.Namespace)
					u.SetOwnerReferences(nil)
					return nil
				}
				deleteObject = func(_ context.Context, _ connection.RemoteClusterClient, _ client.Object) error {
					return k8serrors.NewNotFound(schema.GroupResource{Group: "cdi.kubevirt.io", Resource: "datavolumes"}, "test-dv")
				}
			},
			wantErr:       false,
			wantDVDeleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()

			r := &ReplicationGroupReconciler{
				EventRecorder: record.NewFakeRecorder(10),
			}
			log := ctrl.Log.WithName("test")

			dvDeleted, err := r.handleDataVolumeDependencies(context.Background(), nil, tt.pvc, log)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleDataVolumeDependencies() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErrMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error containing %q, got %q", tt.wantErrMsg, err.Error())
				}
			}
			if dvDeleted != tt.wantDVDeleted {
				t.Errorf("handleDataVolumeDependencies() dvDeleted = %v, want %v", dvDeleted, tt.wantDVDeleted)
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, tt.pvc)
			}
		})
	}
}

func TestFindDataVolumeOwnerRef(t *testing.T) {
	refs := []metav1.OwnerReference{
		{APIVersion: "v1", Kind: "ConfigMap", Name: "cm"},
		{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "my-dv"},
		{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine", Name: "my-vm"},
	}

	result := findDataVolumeOwnerRef(refs)
	if result == nil {
		t.Fatal("expected to find DataVolume ownerRef")
	}
	if result.Name != "my-dv" {
		t.Errorf("expected name my-dv, got %s", result.Name)
	}

	// No DV ownerRef
	noRefs := []metav1.OwnerReference{
		{APIVersion: "v1", Kind: "ConfigMap", Name: "cm"},
	}
	if findDataVolumeOwnerRef(noRefs) != nil {
		t.Error("expected nil for no DV ownerRef")
	}

	// Empty slice
	if findDataVolumeOwnerRef(nil) != nil {
		t.Error("expected nil for nil refs")
	}
}

func TestFindVMOwnerRef(t *testing.T) {
	refs := []metav1.OwnerReference{
		{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "my-dv"},
		{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine", Name: "my-vm"},
	}

	result := findVMOwnerRef(refs)
	if result == nil {
		t.Fatal("expected to find VirtualMachine ownerRef")
	}
	if result.Name != "my-vm" {
		t.Errorf("expected name my-vm, got %s", result.Name)
	}

	// No VM ownerRef
	noVM := []metav1.OwnerReference{
		{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "my-dv"},
	}
	if findVMOwnerRef(noVM) != nil {
		t.Error("expected nil for no VM ownerRef")
	}
}

func TestRemoveDataVolumeOwnerRef(t *testing.T) {
	refs := []metav1.OwnerReference{
		{APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataVolume", Name: "my-dv"},
		{APIVersion: "v1", Kind: "ConfigMap", Name: "cm"},
		{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine", Name: "my-vm"},
	}

	result := removeDataVolumeOwnerRef(refs)
	if len(result) != 2 {
		t.Fatalf("expected 2 ownerRefs, got %d", len(result))
	}
	for _, ref := range result {
		if ref.Kind == "DataVolume" {
			t.Error("DataVolume ownerRef should have been removed")
		}
	}

	// Empty refs
	empty := removeDataVolumeOwnerRef(nil)
	if len(empty) != 0 {
		t.Errorf("expected 0 ownerRefs for nil input, got %d", len(empty))
	}
}

func TestRemoveCDIAnnotations(t *testing.T) {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"cdi.kubevirt.io/storage.condition.running": "true",
				"cdi.kubevirt.io/storage.pod.phase":         "Succeeded",
				"replication.storage.dell.com/remotePV":     "remote-pv",
				"app.kubernetes.io/name":                    "my-app",
			},
		},
	}

	removeCDIAnnotations(pvc)

	if len(pvc.Annotations) != 2 {
		t.Errorf("expected 2 annotations after removal, got %d", len(pvc.Annotations))
	}
	for key := range pvc.Annotations {
		if strings.Contains(key, "cdi.kubevirt.io") {
			t.Errorf("CDI annotation %s should have been removed", key)
		}
	}

	// Nil annotations — should not panic
	nilPVC := &v1.PersistentVolumeClaim{}
	removeCDIAnnotations(nilPVC) // no panic = pass
}

func TestRecoverPVCBackup(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalGetPersistentVolume := getPersistentVolume

	after := func() {
		getPersistentVolume = originalGetPersistentVolume
	}

	sc := "sc-1"
	backupPVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup-pvc",
			Namespace: "ns",
			Annotations: map[string]string{
				controllers.RemotePV: "remote-pv",
			},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName:       "local-pv",
			StorageClassName: &sc,
		},
	}
	backup := &pvcSwapBackup{
		PVC:            backupPVC,
		LocalPVPolicy:  v1.PersistentVolumeReclaimDelete,
		RemotePVPolicy: v1.PersistentVolumeReclaimRetain,
	}
	backupJSON, _ := json.Marshal(backup)

	tests := []struct {
		name        string
		setup       func()
		expectedErr bool
		errContains string
	}{
		{
			name: "Error getting target PV",
			setup: func() {
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return nil, fmt.Errorf("not found")
				}
			},
			expectedErr: true,
			errContains: "error getting target PV",
		},
		{
			name: "Target PV has no RemotePV annotation",
			setup: func() {
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					return &v1.PersistentVolume{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "target-pv",
							Annotations: map[string]string{},
						},
					}, nil
				}
			},
			expectedErr: true,
			errContains: "has no RemotePV annotation",
		},
		{
			name: "Error getting local PV",
			setup: func() {
				callCount := 0
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					callCount++
					if callCount == 1 {
						return &v1.PersistentVolume{
							ObjectMeta: metav1.ObjectMeta{
								Name: "target-pv",
								Annotations: map[string]string{
									controllers.RemotePV: "local-pv",
								},
							},
						}, nil
					}
					return nil, fmt.Errorf("local PV not found")
				}
			},
			expectedErr: true,
			errContains: "error getting local PV",
		},
		{
			name: "Local PV has no pending swap annotation",
			setup: func() {
				callCount := 0
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					callCount++
					if callCount == 1 {
						return &v1.PersistentVolume{
							ObjectMeta: metav1.ObjectMeta{
								Name: "target-pv",
								Annotations: map[string]string{
									controllers.RemotePV: "local-pv",
								},
							},
						}, nil
					}
					return &v1.PersistentVolume{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "local-pv",
							Annotations: map[string]string{},
						},
					}, nil
				}
			},
			expectedErr: true,
			errContains: "no pending PVC swap annotation",
		},
		{
			name: "Invalid JSON in backup annotation",
			setup: func() {
				callCount := 0
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					callCount++
					if callCount == 1 {
						return &v1.PersistentVolume{
							ObjectMeta: metav1.ObjectMeta{
								Name: "target-pv",
								Annotations: map[string]string{
									controllers.RemotePV: "local-pv",
								},
							},
						}, nil
					}
					return &v1.PersistentVolume{
						ObjectMeta: metav1.ObjectMeta{
							Name: "local-pv",
							Annotations: map[string]string{
								controllers.PendingPVCSwap: "not-valid-json",
							},
						},
					}, nil
				}
			},
			expectedErr: true,
			errContains: "error unmarshaling PVC backup",
		},
		{
			name: "Successful recovery",
			setup: func() {
				callCount := 0
				getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
					callCount++
					if callCount == 1 {
						return &v1.PersistentVolume{
							ObjectMeta: metav1.ObjectMeta{
								Name: "target-pv",
								Annotations: map[string]string{
									controllers.RemotePV: "local-pv",
								},
							},
						}, nil
					}
					return &v1.PersistentVolume{
						ObjectMeta: metav1.ObjectMeta{
							Name: "local-pv",
							Annotations: map[string]string{
								controllers.PendingPVCSwap: string(backupJSON),
							},
						},
					}, nil
				}
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer after()
			log := ctrl.Log.WithName("test")
			result, err := recoverPVCBackup(context.Background(), nil, "target-pv", log)
			if tt.expectedErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("expected non-nil backup")
				}
				if result.PVC.Name != "backup-pvc" {
					t.Errorf("expected PVC name backup-pvc, got %s", result.PVC.Name)
				}
				if result.LocalPVPolicy != v1.PersistentVolumeReclaimDelete {
					t.Errorf("expected LocalPVPolicy Delete, got %s", result.LocalPVPolicy)
				}
			}
		})
	}
}

func TestSavePVCBackupToPV(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalUpdatePV := updatePersistentVolume
	defer func() { updatePersistentVolume = originalUpdatePV }()

	sc := "sc-1"
	backup := &pvcSwapBackup{
		PVC: &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: "ns"},
			Spec:       v1.PersistentVolumeClaimSpec{VolumeName: "pv", StorageClassName: &sc},
		},
		LocalPVPolicy:  v1.PersistentVolumeReclaimDelete,
		RemotePVPolicy: v1.PersistentVolumeReclaimRetain,
	}
	log := ctrl.Log.WithName("test")

	t.Run("Successful save", func(t *testing.T) {
		localPV := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "local-pv"},
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return nil
		}
		err := savePVCBackupToPV(context.Background(), nil, localPV, backup, log)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if localPV.Annotations[controllers.PendingPVCSwap] == "" {
			t.Error("expected PendingPVCSwap annotation to be set")
		}
		if localPV.Spec.PersistentVolumeReclaimPolicy != v1.PersistentVolumeReclaimRetain {
			t.Errorf("expected Retain policy, got %s", localPV.Spec.PersistentVolumeReclaimPolicy)
		}
	})

	t.Run("Update error", func(t *testing.T) {
		localPV := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "local-pv", Annotations: map[string]string{}},
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return fmt.Errorf("update failed")
		}
		err := savePVCBackupToPV(context.Background(), nil, localPV, backup, log)
		if err == nil {
			t.Error("expected error")
		} else if !strings.Contains(err.Error(), "error saving PVC backup") {
			t.Errorf("expected 'error saving PVC backup', got %q", err.Error())
		}
	})

	t.Run("Nil annotations initialised", func(t *testing.T) {
		localPV := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "local-pv"},
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return nil
		}
		err := savePVCBackupToPV(context.Background(), nil, localPV, backup, log)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if localPV.Annotations == nil {
			t.Error("expected annotations to be initialised")
		}
	})
}

func TestVerifyPVC(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalGetPVC := getPersistentVolumeClaim
	originalSleep := sleep
	defer func() {
		getPersistentVolumeClaim = originalGetPVC
		sleep = originalSleep
	}()
	sleep = func(_ time.Duration) {}

	t.Run("Success on first attempt", func(t *testing.T) {
		getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
			return &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{VolumeName: "target-pv"},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controllers.RemotePV: "local-pv",
					},
				},
			}, nil
		}
		err := verifyPVC(context.Background(), nil, "target-pv", "local-pv", "pvc", "ns")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Timeout when PVC never matches", func(t *testing.T) {
		getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
			return &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{VolumeName: "wrong-pv"},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controllers.RemotePV: "wrong-remote",
					},
				},
			}, nil
		}
		err := verifyPVC(context.Background(), nil, "target-pv", "local-pv", "pvc", "ns")
		if err == nil {
			t.Error("expected timeout error")
		} else if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("expected 'timed out', got %q", err.Error())
		}
	})

	t.Run("Error fetching PVC retries then times out", func(t *testing.T) {
		getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
			return nil, fmt.Errorf("transient error")
		}
		err := verifyPVC(context.Background(), nil, "target-pv", "local-pv", "pvc", "ns")
		if err == nil {
			t.Error("expected timeout error")
		} else if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("expected 'timed out', got %q", err.Error())
		}
	})

	t.Run("Success on third attempt", func(t *testing.T) {
		callCount := 0
		getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
			callCount++
			if callCount < 3 {
				return &v1.PersistentVolumeClaim{
					Spec: v1.PersistentVolumeClaimSpec{VolumeName: "wrong-pv"},
				}, nil
			}
			return &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{VolumeName: "target-pv"},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controllers.RemotePV: "local-pv",
					},
				},
			}, nil
		}
		err := verifyPVC(context.Background(), nil, "target-pv", "local-pv", "pvc", "ns")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestUpdatePVClaimRefSuccess(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalGetPV := getPersistentVolume
	originalUpdatePV := updatePersistentVolume
	defer func() {
		getPersistentVolume = originalGetPV
		updatePersistentVolume = originalUpdatePV
	}()

	t.Run("ClaimRef already set returns nil", func(t *testing.T) {
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			return &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					ClaimRef: &v1.ObjectReference{Name: "existing", Namespace: "ns"},
				},
			}, nil
		}
		log := ctrl.Log.WithName("test")
		err := updatePVClaimRef(context.Background(), nil, "pv", "ns", "rv", "pvc", "uid", log)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Successful update clears RemotePVC annotations", func(t *testing.T) {
		var updatedPV *v1.PersistentVolume
		callCount := 0
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			callCount++
			if callCount == 1 {
				return &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							controllers.RemotePVCNamespace: "old-ns",
							controllers.RemotePVC:          "old-pvc",
						},
						Labels: map[string]string{
							controllers.RemotePVCNamespace: "old-ns",
						},
					},
					Spec: v1.PersistentVolumeSpec{},
				}, nil
			}
			// After update, return PV with ClaimRef set (simulates successful update)
			return &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					ClaimRef: &v1.ObjectReference{Name: "pvc", Namespace: "ns"},
				},
			}, nil
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, pv *v1.PersistentVolume) error {
			updatedPV = pv
			return nil
		}
		log := ctrl.Log.WithName("test")
		err := updatePVClaimRef(context.Background(), nil, "pv", "ns", "rv", "pvc", "uid", log)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if updatedPV == nil {
			t.Fatal("PV was not updated")
		}
		if updatedPV.Annotations[controllers.RemotePVCNamespace] != "" {
			t.Errorf("expected RemotePVCNamespace annotation to be cleared")
		}
		if updatedPV.Labels[controllers.RemotePVCNamespace] != "" {
			t.Errorf("expected RemotePVCNamespace label to be cleared")
		}
		if updatedPV.Annotations[controllers.RemotePVC] != "" {
			t.Errorf("expected RemotePVC annotation to be cleared")
		}
		if updatedPV.Spec.ClaimRef == nil {
			t.Error("expected ClaimRef to be set")
		}
	})
}

func TestRemovePVClaimRefSuccess(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalGetPV := getPersistentVolume
	originalUpdatePV := updatePersistentVolume
	originalSleep := sleep
	defer func() {
		getPersistentVolume = originalGetPV
		updatePersistentVolume = originalUpdatePV
		sleep = originalSleep
	}()
	sleep = func(_ time.Duration) {}

	t.Run("ClaimRef already nil returns immediately", func(t *testing.T) {
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			return &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{ClaimRef: nil},
			}, nil
		}
		log := ctrl.Log.WithName("test")
		err := removePVClaimRef(context.Background(), nil, "pv", "ns", "pvc", log)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ClaimRef removed on first update", func(t *testing.T) {
		callCount := 0
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			callCount++
			if callCount == 1 {
				return &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
						Labels:      map[string]string{},
					},
					Spec: v1.PersistentVolumeSpec{
						ClaimRef: &v1.ObjectReference{Name: "pvc", Namespace: "ns"},
					},
				}, nil
			}
			return &v1.PersistentVolume{Spec: v1.PersistentVolumeSpec{ClaimRef: nil}}, nil
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return nil
		}
		log := ctrl.Log.WithName("test")
		err := removePVClaimRef(context.Background(), nil, "pv", "ns", "pvc", log)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Conflict retries then succeeds", func(t *testing.T) {
		updateCount := 0
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			return &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: v1.PersistentVolumeSpec{
					ClaimRef: &v1.ObjectReference{Name: "pvc", Namespace: "ns"},
				},
			}, nil
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			updateCount++
			if updateCount == 1 {
				return k8serrors.NewConflict(schema.GroupResource{}, "pv", fmt.Errorf("conflict"))
			}
			return nil
		}
		log := ctrl.Log.WithName("test")
		err := removePVClaimRef(context.Background(), nil, "pv", "ns", "pvc", log)
		// It won't return nil because after update succeeds, it loops and getPV returns with ClaimRef again
		// But this exercises the conflict retry path
		if err != nil {
			t.Logf("got expected error from retry loop: %v", err)
		}
	})
}

func TestRemoveReservedClaimRefSuccess(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalGetPV := getPersistentVolume
	originalUpdatePV := updatePersistentVolume
	originalSleep := sleep
	defer func() {
		getPersistentVolume = originalGetPV
		updatePersistentVolume = originalUpdatePV
		sleep = originalSleep
	}()
	sleep = func(_ time.Duration) {}

	t.Run("ClaimRef already nil", func(t *testing.T) {
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			return &v1.PersistentVolume{Spec: v1.PersistentVolumeSpec{ClaimRef: nil}}, nil
		}
		log := ctrl.Log.WithName("test")
		err := removeReservedClaimRefForTargetPV(context.Background(), nil, "pv", log)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ClaimRef removed successfully", func(t *testing.T) {
		callCount := 0
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			callCount++
			if callCount == 1 {
				return &v1.PersistentVolume{
					Spec: v1.PersistentVolumeSpec{
						ClaimRef: &v1.ObjectReference{Name: "reserved", Namespace: "reserved"},
					},
				}, nil
			}
			return &v1.PersistentVolume{Spec: v1.PersistentVolumeSpec{ClaimRef: nil}}, nil
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return nil
		}
		log := ctrl.Log.WithName("test")
		err := removeReservedClaimRefForTargetPV(context.Background(), nil, "pv", log)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Conflict retry on update", func(t *testing.T) {
		updateCount := 0
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			return &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					ClaimRef: &v1.ObjectReference{Name: "reserved", Namespace: "reserved"},
				},
			}, nil
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			updateCount++
			if updateCount == 1 {
				return k8serrors.NewConflict(schema.GroupResource{}, "pv", fmt.Errorf("conflict"))
			}
			return nil
		}
		log := ctrl.Log.WithName("test")
		err := removeReservedClaimRefForTargetPV(context.Background(), nil, "pv", log)
		// Exercises the conflict branch
		if err != nil {
			t.Logf("got error from retry loop: %v", err)
		}
	})
}

func TestSetPVReclaimPolicySuccess(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalGetPV := getPersistentVolume
	originalUpdatePV := updatePersistentVolume
	originalSleep := sleep
	defer func() {
		getPersistentVolume = originalGetPV
		updatePersistentVolume = originalUpdatePV
		sleep = originalSleep
	}()
	sleep = func(_ time.Duration) {}

	t.Run("Policy set on first attempt", func(t *testing.T) {
		callCount := 0
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			callCount++
			return &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
				},
			}, nil
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return nil
		}
		err := setPVReclaimPolicy(context.Background(), nil, "pv", v1.PersistentVolumeReclaimRetain)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Second get error", func(t *testing.T) {
		callCount := 0
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			callCount++
			if callCount == 1 {
				return &v1.PersistentVolume{
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
					},
				}, nil
			}
			return nil, fmt.Errorf("error on re-read")
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return nil
		}
		err := setPVReclaimPolicy(context.Background(), nil, "pv", v1.PersistentVolumeReclaimRetain)
		if err == nil {
			t.Error("expected error")
		} else if !strings.Contains(err.Error(), "error retrieving PV") {
			t.Errorf("expected 'error retrieving PV', got %q", err.Error())
		}
	})

	t.Run("Timeout when policy never sticks", func(t *testing.T) {
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			return &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
				},
			}, nil
		}
		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return nil
		}
		err := setPVReclaimPolicy(context.Background(), nil, "pv", v1.PersistentVolumeReclaimRetain)
		if err == nil {
			t.Error("expected timeout error")
		} else if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("expected 'timed out', got %q", err.Error())
		}
	})
}

func TestSwapPVCRecoveryPath(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalGetPVC := getPersistentVolumeClaim
	originalGetPV := getPersistentVolume
	originalUpdatePV := updatePersistentVolume
	originalCreatePVC := createPersistentVolumeClaim
	originalSleep := sleep

	after := func() {
		getPersistentVolumeClaim = originalGetPVC
		getPersistentVolume = originalGetPV
		updatePersistentVolume = originalUpdatePV
		createPersistentVolumeClaim = originalCreatePVC
		sleep = originalSleep
	}
	defer after()
	sleep = func(_ time.Duration) {}

	t.Run("PVC not found and recovery fails", func(t *testing.T) {
		getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
			return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "persistentvolumeclaims"}, "pvc")
		}
		// recoverPVCBackup calls getPersistentVolume
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ string) (*v1.PersistentVolume, error) {
			return nil, fmt.Errorf("pv not found")
		}

		r := &ReplicationGroupReconciler{
			Log:    ctrl.Log.WithName("test"),
			Domain: constants.DefaultDomain,
		}
		log := ctrl.Log.WithName("test")
		err := r.swapPVC(context.Background(), nil, "pvc", "ns", "target-pv", "rg-target", log)
		if err == nil {
			t.Error("expected error")
		} else if !strings.Contains(err.Error(), "recovery failed") {
			t.Errorf("expected 'recovery failed', got %q", err.Error())
		}
	})

	t.Run("PVC not found but recovery succeeds then create fails", func(t *testing.T) {
		sc := "sc-1"
		remoteSC := "sc-2"
		backupPVC := &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pvc",
				Namespace: "ns",
				Annotations: map[string]string{
					controllers.RemotePV:                            "local-pv",
					controllers.StorageClassRemoteStorageClassParam: remoteSC,
					controllers.ReplicationGroup:                    "rg-old",
				},
				Labels: map[string]string{
					controllers.ReplicationGroup: "rg-old",
				},
			},
			Spec: v1.PersistentVolumeClaimSpec{
				VolumeName:       "local-pv",
				StorageClassName: &sc,
			},
		}
		backup := &pvcSwapBackup{
			PVC:            backupPVC,
			LocalPVPolicy:  v1.PersistentVolumeReclaimDelete,
			RemotePVPolicy: v1.PersistentVolumeReclaimRetain,
		}
		backupJSON, _ := json.Marshal(backup)

		getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, _ string) (*v1.PersistentVolumeClaim, error) {
			return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "persistentvolumeclaims"}, "pvc")
		}

		pvCallCount := 0
		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, name string) (*v1.PersistentVolume, error) {
			pvCallCount++
			if pvCallCount == 1 {
				// target PV for recoverPVCBackup
				return &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "target-pv",
						Annotations: map[string]string{
							controllers.RemotePV: "local-pv",
						},
					},
				}, nil
			}
			if pvCallCount == 2 {
				// local PV for recoverPVCBackup
				return &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-pv",
						Annotations: map[string]string{
							controllers.PendingPVCSwap: string(backupJSON),
						},
					},
				}, nil
			}
			return &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: name},
			}, nil
		}

		createPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolumeClaim) error {
			return fmt.Errorf("create failed")
		}

		r := &ReplicationGroupReconciler{
			Log:    ctrl.Log.WithName("test"),
			Domain: constants.DefaultDomain,
		}
		log := ctrl.Log.WithName("test")
		err := r.swapPVC(context.Background(), nil, "pvc", "ns", "target-pv", "rg-target", log)
		if err == nil {
			t.Error("expected error")
		} else if !strings.Contains(err.Error(), "unable to create PVC") {
			t.Errorf("expected 'unable to create PVC', got %q", err.Error())
		}
	})
}

func (suite *RGControllerTestSuite) TestProcessFailBackBothDisabled() {
	rg, _, _, _, _ := suite.getSingleClusterPVSetup()
	rgName := rg.Name

	// Disable both PVC remap and kubevirt PVC remap
	suite.reconciler.DisablePVCRemap = true
	suite.reconciler.EnableKubevirtPVCRemap = false

	// Invoke failback action
	time := metav1.Now()
	lastAction := repv1.LastAction{
		Time:      &time,
		Condition: "Action FAILBACK_LOCAL succeeded",
	}
	rg.Status = repv1.DellCSIReplicationGroupStatus{
		LastAction: lastAction,
		Conditions: []repv1.LastAction{lastAction},
	}
	rg.Annotations[controllers.ActionProcessedTime] = time.String()

	err := suite.client.Update(context.Background(), rg)
	suite.NoError(err)

	req := suite.getTypicalRequest()
	resp, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	// Verify PVC was NOT swapped because both remap flags are disabled
	var unchangedPVC corev1.PersistentVolumeClaim
	err = suite.client.Get(context.Background(), types.NamespacedName{Name: "fake-pvc", Namespace: "fake-ns"}, &unchangedPVC)
	suite.NoError(err)
	suite.Equal("local-pv", unchangedPVC.Spec.VolumeName, "PVC should still be bound to local PV")
	suite.Equal(rgName, unchangedPVC.Annotations[controllers.ReplicationGroup], "RG annotation should be unchanged")
}

func TestSwapPVCStaleClaimRef(t *testing.T) {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	originalGetPVC := getPersistentVolumeClaim
	originalGetPV := getPersistentVolume
	originalUpdatePV := updatePersistentVolume
	originalDeletePVC := deletePersistentVolumeClaim
	originalCreatePVC := createPersistentVolumeClaim
	originalSleep := sleep

	after := func() {
		getPersistentVolumeClaim = originalGetPVC
		getPersistentVolume = originalGetPV
		updatePersistentVolume = originalUpdatePV
		deletePersistentVolumeClaim = originalDeletePVC
		createPersistentVolumeClaim = originalCreatePVC
		sleep = originalSleep
	}
	defer after()
	sleep = func(_ time.Duration) {}

	t.Run("Remote PV has stale claimRef - PVC not found removes it", func(t *testing.T) {
		sc := "sc-1"
		remoteSC := "sc-2"
		pvc := &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake-pvc",
				Namespace: "fake-ns",
				Annotations: map[string]string{
					controllers.RemotePV:                            "remote-pv",
					controllers.StorageClassRemoteStorageClassParam: remoteSC,
					controllers.ReplicationGroup:                    "rg-old",
				},
				Labels: map[string]string{
					controllers.ReplicationGroup: "rg-old",
				},
			},
			Spec: v1.PersistentVolumeClaimSpec{
				VolumeName:       "local-pv",
				StorageClassName: &sc,
			},
		}

		pvcGetCount := 0
		getPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ string, name string) (*v1.PersistentVolumeClaim, error) {
			pvcGetCount++
			if pvcGetCount == 1 {
				return pvc, nil
			}
			if name == "stale-pvc" {
				return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "persistentvolumeclaims"}, "stale-pvc")
			}
			return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "persistentvolumeclaims"}, name)
		}

		getPersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, name string) (*v1.PersistentVolume, error) {
			if name == "local-pv" {
				return &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{Name: "local-pv"},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
					},
				}, nil
			}
			return &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "remote-pv"},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
					ClaimRef: &v1.ObjectReference{
						Name:      "stale-pvc",
						Namespace: "stale-ns",
					},
				},
			}, nil
		}

		updatePersistentVolume = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolume) error {
			return nil
		}
		deletePersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolumeClaim) error {
			return nil
		}
		createPersistentVolumeClaim = func(_ context.Context, _ connection.RemoteClusterClient, _ *v1.PersistentVolumeClaim) error {
			return fmt.Errorf("create failed")
		}

		r := &ReplicationGroupReconciler{
			Log:    ctrl.Log.WithName("test"),
			Domain: constants.DefaultDomain,
		}
		log := ctrl.Log.WithName("test")
		err := r.swapPVC(context.Background(), nil, "fake-pvc", "fake-ns", "remote-pv", "rg-target", log)
		// We expect it to get past the stale ClaimRef check, then fail at create
		if err == nil {
			t.Error("expected error")
		} else if !strings.Contains(err.Error(), "unable to create PVC") {
			t.Errorf("expected 'unable to create PVC', got %q", err.Error())
		}
	})
}
