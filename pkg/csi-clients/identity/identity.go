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
 Copyright © 2021-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"time"

	"github.com/dell/dell-csi-extensions/migration"

	"github.com/dell/csm-replication/pkg/common/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonext "github.com/dell/dell-csi-extensions/common"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
)

// ReplicationCapabilitySet represents map of supported replication capabilities
type ReplicationCapabilitySet map[replication.ReplicationCapability_RPC_Type]bool

// MigrationCapabilitySet represents map of supported migration capabilities
type MigrationCapabilitySet map[migration.MigrateTypes]bool

// Identity is an interface that defines calls used for querying identity and state of the driver
type Identity interface {
	ProbeController(ctx context.Context) (string, bool, error)
	ProbeForever(ctx context.Context) (string, error)
	GetReplicationCapabilities(ctx context.Context) (ReplicationCapabilitySet, []*replication.SupportedActions, error)
	GetMigrationCapabilities(ctx context.Context) (MigrationCapabilitySet, error)
}

var (
	getClientProbeController = func(client replication.ReplicationClient, ctx context.Context, in *commonext.ProbeControllerRequest, opts ...grpc.CallOption) (*commonext.ProbeControllerResponse, error) {
		return client.ProbeController(ctx, in, opts...)
	}
	getClientGetReplicationCapabilities = func(client replication.ReplicationClient, ctx context.Context, in *replication.GetReplicationCapabilityRequest, opts ...grpc.CallOption) (*replication.GetReplicationCapabilityResponse, error) {
		return client.GetReplicationCapabilities(ctx, in, opts...)
	}
	getClientGetMigrationCapabilities = func(client migration.MigrationClient, ctx context.Context, in *migration.GetMigrationCapabilityRequest, opts ...grpc.CallOption) (*migration.GetMigrationCapabilityResponse, error) {
		return client.GetMigrationCapabilities(ctx, in, opts...)
	}
	getProbeController = func(r *identity, ctx context.Context) (string, bool, error) {
		return r.ProbeController(ctx)
	}
)

// New return new Identity interface implementation
func New(conn *grpc.ClientConn, log logr.Logger, timeout time.Duration, frequency time.Duration) Identity {
	return &identity{
		conn:      conn,
		log:       log,
		timeout:   timeout,
		frequency: frequency,
	}
}

type identity struct {
	conn      *grpc.ClientConn
	log       logr.Logger
	timeout   time.Duration
	frequency time.Duration
}

// ProbeController queries driver controller
func (r *identity) ProbeController(ctx context.Context) (string, bool, error) {
	r.log.V(logger.InfoLevel).Info("Probing controller")
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	client := replication.NewReplicationClient(r.conn)

	response, err := getClientProbeController(client, tctx, &commonext.ProbeControllerRequest{})
	if err != nil {
		return "", false, err
	}
	driverName := response.GetName()
	ready := response.GetReady()
	if ready == nil {
		// In case there neither an error nor a response,
		// assuming the plugin is in ready state.
		return driverName, true, nil
	}
	return driverName, ready.GetValue(), nil
}

// ProbeForever launches loop that continuously queries controller state
func (r *identity) ProbeForever(ctx context.Context) (string, error) {
	for {
		r.log.V(logger.DebugLevel).Info("Probing driver for readiness")
		driverName, ready, err := getProbeController(r, ctx)
		if err != nil {
			st, ok := status.FromError(err)
			if !ok {
				// Not a grpc error; probe failed before grpc method was called
				return "", err
			}
			if st.Code() != codes.DeadlineExceeded {
				return "", err
			}
			r.log.V(logger.InfoLevel).Info("CSI driver probe timed out")
		} else {
			if ready {
				return driverName, nil
			}
			r.log.V(logger.InfoLevel).Info("CSI Driver not ready yet")
		}
		time.Sleep(r.frequency)
	}
}

// GetReplicationCapabilities queries driver for supported replication capabilities
func (r *identity) GetReplicationCapabilities(ctx context.Context) (ReplicationCapabilitySet,
	[]*replication.SupportedActions, error,
) {
	r.log.V(logger.InfoLevel).Info("Requesting replication capabilities")
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	client := replication.NewReplicationClient(r.conn)

	response, err := getClientGetReplicationCapabilities(client, tctx, &replication.GetReplicationCapabilityRequest{})
	if err != nil {
		return nil, nil, err
	}
	capabilitySet := ReplicationCapabilitySet{}
	for _, capability := range response.Capabilities {
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
	return capabilitySet, response.Actions, nil
}

func (r *identity) GetMigrationCapabilities(ctx context.Context) (MigrationCapabilitySet, error) {
	r.log.V(logger.InfoLevel).Info("Requesting migration capabilities")
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	client := migration.NewMigrationClient(r.conn)

	response, err := getClientGetMigrationCapabilities(client, tctx, &migration.GetMigrationCapabilityRequest{})
	if err != nil {
		return nil, err
	}
	capabilitySet := MigrationCapabilitySet{}
	for _, capability := range response.Capabilities {
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
	return capabilitySet, nil
}
