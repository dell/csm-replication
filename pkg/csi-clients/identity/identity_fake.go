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

package identity

import (
	"context"
	"github.com/dell/dell-csi-extensions/replication"
)

type InjectedError struct {
	error        error
	clearAfter   int
	currentCount int
}

func (i InjectedError) SetError(err error, clearAfter int) {
	// Just overwrite any error which exists already
	i.currentCount = 0
	i.clearAfter = clearAfter
	i.error = err
}

func (i InjectedError) getAndClearError() error {
	if i.clearAfter == -1 {
		// Error which never clears
		return i.error
	}
	i.currentCount++
	err := i.error
	if i.currentCount >= i.clearAfter {
		i.error = nil
		i.currentCount = 0
		i.clearAfter = 0
	}
	return err
}

type MockIdentity struct {
	name             string
	injectedError    InjectedError
	capabilitySet    ReplicationCapabilitySet
	supportedActions []*replication.SupportedActions
}

func getSupportedActions() (ReplicationCapabilitySet, []*replication.SupportedActions) {
	capResponse := &replication.GetReplicationCapabilityResponse{}
	for i := 0; i < 3; i++ {
		capResponse.Capabilities = append(capResponse.Capabilities, &replication.ReplicationCapability{
			Type: &replication.ReplicationCapability_Rpc{
				Rpc: &replication.ReplicationCapability_RPC{
					Type: replication.ReplicationCapability_RPC_Type(i),
				},
			},
		})
	}
	capabilitySet := ReplicationCapabilitySet{}
	for _, capability := range capResponse.Capabilities {
		if capability == nil {
			continue
		}
		rpc := capability.GetRpc()
		if rpc == nil {
			continue
		}
		t := rpc.GetType()
		capabilitySet[t] = true
	}
	for i := 1; i < 20; i++ {
		capResponse.Actions = append(capResponse.Actions, &replication.SupportedActions{
			Actions: &replication.SupportedActions_Type{
				Type: replication.ActionTypes(i),
			},
		})
	}
	return capabilitySet, capResponse.Actions
}

func NewFakeIdentityClient(name string) Identity {
	capabilitySet, actions := getSupportedActions()
	return &MockIdentity{
		name:             name,
		injectedError:    InjectedError{},
		capabilitySet:    capabilitySet,
		supportedActions: actions,
	}
}

func (m *MockIdentity) ProbeController(ctx context.Context) (string, bool, error) {
	if err := m.injectedError.getAndClearError(); err != nil {
		return "", false, err
	}
	return m.name, true, nil
}

func (m *MockIdentity) ProbeForever(ctx context.Context) (string, error) {
	if err := m.injectedError.getAndClearError(); err != nil {
		return "", err
	}
	return m.name, nil
}

func (m *MockIdentity) GetReplicationCapabilities(ctx context.Context) (ReplicationCapabilitySet,
	[]*replication.SupportedActions, error) {
	if err := m.injectedError.getAndClearError(); err != nil {
		return ReplicationCapabilitySet{}, []*replication.SupportedActions{}, err
	}
	return m.capabilitySet, m.supportedActions, nil
}

func (m *MockIdentity) InjectError(err error) {
	m.injectedError.SetError(err, -1)
}

func (m *MockIdentity) InjectErrorAutoClear(err error) {
	m.injectedError.SetError(err, 1)
}

func (m *MockIdentity) InjectErrorClearAfterN(err error, clearAfter int) {
	m.injectedError.SetError(err, clearAfter)
}

func (m *MockIdentity) SetSupportedActions(supportedActions []*replication.SupportedActions) {
	m.supportedActions = supportedActions
}

func (m *MockIdentity) SetCapabilitySet(capabilitySet ReplicationCapabilitySet) {
	m.capabilitySet = capabilitySet
}
