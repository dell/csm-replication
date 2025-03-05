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

package identity

import (
	"context"
	"testing"
	"time"

	"github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

func Test_identity_ProbeForever(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		r       *identity
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "ProbeForever Failed",
			r:       &identity{conn: createFakeConnection(), log: ctrl.Log.WithName("identity.v1.Identity/ProbeForever"), timeout: 10, frequency: 10},
			args:    args{ctx},
			want:    "",
			wantErr: true,
		},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.r.ProbeForever(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("identity.ProbeForever() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("identity.ProbeForever() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProbeController(t *testing.T) {
	mockIdentity := &MockIdentity{}
	mockIdentity.On("ProbeController", mock.Anything).Return("mock-driver-name", true, nil)

	// Call the function you want to test
	driverName, ready, err := mockIdentity.ProbeController(context.Background())

	// Assert that the function returned the expected result
	assert.NoError(t, err)
	assert.Equal(t, "mock-driver-name", driverName)
	assert.Equal(t, true, ready)

	// Assert that the mock object was called as expected
	mockIdentity.AssertExpectations(t)
}

type MockIdentity struct {
	mock.Mock
}

// GetMigrationCapabilities provides a mock function with given fields: ctx
func (_m *MockIdentity) GetMigrationCapabilities(ctx context.Context) (MigrationCapabilitySet, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetMigrationCapabilities")
	}

	var r0 MigrationCapabilitySet
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (MigrationCapabilitySet, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) MigrationCapabilitySet); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(MigrationCapabilitySet)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetReplicationCapabilities provides a mock function with given fields: ctx
func (_m *MockIdentity) GetReplicationCapabilities(ctx context.Context) (ReplicationCapabilitySet, []*replication.SupportedActions, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetReplicationCapabilities")
	}

	var r0 ReplicationCapabilitySet
	var r1 []*replication.SupportedActions
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context) (ReplicationCapabilitySet, []*replication.SupportedActions, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) ReplicationCapabilitySet); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(ReplicationCapabilitySet)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) []*replication.SupportedActions); ok {
		r1 = rf(ctx)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).([]*replication.SupportedActions)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context) error); ok {
		r2 = rf(ctx)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// ProbeController provides a mock function with given fields: ctx
func (_m *MockIdentity) ProbeController(ctx context.Context) (string, bool, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for ProbeController")
	}

	var r0 string
	var r1 bool
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context) (string, bool, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context) bool); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Get(1).(bool)
	}

	if rf, ok := ret.Get(2).(func(context.Context) error); ok {
		r2 = rf(ctx)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// ProbeForever provides a mock function with given fields: ctx
func (_m *MockIdentity) ProbeForever(ctx context.Context) (string, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for ProbeForever")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (string, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewIdentity creates a new instance of MockIdentity. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewIdentity(t interface {
	mock.TestingT
	Cleanup(func())
},
) *MockIdentity {
	mock := &MockIdentity{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
