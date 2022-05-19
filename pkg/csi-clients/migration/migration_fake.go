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

package migration

import (
	"context"

	csiext "github.com/dell/dell-csi-extensions/migration"
)

type injectedError struct {
	error        error
	clearAfter   int
	currentCount int
}

func (i *injectedError) getError() error {
	if i.clearAfter != -1 {
		i.currentCount++
	}
	return i.error
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
func (m *MockMigration) VolumeMigrate(ctx context.Context, volumeHandle string, storageClass string, migrateType *csiext.VolumeMigrateRequest_Type,
	scParams map[string]string, sourcescParams map[string]string, toClone bool) (*csiext.VolumeMigrateResponse, error) {

	defer m.ClearError(false)
	if err := m.injectedError.getError(); err != nil {
		return nil, err
	}

	response := csiext.VolumeMigrateResponse{
		MigratedVolume: &csiext.Volume{},
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
