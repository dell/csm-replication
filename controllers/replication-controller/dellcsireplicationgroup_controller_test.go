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
	"fmt"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csm-replication/pkg/connection"
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

type PVCRemapTestSuite struct {
	suite.Suite
	client client.Client
	driver utils.Driver
	config connection.MultiClusterClient
}

type RGControllerTestSuite struct {
	suite.Suite
	client     client.Client
	driver     utils.Driver
	config     connection.MultiClusterClient
	reconciler *ReplicationGroupReconciler
	mockUtils  *utils.MockUtils
}

func (suite *RGControllerTestSuite) SetupTest() {
	suite.Init()
}

func (suite *RGControllerTestSuite) Init() {
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	suite.driver = utils.GetDefaultDriver()
	suite.client = utils.GetFakeClient()
	fakeConfig := config.NewFakeConfig(suite.driver.SourceClusterID, suite.driver.RemoteClusterID)
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
	newConfig := config.NewFakeConfigForSingleCluster(suite.client,
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
	newConfig := config.NewFakeConfigForSingleCluster(suite.client,
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

func TestRGControllerTestSuite(t *testing.T) {
	testSuite := new(RGControllerTestSuite)
	suite.Run(t, testSuite)
}

func (suite *RGControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}

func (suite *RGControllerTestSuite) TestSetupWithManagerRg() {
	suite.Init()
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 10*time.Second)
	err := suite.reconciler.SetupWithManager(mgr, expRateLimiter, 1, false)
	suite.Error(err, "Setup should fail when there is no manager")
}

func (suite *RGControllerTestSuite) TestRemapPVC() {
	// scenario: RG without sync complete
	newConfig := config.NewFakeConfigForSingleCluster(suite.client,
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
	assert.NotNil(suite.T(), pvcObj)

	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)

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
	resp, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
	suite.Equal(false, resp.Requeue)

	// Verify that the FAILOVER action was processed
	updatedRG := new(repv1.DellCSIReplicationGroup)
	err = suite.client.Get(context.Background(), req.NamespacedName, rg)
	suite.NoError(err)
	processedTime, ok := updatedRG.Annotations[controllers.ActionProcessedTime]
    if !ok {
        suite.FailNow("ActionProcessedTime annotation is missing")
    }
	// suite.Equal(time.String(), rg.Annotations[controllers.ActionProcessedTime], "Action should be marked as processed")
	suite.Equal(time.String(), processedTime, "Action should be marked as processed with the correct time")

	/*
			// Retrieve the original local PV
		    originalLocalPV := &corev1.PersistentVolume{}
		    err := suite.client.Get(ctx, types.NamespacedName{Name: "local-pv"}, originalLocalPV)
		    suite.NoError(err)

		    // Retrieve the original remote PV
		    originalRemotePV := &corev1.PersistentVolume{}
		    err = suite.client.Get(ctx, types.NamespacedName{Name: "remote-pv"}, originalRemotePV)
		    suite.NoError(err)

		    // Store the original reclaim policies
		    originalLocalReclaimPolicy := originalLocalPV.Spec.PersistentVolumeReclaimPolicy
		    originalRemoteReclaimPolicy := originalRemotePV.Spec.PersistentVolumeReclaimPolicy */

	err = swapAllPVC(ctx, rClient, rgName, replicatedRGName, suite.reconciler.Log)
	suite.NoError(err)

	/*
	 // Fetch the updated PVs
	 updatedLocalPV := &corev1.PersistentVolume{}
	 updatedRemotePV := &corev1.PersistentVolume{}
	 err = suite.client.Get(ctx, types.NamespacedName{Name: "local-pv"}, updatedLocalPV)
	 suite.NoError(err)
	 err = suite.client.Get(ctx, types.NamespacedName{Name: "remote-pv"}, updatedRemotePV)
	 suite.NoError(err)

	 // Check if the reclaim policies are unchanged
	 updatedLocalReclaimPolicy := updatedLocalPV.Spec.PersistentVolumeReclaimPolicy
	 updatedRemoteReclaimPolicy := updatedRemotePV.Spec.PersistentVolumeReclaimPolicy

	 assert.Equal(suite.T(), originalLocalReclaimPolicy, updatedLocalReclaimPolicy, "Local PV reclaim policy should remain unchanged after swapAllPVC")
	 assert.Equal(suite.T(), originalRemoteReclaimPolicy, updatedRemoteReclaimPolicy, "Remote PV reclaim policy should remain unchanged after swapAllPVC") */

	// Fetch the updated local PV
	updatedLocalPV := &corev1.PersistentVolume{}
	err = suite.client.Get(ctx, types.NamespacedName{Name: "local-pv"}, updatedLocalPV)
	suite.NoError(err)

	// Fetch the updated remote PV
	updatedRemotePV := &corev1.PersistentVolume{}
	err = suite.client.Get(ctx, types.NamespacedName{Name: "remote-pv"}, updatedRemotePV)
	suite.NoError(err)

	// Check that the reclaim policies are both set to "Delete" after swapAllPVC
	assert.Equal(suite.T(), controllers.RemoteRetentionValueDelete, string(updatedLocalPV.Spec.PersistentVolumeReclaimPolicy), "Local PV reclaim policy should be 'Delete' after swapAllPVC")
	assert.Equal(suite.T(), controllers.RemoteRetentionValueDelete, string(updatedRemotePV.Spec.PersistentVolumeReclaimPolicy), "Remote PV reclaim policy should be 'Delete' after swapAllPVC")

}
