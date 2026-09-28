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
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/internal/metrics"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	csireplication "github.com/dell/csm-replication/test/mocks"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type failingListClient struct {
	client.Client
}

func (f failingListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errors.New("list failed")
}

type MonitoringControllerTestSuite struct {
	suite.Suite
	client      client.Client
	driver      utils.Driver
	repClient   *csireplication.MockReplication
	rgMonitor   *ReplicationGroupMonitoring
	rgReconcile *ReplicationGroupReconciler
}

func (suite *MonitoringControllerTestSuite) SetupSuite() {
	suite.Init()
}

func (suite *MonitoringControllerTestSuite) SetupTest() {
}

func (suite *MonitoringControllerTestSuite) getRGs() []client.Object {
	initObjs := make([]client.Object, 0)
	for i := 0; i < 10; i++ {
		rgName := fmt.Sprintf("%s-%d", suite.driver.RGName, i)
		rgObj := utils.GetRGObj(rgName, suite.driver.DriverName, suite.driver.RemoteClusterID, utils.LocalPGID,
			utils.RemotePGID, nil, nil)
		initObjs = append(initObjs, rgObj)
		pvName := fmt.Sprintf("pv-%s-%d", suite.driver.RGName, i)
		pvObj := utils.GetPVObj(pvName, "vol-handle", suite.driver.DriverName, suite.driver.StorageClass, nil)
		labels := make(map[string]string)
		labels[controllers.DriverName] = suite.driver.DriverName
		labels[controllers.ReplicationGroup] = rgName
		pvObj.Labels = labels
		initObjs = append(initObjs, pvObj)
	}
	return initObjs
}

func (suite *MonitoringControllerTestSuite) Init() {
	suite.driver = utils.GetDefaultDriver()
	initObjs := suite.getRGs()
	suite.client = utils.GetFakeClientWithObjects(initObjs...)
	repClient := csireplication.NewFakeReplicationClient(utils.ContextPrefix)
	suite.repClient = &repClient
	suite.initController()
	suite.runMonitor()
}

func (suite *MonitoringControllerTestSuite) initController() {
	rgMonitor := ReplicationGroupMonitoring{
		Client:             suite.client,
		DriverName:         suite.driver.DriverName,
		ReplicationClient:  suite.repClient,
		MonitoringInterval: 1 * time.Second,
	}
	suite.rgMonitor = &rgMonitor
}

func TestMonitoringControllerTestSuite(t *testing.T) {
	testSuite := new(MonitoringControllerTestSuite)
	suite.Run(t, testSuite)
}

func (suite *MonitoringControllerTestSuite) runMonitor() {
	ctx, cancel := context.WithCancel(context.Background())
	suite.T().Cleanup(func() {
		// cancel the rg monitor loop when the tests are done
		// to avoid data races caused by other tests resetting the
		// label key domain values in InitLabelsAndAnnotations()
		cancel()
	})
	err := suite.rgMonitor.Monitor(ctx)
	suite.NoError(err)
}

func (suite *MonitoringControllerTestSuite) TestMonitorReplicationGroups() {
	time.Sleep(2 * time.Second)
	var rgList repv1.DellCSIReplicationGroupList
	err := suite.client.List(context.Background(), &rgList)
	suite.NoError(err)
	updateTimes := make(map[string]time.Time)
	for _, rg := range rgList.Items {
		suite.NotNil(rg.Status.ReplicationLinkState)
		suite.NotNil(rg.Status.ReplicationLinkState.LastSuccessfulUpdate)
		updateTimes[rg.Name] = rg.DeepCopy().Status.ReplicationLinkState.LastSuccessfulUpdate.Time
	}
	suite.T().Log("Sleeping to allow another RG link update")
	time.Sleep(3 * time.Second)
	err = suite.client.List(context.Background(), &rgList)
	suite.NoError(err)
	for _, rg := range rgList.Items {
		suite.NotNil(rg.Status.ReplicationLinkState)
		suite.NotNil(rg.Status.ReplicationLinkState.LastSuccessfulUpdate)
		suite.NotEqual(updateTimes[rg.Name], rg.Status.ReplicationLinkState.LastSuccessfulUpdate.Time)
	}
}

func (suite *MonitoringControllerTestSuite) TestMonitorReplicationGroupsWithErrors() {
	time.Sleep(2500 * time.Millisecond)
	var rgList repv1.DellCSIReplicationGroupList
	err := suite.client.List(context.Background(), &rgList)
	suite.NoError(err)
	updateTimes := make(map[string]time.Time)
	for _, rg := range rgList.Items {
		suite.NotNil(rg.Status.ReplicationLinkState)
		suite.NotNil(rg.Status.ReplicationLinkState.LastSuccessfulUpdate)
		updateTimes[rg.Name] = rg.DeepCopy().Status.ReplicationLinkState.LastSuccessfulUpdate.Time
	}
	errorMsg := "failed to get status"
	suite.rgMonitor.Lock.Lock()
	suite.repClient.InjectError(errors.New(errorMsg))
	suite.rgMonitor.Lock.Unlock()
	suite.T().Log("Sleeping to allow another RG link update")
	time.Sleep(3 * time.Second)
	err = suite.client.List(context.Background(), &rgList)
	suite.NoError(err)
	for _, rg := range rgList.Items {
		suite.NotNil(rg.Status.ReplicationLinkState)
		suite.Equal(replication.StorageProtectionGroupStatus_UNKNOWN.String(), rg.Status.ReplicationLinkState.State)
		suite.Equal(updateTimes[rg.Name], rg.Status.ReplicationLinkState.LastSuccessfulUpdate.Time)
		suite.Equal(errorMsg, rg.Status.ReplicationLinkState.ErrorMessage)
	}
	suite.repClient.ClearErrorAndCondition(true)
}

func (suite *MonitoringControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}

func TestReplicationGroupMonitoringTracksCompleteCollectionCycles(t *testing.T) {
	tests := []struct {
		name      string
		client    client.Client
		wantStale string
		wantTime  bool
	}{
		{name: "complete", client: utils.GetFakeClient(), wantStale: "0", wantTime: true},
		{name: "list failure", client: failingListClient{Client: utils.GetFakeClient()}, wantStale: "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := prometheus.NewPedanticRegistry()
			previous := metrics.GetGlobalReplicationMetrics()
			metrics.SetGlobalReplicationMetrics(metrics.NewReplicationMetrics(registry))
			defer metrics.SetGlobalReplicationMetrics(previous)

			monitor := &ReplicationGroupMonitoring{Client: tt.client, DriverName: "driver", MonitoringInterval: time.Second}
			monitor.monitorReplicationGroups()

			expected := fmt.Sprintf(`# HELP dell_csm_repl_metrics_stale 1 when replication metrics are stale (monitoring loop failing), 0 when fresh.
# TYPE dell_csm_repl_metrics_stale gauge
dell_csm_repl_metrics_stale{driver="driver"} %s
`, tt.wantStale)
			assert.NoError(t, testutil.GatherAndCompare(registry, strings.NewReader(expected), metrics.MetricReplicationMetricsStale))

			families, err := registry.Gather()
			assert.NoError(t, err)
			foundTimestamp := false
			for _, family := range families {
				if family.GetName() == metrics.MetricReplicationLastCollectionTimestamp {
					foundTimestamp = len(family.Metric) == 1 && family.Metric[0].GetGauge().GetValue() > 0
				}
			}
			assert.Equal(t, tt.wantTime, foundTimestamp)
		})
	}
}

func TestReplicationGroupMonitoring_isUpdateRequired(t *testing.T) {
	monitor := &ReplicationGroupMonitoring{MonitoringInterval: time.Second}

	rg := repv1.DellCSIReplicationGroup{}
	if !monitor.isUpdateRequired(rg) {
		t.Fatalf("expected update to be required when last update time is zero")
	}

	rg.Spec.Action = "Suspend"
	rg.Status.ReplicationLinkState.LastSuccessfulUpdate = &metav1.Time{Time: time.Now().Add(-2 * time.Second)}
	if monitor.isUpdateRequired(rg) {
		t.Fatalf("expected update to be skipped while an action is in progress")
	}

	rg.Spec.Action = ""
	rg.Status.ReplicationLinkState.LastSuccessfulUpdate = &metav1.Time{Time: time.Now()}
	if monitor.isUpdateRequired(rg) {
		t.Fatalf("expected update to be skipped before monitoring interval elapses")
	}

	rg.Status.ReplicationLinkState.LastSuccessfulUpdate = &metav1.Time{Time: time.Now().Add(-2 * time.Second)}
	if !monitor.isUpdateRequired(rg) {
		t.Fatalf("expected update after monitoring interval elapsed")
	}
}

func TestUpdateRGLinkStatus(t *testing.T) {
	rg := utils.GetRGObj("rg-link-status", "driver", "remote-cluster", utils.LocalPGID, utils.RemotePGID, nil, nil)
	cl := utils.GetFakeClientWithObjects(rg)

	err := updateRGLinkStatus(context.Background(), cl, rg.DeepCopy(), replication.StorageProtectionGroupStatus_SYNCHRONIZED.String(), true, "")
	if err != nil {
		t.Fatalf("expected status update to succeed, got %v", err)
	}

	updatedRG := &repv1.DellCSIReplicationGroup{}
	err = cl.Get(context.Background(), types.NamespacedName{Name: rg.Name}, updatedRG)
	if err != nil {
		t.Fatalf("expected to fetch updated RG, got %v", err)
	}
	if updatedRG.Status.ReplicationLinkState.State != replication.StorageProtectionGroupStatus_SYNCHRONIZED.String() {
		t.Fatalf("expected replication link state to be updated")
	}

	err = updateRGLinkStatus(context.Background(), utils.GetFakeClient(), rg.DeepCopy(), replication.StorageProtectionGroupStatus_UNKNOWN.String(), false, "err")
	if err == nil {
		t.Fatalf("expected status update to fail for missing object")
	}
}

func TestReplicationGroupMonitoring_monitorReplicationGroupsWithoutAssociatedPVs(t *testing.T) {
	driver := utils.GetDefaultDriver()
	rg := utils.GetRGObj("rg-without-pv", driver.DriverName, driver.RemoteClusterID, utils.LocalPGID, utils.RemotePGID, nil, nil)
	cl := utils.GetFakeClientWithObjects(rg)
	monitor := &ReplicationGroupMonitoring{
		Client:             cl,
		DriverName:         driver.DriverName,
		ReplicationClient:  nil,
		MonitoringInterval: time.Second,
	}

	monitor.monitorReplicationGroups()

	updatedRG := &repv1.DellCSIReplicationGroup{}
	err := cl.Get(context.Background(), types.NamespacedName{Name: rg.Name}, updatedRG)
	if err != nil {
		t.Fatalf("expected to fetch updated RG, got %v", err)
	}
	if updatedRG.Status.ReplicationLinkState.State != replication.StorageProtectionGroupStatus_EMPTY.String() {
		t.Fatalf("expected RG link state to be EMPTY when no associated PVs exist")
	}
}

func TestReplicationGroupMonitoring_monitorReplicationGroupsSkipsUpdateWhenActionInProgress(t *testing.T) {
	driver := utils.GetDefaultDriver()
	rg := utils.GetRGObj("rg-action-in-progress", driver.DriverName, driver.RemoteClusterID, utils.LocalPGID, utils.RemotePGID, nil, nil)
	rg.Spec.Action = "Suspend"
	rg.Status.ReplicationLinkState.LastSuccessfulUpdate = &metav1.Time{Time: time.Now().Add(-5 * time.Second)}
	rg.Status.ReplicationLinkState.State = "old-status"
	pv := utils.GetPVObj("pv-rg-action-in-progress", "vol-handle", driver.DriverName, driver.StorageClass, nil)
	pv.Labels = map[string]string{
		controllers.DriverName:       driver.DriverName,
		controllers.ReplicationGroup: rg.Name,
	}
	cl := utils.GetFakeClientWithObjects(rg, pv)
	monitor := &ReplicationGroupMonitoring{
		Client:             cl,
		DriverName:         driver.DriverName,
		ReplicationClient:  nil,
		MonitoringInterval: time.Second,
	}

	monitor.monitorReplicationGroups()

	updatedRG := &repv1.DellCSIReplicationGroup{}
	err := cl.Get(context.Background(), types.NamespacedName{Name: rg.Name}, updatedRG)
	if err != nil {
		t.Fatalf("expected to fetch updated RG, got %v", err)
	}
	if updatedRG.Status.ReplicationLinkState.State != "old-status" {
		t.Fatalf("expected RG link state to remain unchanged while action is in progress")
	}
}
