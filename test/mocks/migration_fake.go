/*
 Copyright © 2022-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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

	csiext "github.com/dell/dell-csi-extensions/migration"
)

// MockMigration is dummy implementation of Migration interface
type MockMigration struct {
	contextPrefix string
	injectedError *injectedError
}

// NewFakeMigrationClient returns mock implementation of Migration interface
func NewFakeMigrationClient(contextPrefix string) MockMigration {
	return MockMigration{
		injectedError: &injectedError{},
		contextPrefix: contextPrefix,
	}
}

// VolumeMigrate migrates volume
func (m *MockMigration) VolumeMigrate(_ context.Context, _ string, _ string, _ *csiext.VolumeMigrateRequest_Type,
	_ map[string]string, _ map[string]string, _ bool,
) (*csiext.VolumeMigrateResponse, error) {
	defer m.ClearError(false)
	if err := m.injectedError.getError(); err != nil {
		return nil, err
	}

	response := csiext.VolumeMigrateResponse{
		MigratedVolume: &csiext.Volume{},
	}

	return &response, nil
}

// ArrayMigrate migrates volume from source array to target
func (m *MockMigration) ArrayMigrate(_ context.Context, migrateAction *csiext.ArrayMigrateRequest_Action, _ map[string]string) (*csiext.ArrayMigrateResponse, error) {
	defer m.ClearError(false)
	if err := m.injectedError.getError(); err != nil {
		return nil, err
	}

	response := csiext.ArrayMigrateResponse{
		Success: true,
		ActionTypes: &csiext.ArrayMigrateResponse_Action{
			Action: migrateAction.Action,
		},
	}

	return &response, nil
}

// ClearError clears injected error
func (m *MockMigration) ClearError(force bool) {
	m.injectedError.clearError(force)
}

// InjectError injects error
func (m *MockMigration) InjectError(err error) {
	m.injectedError.setError(err, -1)
}
