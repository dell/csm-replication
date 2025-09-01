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
 Copyright © 2022-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"time"

	csiext "github.com/dell/dell-csi-extensions/migration"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
)

// Migration is an interface that defines calls used for querying migration management calls to the driver
type Migration interface {
	VolumeMigrate(context.Context, string, string, *csiext.VolumeMigrateRequest_Type, map[string]string, map[string]string, bool) (*csiext.VolumeMigrateResponse, error)
	ArrayMigrate(context.Context, *csiext.ArrayMigrateRequest_Action, map[string]string) (*csiext.ArrayMigrateResponse, error)
}

// New returns new implementation of Replication interface
func New(conn *grpc.ClientConn, log logr.Logger, timeout time.Duration) Migration {
	return &migration{
		conn:    conn,
		log:     log,
		timeout: timeout,
	}
}

type migration struct {
	conn    *grpc.ClientConn
	log     logr.Logger
	timeout time.Duration
}

// VolumeMigrate migrate volume
func (m *migration) VolumeMigrate(ctx context.Context, volumeHandle string, storageClass string, migrateType *csiext.VolumeMigrateRequest_Type, scParams map[string]string, scSourceParams map[string]string, toClone bool) (*csiext.VolumeMigrateResponse, error) {
	tc, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	client := csiext.NewMigrationClient(m.conn)

	req := &csiext.VolumeMigrateRequest{
		VolumeHandle:       volumeHandle,
		StorageClass:       storageClass,
		MigrateTypes:       migrateType,
		ScParameters:       scParams,
		ScSourceParameters: scSourceParams,
		ShouldClone:        toClone,
	}

	res, err := client.VolumeMigrate(tc, req)
	return res, err
}

// ArrayMigrate
func (m *migration) ArrayMigrate(ctx context.Context, migrateAction *csiext.ArrayMigrateRequest_Action, Params map[string]string) (*csiext.ArrayMigrateResponse, error) {
	tc, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	client := csiext.NewMigrationClient(m.conn)

	req := &csiext.ArrayMigrateRequest{
		ActionTypes: migrateAction,
		Parameters:  Params,
	}

	res, err := client.ArrayMigrate(tc, req)
	return res, err
}
