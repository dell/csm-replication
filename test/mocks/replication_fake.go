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

package mocks

import (
	"context"
	"fmt"
	"path"

	csiext "github.com/dell/dell-csi-extensions/replication"
)

type injectedError struct {
	error        error
	clearAfter   int
	currentCount int
}

func (i *injectedError) setError(err error, clearAfter int) {
	// Just overwrite any error which exists already
	i.currentCount = 0
	i.clearAfter = clearAfter
	i.error = err
}

func (i *injectedError) clearError(force bool) {
	if force {
		i.clearAfter = 0
	}
	if i.clearAfter != -1 {
		if i.currentCount >= i.clearAfter {
			i.error = nil
			i.currentCount = 0
			i.clearAfter = 0
		}
	}
}

func (i *injectedError) getError() error {
	if i.clearAfter != -1 {
		i.currentCount++
	}
	return i.error
}

type conditionType string

const (
	// ExecuteActionWithoutStatus condition
	ExecuteActionWithoutStatus = conditionType("ExecuteActionWithoutStatus")
	// ExecuteActionWithoutSuccess condition
	ExecuteActionWithoutSuccess = conditionType("ExecuteActionWithoutSuccess")
	// ExecuteActionWithSwap condition
	ExecuteActionWithSwap = conditionType("ExecuteActionWithSwap")
	// CreatePGWithMissingPGID condition
	CreatePGWithMissingPGID = conditionType("CreatePGWithMissingPGID")
	// CreatePGWithOutStatus condition
	CreatePGWithOutStatus = conditionType("CreatePGWithOutStatus")
	// GetPGStatusForTarget condition
	GetPGStatusForTarget = conditionType("GetPGStatusForTarget")
	// GetPGStatusInProgress condition
	GetPGStatusInProgress = conditionType("GetPGStatusInProgress")
)

// MockReplication is dummy implementation of Replication interface
type MockReplication struct {
	contextPrefix string
	injectedError *injectedError
	conditionType conditionType
}

// NewFakeReplicationClient returns mock implementation of Replication interface
func NewFakeReplicationClient(contextPrefix string) MockReplication {
	return MockReplication{
		injectedError: &injectedError{},
		contextPrefix: contextPrefix,
	}
}

// GetStorageProtectionGroupStatus mocks call
func (m *MockReplication) GetStorageProtectionGroupStatus(_ context.Context,
	_ string, _ map[string]string,
) (*csiext.GetStorageProtectionGroupStatusResponse, error) {
	defer m.ClearErrorAndCondition(false)
	if err := m.injectedError.getError(); err != nil {
		return nil, err
	}
	response := csiext.GetStorageProtectionGroupStatusResponse{
		Status: &csiext.StorageProtectionGroupStatus{
			IsSource: true,
			State:    0,
		},
	}
	switch m.conditionType {
	case GetPGStatusForTarget:
		response.Status.IsSource = false
	case GetPGStatusInProgress:
		response.Status.State = 1
	}
	return &response, nil
}

// ExecuteAction mocks call
func (m *MockReplication) ExecuteAction(_ context.Context, _ string,
	actionType *csiext.ExecuteActionRequest_Action, _ map[string]string,
	_ string, _ map[string]string,
) (*csiext.ExecuteActionResponse, error) {
	defer m.ClearErrorAndCondition(false)
	if err := m.injectedError.getError(); err != nil {
		return nil, err
	}
	response := csiext.ExecuteActionResponse{
		Success: true,
		ActionTypes: &csiext.ExecuteActionResponse_Action{
			Action: actionType.Action,
		},
		Status: &csiext.StorageProtectionGroupStatus{
			IsSource: true,
			State:    0,
		},
	}
	switch m.conditionType {
	case ExecuteActionWithoutStatus:
		response.Status = nil
	case ExecuteActionWithoutSuccess:
		response.Success = false
	case ExecuteActionWithSwap:
		response.Status.IsSource = false
		response.Status.State = 2
	}
	return &response, nil
}

// CreateRemoteVolume mocks call
func (m *MockReplication) CreateRemoteVolume(_ context.Context, volumeHandle string,
	params map[string]string,
) (*csiext.CreateRemoteVolumeResponse, error) {
	defer m.ClearErrorAndCondition(false)
	if err := m.injectedError.getError(); err != nil {
		return nil, err
	}
	volParams := params
	volParams[path.Join(m.contextPrefix, "arrayParam1")] = "arrayVal1"
	volParams[path.Join(m.contextPrefix, "arrayParam2")] = "arrayVal2"
	response := csiext.CreateRemoteVolumeResponse{
		RemoteVolume: &csiext.Volume{
			CapacityBytes: 1024 * 1024 * 1024,
			VolumeId:      fmt.Sprintf("remote-%s-fake-volume", volumeHandle),
			VolumeContext: volParams,
		},
	}
	return &response, nil
}

// DeleteLocalVolume mocks call
func (m *MockReplication) DeleteLocalVolume(_ context.Context, _ string,
	_ map[string]string,
) (*csiext.DeleteLocalVolumeResponse, error) {
	defer m.ClearErrorAndCondition(false)
	if err := m.injectedError.getError(); err != nil {
		return nil, err
	}
	response := csiext.DeleteLocalVolumeResponse{}
	return &response, nil
}

// CreateStorageProtectionGroup mocks call
func (m *MockReplication) CreateStorageProtectionGroup(_ context.Context, _ string,
	_ map[string]string,
) (*csiext.CreateStorageProtectionGroupResponse, error) {
	defer m.ClearErrorAndCondition(false)
	if err := m.injectedError.getError(); err != nil {
		return nil, err
	}
	localParams := make(map[string]string)
	localParams["local"] = "X"
	localParams["remote"] = "Y"
	// Some params with context prefix
	localParams[path.Join(m.contextPrefix, "arrayParam1")] = "arrayVal1"
	localParams[path.Join(m.contextPrefix, "arrayParam2")] = "arrayVal2"
	remoteParams := make(map[string]string)
	remoteParams["local"] = "Y"
	remoteParams["remote"] = "Y"
	remoteParams[path.Join(m.contextPrefix, "arrayParam3")] = "arrayVal3"
	remoteParams[path.Join(m.contextPrefix, "arrayParam4")] = "arrayVal4"
	response := csiext.CreateStorageProtectionGroupResponse{
		LocalProtectionGroupId:          "localPGID",
		RemoteProtectionGroupId:         "remotePGID",
		LocalProtectionGroupAttributes:  localParams,
		RemoteProtectionGroupAttributes: remoteParams,
		Status: &csiext.StorageProtectionGroupStatus{
			IsSource: true,
			State:    0,
		},
	}
	switch m.conditionType {
	case CreatePGWithMissingPGID:
		response.LocalProtectionGroupId = ""
	case CreatePGWithOutStatus:
		response.Status = nil
	}
	return &response, nil
}

// DeleteStorageProtectionGroup mocks call
func (m *MockReplication) DeleteStorageProtectionGroup(_ context.Context, _ string,
	_ map[string]string,
) error {
	defer m.ClearErrorAndCondition(false)
	return m.injectedError.getError()
}

// InjectError injects error
func (m *MockReplication) InjectError(err error) {
	m.injectedError.setError(err, -1)
}

// InjectErrorAutoClear injects error and clears after 1 try
func (m *MockReplication) InjectErrorAutoClear(err error) {
	m.injectedError.setError(err, 1)
}

// InjectErrorClearAfterN injects error and clears after N tries
func (m *MockReplication) InjectErrorClearAfterN(err error, clearAfter int) {
	m.injectedError.setError(err, clearAfter)
}

// SetCondition sets condition to provided condition type
func (m *MockReplication) SetCondition(conditionType conditionType) {
	m.conditionType = conditionType
}

// ClearErrorAndCondition clears injected error and resets condition
func (m *MockReplication) ClearErrorAndCondition(force bool) {
	m.conditionType = ""
	m.injectedError.clearError(force)
}
