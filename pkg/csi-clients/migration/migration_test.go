/*
 Copyright © 2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"testing"
	"time"

	csiext "github.com/dell/dell-csi-extensions/migration"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type mockMigrationClient struct{}

func (m *mockMigrationClient) VolumeMigrate(_ context.Context, _ *csiext.VolumeMigrateRequest, _ ...grpc.CallOption) (*csiext.VolumeMigrateResponse, error) {
	return &csiext.VolumeMigrateResponse{}, nil
}

func (m *mockMigrationClient) ArrayMigrate(_ context.Context, _ *csiext.ArrayMigrateRequest, _ ...grpc.CallOption) (*csiext.ArrayMigrateResponse, error) {
	return &csiext.ArrayMigrateResponse{}, nil
}

func createFakeConnection() *grpc.ClientConn {
	conn, _ := grpc.NewClient("localhost:50055", grpc.WithTransportCredentials(insecure.NewCredentials()))
	return conn
}

func TestVolumeMigrate(t *testing.T) {
	tests := []struct {
		name           string
		volumeHandle   string
		storageClass   string
		migrateType    *csiext.VolumeMigrateRequest_Type
		scParams       map[string]string
		scSourceParams map[string]string
		toClone        bool
		expectedError  string
	}{
		{
			name:           "Failed to migrate volume",
			volumeHandle:   "vol1",
			storageClass:   "sc1",
			migrateType:    &csiext.VolumeMigrateRequest_Type{},
			scParams:       map[string]string{"param1": "value1"},
			scSourceParams: map[string]string{"sourceParam1": "sourceValue1"},
			toClone:        true,
			expectedError:  "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &migration{
				conn:    createFakeConnection(),
				log:     logr.Discard(),
				timeout: 5 * time.Second,
			}

			ctx := context.Background()
			_, err := m.VolumeMigrate(ctx, tt.volumeHandle, tt.storageClass, tt.migrateType, tt.scParams, tt.scSourceParams, tt.toClone)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestArrayMigrate(t *testing.T) {
	tests := []struct {
		name          string
		migrateAction *csiext.ArrayMigrateRequest_Action
		params        map[string]string
		expectedError string
	}{
		{
			name:          "Failed to migrate array",
			migrateAction: &csiext.ArrayMigrateRequest_Action{},
			params:        map[string]string{"param1": "value1"},
			expectedError: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &migration{
				conn:    createFakeConnection(),
				log:     logr.Discard(),
				timeout: 5 * time.Second,
			}

			ctx := context.Background()
			_, err := m.ArrayMigrate(ctx, tt.migrateAction, tt.params)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}
