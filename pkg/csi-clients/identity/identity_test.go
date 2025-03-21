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

package identity

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	commonext "github.com/dell/dell-csi-extensions/common"
	"github.com/dell/dell-csi-extensions/migration"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
	ctrl "sigs.k8s.io/controller-runtime"
)

func createFakeConnection() *grpc.ClientConn {
	conn, _ := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	return conn
}

func TestNew(t *testing.T) {
	// Create a fake gRPC connection
	conn := &grpc.ClientConn{}

	// Create a fake logger
	log := logr.Discard()

	// Create a fake timeout and frequency
	timeout := time.Second
	frequency := time.Minute

	// Call the New function
	result := New(conn, log, timeout, frequency)

	// Assert that the result is not nil
	if result == nil {
		t.Errorf("Expected a non-nil result, but got nil")
	}

	// Assert that the result is of type *identity
	_, ok := result.(*identity)
	if !ok {
		t.Errorf("Expected a result of type *identity, but got %T", result)
	}

	// Assert that the result has the correct connection
	if result.(*identity).conn != conn {
		t.Errorf("Expected the result to have connection %v, but got %v", conn, result.(*identity).conn)
	}

	// Assert that the result has the correct logger
	if result.(*identity).log != log {
		t.Errorf("Expected the result to have logger %v, but got %v", log, result.(*identity).log)
	}

	// Assert that the result has the correct timeout
	if result.(*identity).timeout != timeout {
		t.Errorf("Expected the result to have timeout %v, but got %v", timeout, result.(*identity).timeout)
	}

	// Assert that the result has the correct frequency
	if result.(*identity).frequency != frequency {
		t.Errorf("Expected the result to have frequency %v, but got %v", frequency, result.(*identity).frequency)
	}
}

func Test_identity_ProbeController(t *testing.T) {
	originalGetClientProbeController := getClientProbeController

	after := func() {
		getClientProbeController = originalGetClientProbeController
	}

	type fields struct {
		conn      *grpc.ClientConn
		log       logr.Logger
		timeout   time.Duration
		frequency time.Duration
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		setup   func()
		want    string
		want1   bool
		wantErr bool
	}{
		{
			name:   "ProbeController Failed",
			fields: fields{createFakeConnection(), ctrl.Log.WithName("identity.v1.Identity/ProbeController"), time.Second, time.Second},
			args:   args{context.Background()},
			setup: func() {
				getClientProbeController = func(_ replication.ReplicationClient, _ context.Context, _ *commonext.ProbeControllerRequest, _ ...grpc.CallOption) (*commonext.ProbeControllerResponse, error) {
					return nil, errors.New("error")
				}
			},
			want:    "",
			want1:   false,
			wantErr: true,
		},
		{
			name:   "ProbeController Success",
			fields: fields{createFakeConnection(), ctrl.Log.WithName("identity.v1.Identity/ProbeController"), time.Second, time.Second},
			args:   args{context.Background()},
			setup: func() {
				getClientProbeController = func(_ replication.ReplicationClient, _ context.Context, _ *commonext.ProbeControllerRequest, _ ...grpc.CallOption) (*commonext.ProbeControllerResponse, error) {
					return &commonext.ProbeControllerResponse{
						Name: "test",
						Ready: &wrapperspb.BoolValue{
							Value: true,
						},
					}, nil
				}
			},
			want:    "test",
			want1:   true,
			wantErr: false,
		},
		{
			name:   "ProbeController Success (ready = nil)",
			fields: fields{createFakeConnection(), ctrl.Log.WithName("identity.v1.Identity/ProbeController"), time.Second, time.Second},
			args:   args{context.Background()},
			setup: func() {
				getClientProbeController = func(_ replication.ReplicationClient, _ context.Context, _ *commonext.ProbeControllerRequest, _ ...grpc.CallOption) (*commonext.ProbeControllerResponse, error) {
					return &commonext.ProbeControllerResponse{
						Name:  "test",
						Ready: nil,
					}, nil
				}
			},
			want:    "test",
			want1:   true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			if tt.setup != nil {
				tt.setup()
			}
			r := &identity{
				conn:      tt.fields.conn,
				log:       tt.fields.log,
				timeout:   tt.fields.timeout,
				frequency: tt.fields.frequency,
			}
			got, got1, err := r.ProbeController(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("identity.ProbeController() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("identity.ProbeController() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("identity.ProbeController() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_identity_GetReplicationCapabilities(t *testing.T) {
	originalGetClientGetReplicationCapabilities := getClientGetReplicationCapabilities

	after := func() {
		getClientGetReplicationCapabilities = originalGetClientGetReplicationCapabilities
	}

	type fields struct {
		conn      *grpc.ClientConn
		log       logr.Logger
		timeout   time.Duration
		frequency time.Duration
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		setup   func()
		want    ReplicationCapabilitySet
		want1   []*replication.SupportedActions
		wantErr bool
	}{
		{
			name:   "GetReplicationCapabilities Failed",
			fields: fields{createFakeConnection(), ctrl.Log.WithName("identity.v1.Identity/GetReplicationCapabilities"), time.Second, time.Second},
			args:   args{context.Background()},
			setup: func() {
				getClientGetReplicationCapabilities = func(_ replication.ReplicationClient, _ context.Context, _ *replication.GetReplicationCapabilityRequest, _ ...grpc.CallOption) (*replication.GetReplicationCapabilityResponse, error) {
					return nil, errors.New("error")
				}
			},
			want:    nil,
			want1:   nil,
			wantErr: true,
		},
		{
			name:   "GetReplicationCapabilities Passed",
			fields: fields{createFakeConnection(), ctrl.Log.WithName("identity.v1.Identity/GetReplicationCapabilities"), time.Second, time.Second},
			args:   args{context.Background()},
			setup: func() {
				getClientGetReplicationCapabilities = func(_ replication.ReplicationClient, _ context.Context, _ *replication.GetReplicationCapabilityRequest, _ ...grpc.CallOption) (*replication.GetReplicationCapabilityResponse, error) {
					return &replication.GetReplicationCapabilityResponse{
						Capabilities: []*replication.ReplicationCapability{
							nil,
							{
								Type: &replication.ReplicationCapability_Rpc{
									Rpc: nil,
								},
							},
							{
								Type: &replication.ReplicationCapability_Rpc{
									Rpc: &replication.ReplicationCapability_RPC{
										Type: replication.ReplicationCapability_RPC_CREATE_REMOTE_VOLUME,
									},
								},
							},
						},
						Actions: []*replication.SupportedActions{
							{
								Actions: &replication.SupportedActions_Type{
									Type: replication.ActionTypes_CREATE_SNAPSHOT,
								},
							},
						},
					}, nil
				}
			},
			want: ReplicationCapabilitySet{
				replication.ReplicationCapability_RPC_CREATE_REMOTE_VOLUME: true,
			},
			want1: []*replication.SupportedActions{
				{
					Actions: &replication.SupportedActions_Type{
						Type: replication.ActionTypes_CREATE_SNAPSHOT,
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			if tt.setup != nil {
				tt.setup()
			}

			r := &identity{
				conn:      tt.fields.conn,
				log:       tt.fields.log,
				timeout:   tt.fields.timeout,
				frequency: tt.fields.frequency,
			}
			got, got1, err := r.GetReplicationCapabilities(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("identity.GetReplicationCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("identity.GetReplicationCapabilities() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("identity.GetReplicationCapabilities() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_identity_GetMigrationCapabilities(t *testing.T) {
	originalGetClientGetMigrationCapabilities := getClientGetMigrationCapabilities
	after := func() {
		getClientGetMigrationCapabilities = originalGetClientGetMigrationCapabilities
	}
	type fields struct {
		conn      *grpc.ClientConn
		log       logr.Logger
		timeout   time.Duration
		frequency time.Duration
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		setup   func()
		want    MigrationCapabilitySet
		wantErr bool
	}{
		{
			name:   "GetMigrationCapabilities Failed",
			fields: fields{createFakeConnection(), ctrl.Log.WithName("identity.v1.Identity/GetMigrationCapabilities"), time.Second, time.Second},
			args:   args{context.Background()},
			setup: func() {
				getClientGetMigrationCapabilities = func(_ migration.MigrationClient, _ context.Context, _ *migration.GetMigrationCapabilityRequest, _ ...grpc.CallOption) (*migration.GetMigrationCapabilityResponse, error) {
					return nil, errors.New("error")
				}
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "GetMigrationCapabilities Failed",
			fields: fields{createFakeConnection(), ctrl.Log.WithName("identity.v1.Identity/GetMigrationCapabilities"), time.Second, time.Second},
			args:   args{context.Background()},
			setup: func() {
				getClientGetMigrationCapabilities = func(_ migration.MigrationClient, _ context.Context, _ *migration.GetMigrationCapabilityRequest, _ ...grpc.CallOption) (*migration.GetMigrationCapabilityResponse, error) {
					return &migration.GetMigrationCapabilityResponse{
						Capabilities: []*migration.MigrationCapability{
							nil,
							{
								Type: &migration.MigrationCapability_Rpc{
									Rpc: nil,
								},
							},
							{
								Type: &migration.MigrationCapability_Rpc{
									Rpc: &migration.MigrationCapability_RPC{
										Type: migration.MigrateTypes_NON_REPL_TO_REPL,
									},
								},
							},
						},
					}, nil
				}
			},
			want: MigrationCapabilitySet{
				migration.MigrateTypes_NON_REPL_TO_REPL: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			if tt.setup != nil {
				tt.setup()
			}
			r := &identity{
				conn:      tt.fields.conn,
				log:       tt.fields.log,
				timeout:   tt.fields.timeout,
				frequency: tt.fields.frequency,
			}
			got, err := r.GetMigrationCapabilities(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("identity.GetMigrationCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("identity.GetMigrationCapabilities() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProbeForever(t *testing.T) {
	// Create a mock gRPC connection
	mockConn := createFakeConnection()

	// Create a mock getProbeController function
	mockGetProbeController := func(_ *identity, _ context.Context) (string, bool, error) {
		return "test-driver", true, nil
	}

	// Create a new identity instance
	r := &identity{
		conn:      mockConn,
		log:       ctrl.Log.WithName("identity.v1.Identity/ProbeForever"),
		timeout:   time.Second,
		frequency: time.Second,
	}

	// Set the getProbeController function to the mock function
	getProbeController = mockGetProbeController

	// Call the ProbeForever method
	driverName, err := r.ProbeForever(context.Background())

	// Assert the result
	assert.NoError(t, err)
	assert.Equal(t, "test-driver", driverName)
}

func TestProbeForever_Failure(t *testing.T) {
	// Create a mock gRPC connection
	mockConn := createFakeConnection()

	// Create a mock getProbeController function
	mockGetProbeController := func(_ *identity, _ context.Context) (string, bool, error) {
		return "", false, errors.New("failed to get probe controller")
	}

	// Create a new identity instance
	r := &identity{
		conn:      mockConn,
		log:       ctrl.Log.WithName("identity.v1.Identity/ProbeForever"),
		timeout:   time.Second,
		frequency: time.Second,
	}

	// Set the getProbeController function to the mock function
	getProbeController = mockGetProbeController

	// Call the ProbeForever method
	driverName, err := r.ProbeForever(context.Background())

	// Assert the result
	assert.Error(t, err)
	assert.Equal(t, "", driverName)
}

func TestProbeForever_FailureStatus(t *testing.T) {
	// Create a mock gRPC connection
	mockConn := createFakeConnection()

	// Create a mock getProbeController function
	mockGetProbeController := func(_ *identity, _ context.Context) (string, bool, error) {
		return "", false, status.Error(codes.FailedPrecondition, "failed to probe controller")
	}

	// Create a new identity instance
	r := &identity{
		conn:      mockConn,
		log:       ctrl.Log.WithName("identity.v1.Identity/ProbeForever"),
		timeout:   time.Second,
		frequency: time.Second,
	}

	// Set the getProbeController function to the mock function
	getProbeController = mockGetProbeController

	// Call the ProbeForever method
	driverName, err := r.ProbeForever(context.Background())

	// Assert the result
	assert.Error(t, err)
	assert.Equal(t, "", driverName)
	assert.Equal(t, "rpc error: code = FailedPrecondition desc = failed to probe controller", err.Error())
}
