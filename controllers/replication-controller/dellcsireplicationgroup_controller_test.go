/*
Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package replication_controller

import (
	"context"
	"fmt"
	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/stretchr/testify/suite"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
	"time"
)

type RGControllerTestSuite struct {
	suite.Suite
	client     client.Client
	driver     utils.Driver
	config     connection.MultiClusterClient
	reconciler *ReplicationGroupReconciler
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

func (suite *RGControllerTestSuite) getLocalRG(name, clusterID string) *storagev1alpha1.DellCSIReplicationGroup {
	//creating fake resource group
	replicationGroup := utils.GetRGObj(name, suite.driver.DriverName, clusterID,
		utils.LocalPGID, utils.RemotePGID, suite.getLocalParams(), suite.getRemoteParams())
	return replicationGroup
}

func (suite *RGControllerTestSuite) getRemoteRG(name, clusterID string) *storagev1alpha1.DellCSIReplicationGroup {
	//creating fake resource group
	replicationGroup := utils.GetRGObj(name, suite.driver.DriverName, clusterID,
		utils.RemotePGID, utils.LocalPGID, suite.getRemoteParams(), suite.getLocalParams())
	return replicationGroup
}

func (suite *RGControllerTestSuite) getRGWithoutSyncComplete(name string, local bool, self bool) *storagev1alpha1.DellCSIReplicationGroup {
	annotations := make(map[string]string)
	annotations[controllers.RemoteReplicationGroup] = name
	annotations[controllers.ContextPrefix] = utils.ContextPrefix

	annotations[controllers.RemoteRGRetentionPolicy] = controllers.RemoteRetentionValueDelete

	rgFinalizers := []string{controllers.RGFinalizer}

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
	if local {
		if self {
			annotations[controllers.RemoteClusterId] = utils.Self
			rg = suite.getLocalRG(name, utils.Self)
		} else {
			annotations[controllers.RemoteClusterId] = suite.driver.RemoteClusterID
			rg = suite.getLocalRG(name, suite.driver.RemoteClusterID)
		}
	} else {
		if self {
			annotations[controllers.RemoteClusterId] = utils.Self
			rg = suite.getRemoteRG(name, utils.Self)
		} else {
			annotations[controllers.RemoteClusterId] = suite.driver.SourceClusterID
			rg = suite.getRemoteRG(name, suite.driver.SourceClusterID)
		}
	}
	rg.Annotations = annotations
	rg.Finalizers = rgFinalizers
	return rg
}

func (suite *RGControllerTestSuite) getRGWithSyncComplete(name string) *storagev1alpha1.DellCSIReplicationGroup {
	annotations := make(map[string]string)
	annotations[controllers.RGSyncComplete] = "yes"
	annotations[controllers.RemoteReplicationGroup] = suite.driver.RGName
	annotations[controllers.RemoteClusterId] = suite.driver.RemoteClusterID
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

func (suite *RGControllerTestSuite) createSCAndRG(sc *storagev1.StorageClass, rg *storagev1alpha1.DellCSIReplicationGroup) {
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
	rg.Annotations[controllers.RemoteClusterId] = "invalidClusterID"
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
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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

	rg := new(storagev1alpha1.DellCSIReplicationGroup)
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
	err := suite.reconciler.SetupWithManager(mgr, expRateLimiter, 1)
	suite.Error(err, "Setup should fail when there is no manager")
}
