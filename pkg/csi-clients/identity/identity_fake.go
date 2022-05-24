/*
 Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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

type injectedError struct {
	error        error
	clearAfter   int
	currentCount int
}

func (i injectedError) setError(err error, clearAfter int) {
	// Just overwrite any error which exists already
	i.currentCount = 0
	i.clearAfter = clearAfter
	i.error = err
}

func (i injectedError) getAndClearError() error {
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

type mockIdentity struct {
	name             string
	injectedError    injectedError
	capabilitySet    ReplicationCapabilitySet
	supportedActions []*replication.SupportedActions
}

func (m *mockIdentity) GetMigrationCapabilities(ctx context.Context) (MigrationCapabilitySet, error) {
	//TODO implement me
	panic("implement me")
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

// NewFakeIdentityClient returns fake identity client
func NewFakeIdentityClient(name string) Identity {
	capabilitySet, actions := getSupportedActions()
	return &mockIdentity{
		name:             name,
		injectedError:    injectedError{},
		capabilitySet:    capabilitySet,
		supportedActions: actions,
	}
}

func (m *mockIdentity) ProbeController(ctx context.Context) (string, bool, error) {
	if err := m.injectedError.getAndClearError(); err != nil {
		return "", false, err
	}
	return m.name, true, nil
}

func (m *mockIdentity) ProbeForever(ctx context.Context) (string, error) {
	if err := m.injectedError.getAndClearError(); err != nil {
		return "", err
	}
	return m.name, nil
}

func (m *mockIdentity) GetReplicationCapabilities(ctx context.Context) (ReplicationCapabilitySet,
	[]*replication.SupportedActions, error) {
	if err := m.injectedError.getAndClearError(); err != nil {
		return ReplicationCapabilitySet{}, []*replication.SupportedActions{}, err
	}
	return m.capabilitySet, m.supportedActions, nil
}

func (m *mockIdentity) InjectError(err error) {
	m.injectedError.setError(err, -1)
}

func (m *mockIdentity) InjectErrorAutoClear(err error) {
	m.injectedError.setError(err, 1)
}

func (m *mockIdentity) InjectErrorClearAfterN(err error, clearAfter int) {
	m.injectedError.setError(err, clearAfter)
}

func (m *mockIdentity) SetSupportedActions(supportedActions []*replication.SupportedActions) {
	m.supportedActions = supportedActions
}

func (m *mockIdentity) SetCapabilitySet(capabilitySet ReplicationCapabilitySet) {
	m.capabilitySet = capabilitySet
}
