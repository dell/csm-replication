/*
 *
 * Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

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
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	csireplication "github.com/dell/csm-replication/test/mocks"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/stretchr/testify/suite"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
		Log:                ctrl.Log.WithName("controllers").WithName("Monitoring"),
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
	err := suite.rgMonitor.Monitor(context.Background())
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
