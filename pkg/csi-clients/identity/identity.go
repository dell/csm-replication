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
	"github.com/dell/csm-replication/pkg/common"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
)

type ReplicationCapabilitySet map[replication.ReplicationCapability_RPC_Type]bool

type Identity interface {
	ProbeController(ctx context.Context) (string, bool, error)
	ProbeForever(ctx context.Context) (string, error)
	GetReplicationCapabilities(ctx context.Context) (ReplicationCapabilitySet, []*replication.SupportedActions, error)
}

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

func (r *identity) ProbeController(ctx context.Context) (string, bool, error) {
	r.log.V(common.InfoLevel).Info("Probing controller")
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	client := replication.NewReplicationClient(r.conn)

	response, err := client.ProbeController(tctx, &replication.ProbeControllerRequest{})
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

func (r *identity) ProbeForever(ctx context.Context) (string, error) {
	for {
		r.log.V(common.DebugLevel).Info("Probing driver for readiness")
		driverName, ready, err := r.ProbeController(ctx)
		if err != nil {
			st, ok := status.FromError(err)
			if !ok {
				// Not a grpc error; probe failed before grpc method was called
				return "", err
			}
			if st.Code() != codes.DeadlineExceeded {
				return "", err
			}
			r.log.V(common.InfoLevel).Info("CSI driver probe timed out")
		} else {
			if ready {
				return driverName, nil
			}
			r.log.V(common.InfoLevel).Info("CSI Driver not ready yet")
		}
		time.Sleep(r.frequency)
	}
}

func (r *identity) GetReplicationCapabilities(ctx context.Context) (ReplicationCapabilitySet,
	[]*replication.SupportedActions, error) {
	r.log.V(common.InfoLevel).Info("Requesting replication capabilities")
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	client := replication.NewReplicationClient(r.conn)

	response, err := client.GetReplicationCapabilities(tctx, &replication.GetReplicationCapabilityRequest{})
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
