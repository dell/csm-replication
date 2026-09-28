/*
Copyright © 2025-2026 Dell Inc. or its subsidiaries. All Rights Reserved.

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
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	controller "github.com/dell/csm-replication/controllers/csi-node-rescanner"
	"github.com/dell/csm-replication/pkg/common/constants"
	"github.com/dell/csm-replication/pkg/config"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	"github.com/dell/csmlog"
	"github.com/dell/dell-csi-extensions/migration"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimeconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

func TestGetCSIConn(t *testing.T) {
	defaultGetConnection := getConnection
	tests := []struct {
		name        string
		csiAddress  string
		setup       func()
		expectedErr error
	}{
		{
			name:        "success",
			csiAddress:  "/var/run/csi.sock",
			expectedErr: nil,
		},
		{
			name:       "failure",
			csiAddress: "",
			setup: func() {
				getConnection = func(_ string) (*grpc.ClientConn, error) {
					return nil, errors.New("failed to connect to CSI driver")
				}
			},
			expectedErr: fmt.Errorf("failed to connect to CSI driver"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save the original osExit function
			originalOsExit := osExit
			defer func() { osExit = originalOsExit }()

			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}

			if tt.setup != nil {
				tt.setup()
			}

			conn := getCSIConn(tt.csiAddress)

			if tt.name == "success" {
				assert.NotNil(t, conn)
			} else if tt.name == "failure" {
				assert.Equal(t, 1, exitCode, "Expected exit code 1, got %d", exitCode)
			}

			getConnection = defaultGetConnection
		})
	}
}

func TestProbeCSIDriver(t *testing.T) {
	tests := []struct {
		name               string
		context            context.Context
		csiConn            *grpc.ClientConn
		expectedErr        error
		expectedDriverName string
	}{
		{
			name:               "success",
			context:            context.Background(),
			csiConn:            &grpc.ClientConn{},
			expectedErr:        nil,
			expectedDriverName: "driver-name",
		},
		{
			name:               "failure",
			context:            context.Background(),
			csiConn:            &grpc.ClientConn{},
			expectedErr:        fmt.Errorf("error waiting for the CSI driver to be ready"),
			expectedDriverName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrationCapabilities := getSampleMigrationCapabilities()

			// Save the original osExit function
			originalOsExit := osExit
			defer func() { osExit = originalOsExit }()

			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}

			identityClient := &MockIdentityClient{}
			identityClient.On("ProbeForever", tt.context).Return(tt.expectedDriverName, tt.expectedErr)
			if tt.name == "success" {
				identityClient.On("GetMigrationCapabilities", tt.context).Return(migrationCapabilities, tt.expectedErr)
			} else {
				identityClient.On("GetMigrationCapabilities", tt.context).Return(csiidentity.MigrationCapabilitySet{}, tt.expectedErr)
			}

			identityClient.On("GetReplicationCapabilities", tt.context).Return(mock.Anything, mock.Anything)
			identityClient.On("ProbeController", tt.context).Return(tt.expectedDriverName, true, tt.expectedErr)

			driverName := probeCSIDriver(tt.context, tt.csiConn, identityClient)
			if tt.expectedErr == nil {
				assert.Equal(t, tt.expectedDriverName, driverName)
			} else {
				// Assert the exit code
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, got %d", exitCode)
				}
			}
		})
	}
}

func TestProbeCSIDriverWrongCapability(t *testing.T) {
	tests := []struct {
		name               string
		context            context.Context
		csiConn            *grpc.ClientConn
		expectedErr        error
		expectedDriverName string
	}{
		{
			name:               "invalid capability",
			context:            context.Background(),
			csiConn:            &grpc.ClientConn{},
			expectedErr:        fmt.Errorf("error waiting for the CSI driver to be ready"),
			expectedDriverName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrationCapabilities := getSampleInavlidMigrationCapabilities()

			// Save the original osExit function
			originalOsExit := osExit
			defer func() { osExit = originalOsExit }()

			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}

			identityClient := &MockIdentityClient{}
			identityClient.On("ProbeForever", tt.context).Return(tt.expectedDriverName, tt.expectedErr)
			if tt.name == "invalid capability" {
				identityClient.On("GetMigrationCapabilities", tt.context).Return(migrationCapabilities, tt.expectedErr)
			} else {
				identityClient.On("GetMigrationCapabilities", tt.context).Return(csiidentity.MigrationCapabilitySet{}, tt.expectedErr)
			}

			identityClient.On("GetReplicationCapabilities", tt.context).Return(mock.Anything, mock.Anything)
			identityClient.On("ProbeController", tt.context).Return(tt.expectedDriverName, true, tt.expectedErr)

			driverName := probeCSIDriver(tt.context, tt.csiConn, identityClient)
			if tt.expectedErr == nil {
				assert.Equal(t, tt.expectedDriverName, driverName)
			} else {
				// Assert the exit code
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, got %d", exitCode)
				}
			}
		})
	}
}

func createFakeConnection() *grpc.ClientConn {
	conn, _ := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	return conn
}

func TestCreateMetricsServer(t *testing.T) {
	tests := []struct {
		name                       string
		driverName                 string
		metricsAddr                string
		enableLeaderElection       bool
		retryIntervalStart         time.Duration
		retryIntervalMax           time.Duration
		maxRetryDurationForActions time.Duration
		workerThreads              int
		expectedErr                error
	}{
		{
			name:                       "success",
			driverName:                 "driver-name",
			metricsAddr:                ":8001",
			enableLeaderElection:       false,
			retryIntervalStart:         1 * time.Second,
			retryIntervalMax:           5 * time.Minute,
			maxRetryDurationForActions: time.Hour,
			workerThreads:              2,
			expectedErr:                nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultGetCtrlNewManager := getCtrlNewManager
			defaultGetManagerStart := getManagerStart
			defaultGetWorkqueueReconcileRequest := getWorkqueueReconcileRequest
			defaultGetNodeRescanReconciler := getNodeRescanReconcilerManager

			getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
				return nil, errors.New("Unable to start manager")
			}

			getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
				return nil
			}

			getManagerStart = func(_ manager.Manager) error {
				return nil
			}

			getNodeRescanReconcilerManager = func(_ *controller.NodeRescanReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
				return nil
			}
			// Save the original osExit function
			originalOsExit := osExit
			defer func() { osExit = originalOsExit }()

			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}

			// Checked for Panic here as the panic is due to chain function calls and actual function is covered as expected
			// And actual code for the chained functions are covered on specific test cases related to that function
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("The code did not panic")
				}
			}()
			// Call the function under test
			createMetricsServer(context.Background(), tt.driverName, tt.metricsAddr, tt.enableLeaderElection, tt.retryIntervalStart, tt.retryIntervalMax, tt.maxRetryDurationForActions, tt.workerThreads)

			// Assert the exit code
			assert.NotEqual(t, 1, exitCode, "Expected exit code 1, got %d", exitCode)
			if exitCode != 1 {
				t.Errorf("Expected exit code 1, got %d", exitCode)
			}

			// Restore the original function after the test
			getCtrlNewManager = defaultGetCtrlNewManager
			getManagerStart = defaultGetManagerStart
			getWorkqueueReconcileRequest = defaultGetWorkqueueReconcileRequest
			getNodeRescanReconcilerManager = defaultGetNodeRescanReconciler
		})
	}
}

func TestCreateMetricsServerWithNodeRescannerError(t *testing.T) {
	tests := []struct {
		name                       string
		driverName                 string
		metricsAddr                string
		enableLeaderElection       bool
		retryIntervalStart         time.Duration
		retryIntervalMax           time.Duration
		maxRetryDurationForActions time.Duration
		workerThreads              int
		expectedErr                error
	}{
		{
			name:                       "success",
			driverName:                 "driver-name",
			metricsAddr:                ":8001",
			enableLeaderElection:       false,
			retryIntervalStart:         1 * time.Second,
			retryIntervalMax:           5 * time.Minute,
			maxRetryDurationForActions: time.Hour,
			workerThreads:              2,
			expectedErr:                nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultGetCtrlNewManager := getCtrlNewManager
			defaultGetManagerStart := getManagerStart
			defaultGetWorkqueueReconcileRequest := getWorkqueueReconcileRequest
			defaultGetNodeRescanReconciler := getNodeRescanReconcilerManager

			getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
				return &MockManager{}, nil
			}

			getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
				return nil
			}

			getManagerStart = func(_ manager.Manager) error {
				return nil
			}

			getNodeRescanReconcilerManager = func(_ *controller.NodeRescanReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
				return errors.New("Unable to create controller")
			}
			// Save the original osExit function
			originalOsExit := osExit
			defer func() { osExit = originalOsExit }()

			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}

			createMetricsServer(context.Background(), tt.driverName, tt.metricsAddr, tt.enableLeaderElection, tt.retryIntervalStart, tt.retryIntervalMax, tt.maxRetryDurationForActions, tt.workerThreads)

			// Assert the exit code
			assert.Equal(t, 1, exitCode, "Expected exit code 1, got %d", exitCode)

			// Restore the original function after the test
			getCtrlNewManager = defaultGetCtrlNewManager
			getManagerStart = defaultGetManagerStart
			getWorkqueueReconcileRequest = defaultGetWorkqueueReconcileRequest
			getNodeRescanReconcilerManager = defaultGetNodeRescanReconciler
		})
	}
}

func TestCreateMetricsServerWithStratingManagerError(t *testing.T) {
	tests := []struct {
		name                       string
		driverName                 string
		metricsAddr                string
		enableLeaderElection       bool
		retryIntervalStart         time.Duration
		retryIntervalMax           time.Duration
		maxRetryDurationForActions time.Duration
		workerThreads              int
		expectedErr                error
	}{
		{
			name:                       "success",
			driverName:                 "driver-name",
			metricsAddr:                ":8001",
			enableLeaderElection:       false,
			retryIntervalStart:         1 * time.Second,
			retryIntervalMax:           5 * time.Minute,
			maxRetryDurationForActions: time.Hour,
			workerThreads:              2,
			expectedErr:                nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultGetCtrlNewManager := getCtrlNewManager
			defaultGetManagerStart := getManagerStart
			defaultGetWorkqueueReconcileRequest := getWorkqueueReconcileRequest
			defaultGetNodeRescanReconciler := getNodeRescanReconcilerManager

			getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
				return &MockManager{}, nil
			}

			getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
				return nil
			}

			getManagerStart = func(_ manager.Manager) error {
				return errors.New("problem running maanager")
			}

			getNodeRescanReconcilerManager = func(_ *controller.NodeRescanReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
				return nil
			}
			// Save the original osExit function
			originalOsExit := osExit
			defer func() { osExit = originalOsExit }()

			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}

			createMetricsServer(context.Background(), tt.driverName, tt.metricsAddr, tt.enableLeaderElection, tt.retryIntervalStart, tt.retryIntervalMax, tt.maxRetryDurationForActions, tt.workerThreads)

			// Assert the exit code
			assert.Equal(t, 1, exitCode, "Expected exit code 1, got %d", exitCode)

			// Restore the original function after the test
			getCtrlNewManager = defaultGetCtrlNewManager
			getManagerStart = defaultGetManagerStart
			getWorkqueueReconcileRequest = defaultGetWorkqueueReconcileRequest
			getNodeRescanReconcilerManager = defaultGetNodeRescanReconciler
		})
	}
}

func TestProcessConfigMapChanges(t *testing.T) {
	defaultGetUpdateConfigMapFunc := getUpdateConfigMapFunc
	defer func() {
		getUpdateConfigMapFunc = defaultGetUpdateConfigMapFunc
	}()

	mgr := &NodeRescanner{
		Opts:     config.ControllerManagerOpts{},
		Manager:  &MockManager{},
		NodeName: "test-node",
		config:   &config.Config{},
	}

	// Test case 1: Error in getUpdateConfigMapFunc
	getUpdateConfigMapFunc = func(_ *NodeRescanner, _ context.Context) error {
		return fmt.Errorf("config update error")
	}
	t.Run("Error in getUpdateConfigMapFunc", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})

	// Test case 2: Success with valid log level
	getUpdateConfigMapFunc = func(_ *NodeRescanner, _ context.Context) error {
		return nil
	}
	mgr.config.LogLevel = "info"
	t.Run("Success with valid log level", func(t *testing.T) {
		mgr.processConfigMapChanges()
		assert.Equal(t, csmlog.InfoLevel, csmlog.GetLevel())
	})

	// Test case 3: Invalid log level
	getUpdateConfigMapFunc = func(_ *NodeRescanner, _ context.Context) error {
		return nil
	}
	mgr.config.LogLevel = "invalid-level"
	t.Run("Invalid log level", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})

	// Test case 4: Valid log format
	getUpdateConfigMapFunc = func(_ *NodeRescanner, _ context.Context) error {
		return nil
	}
	mgr.config.LogLevel = "info"
	mgr.config.LogFormat = "TEXT"
	t.Run("Valid log format", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})

	// Test case 5: Invalid log format
	getUpdateConfigMapFunc = func(_ *NodeRescanner, _ context.Context) error {
		return nil
	}
	mgr.config.LogFormat = "invalid"
	t.Run("Invalid log format", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})
}

func TestSetupConfigMapWatcher(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Test with valid config",
		},
		{
			name: "Test with changed config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			mgr := &NodeRescanner{
				Opts:     config.ControllerManagerOpts{},
				Manager:  &MockManager{},
				NodeName: "test-node",
				config:   &config.Config{LogLevel: "info"},
			}

			if tt.name == "Test with valid config" {
				mgr.setupConfigMapWatcher()
			} else if tt.name == "Test with changed config" {
				mgr.setupConfigMapWatcher()
				csmlog.SetLevel(csmlog.InfoLevel)
				mgr.setupConfigMapWatcher()
			}
			// Test passes if no panic occurs
		})
	}
}

func TestProbeAndCreateMetricsServer(t *testing.T) {
	defaultGetManagerStart := getManagerStart
	defaultGetCtrlNewManager := getCtrlNewManager
	defaultGetWorkqueueReconcileRequest := getWorkqueueReconcileRequest
	defaultGetNodeRescanReconciler := getNodeRescanReconcilerManager

	getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
		return &MockManager{}, nil
	}

	getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
		return nil
	}

	getManagerStart = func(_ manager.Manager) error {
		return nil
	}

	getNodeRescanReconcilerManager = func(_ *controller.NodeRescanReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
		return nil
	}

	// Create a mock CSI connection
	csiConn := createFakeConnection()

	// Get migration capabilities
	migrationCapabilities := getSampleMigrationCapabilities()

	// Override osExit to capture the exit code
	var exitCode int
	osExit = func(code int) {
		exitCode = code
	}

	// Create a mock flag map
	flagMap := map[string]string{
		"csi-address":                    "localhost:4772",
		"metrics-addr":                   ":8001",
		"leader-election":                "false",
		"retry-interval-start":           "1s",
		"retry-interval-max":             "5m",
		"max-retry-duration-for-actions": "1h",
		"worker-threads":                 "2",
	}

	// Create a mock context
	ctx := context.Background()

	identityClient := &MockIdentityClient{}
	identityClient.On("ProbeForever", ctx).Return("driver-name", nil)
	identityClient.On("GetMigrationCapabilities", ctx).Return(migrationCapabilities, nil)
	identityClient.On("GetReplicationCapabilities", ctx).Return(mock.Anything, mock.Anything)
	identityClient.On("ProbeController", ctx).Return("driver-name", true, nil)

	// Call the function with the mock dependencies
	probeAndCreateMetricsServer(ctx, csiConn, identityClient, flagMap)

	// Assert the exit code
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	// Restore the original function after the test
	getManagerStart = defaultGetManagerStart
	getCtrlNewManager = defaultGetCtrlNewManager
	getWorkqueueReconcileRequest = defaultGetWorkqueueReconcileRequest
	getNodeRescanReconcilerManager = defaultGetNodeRescanReconciler
}

func TestSetupFlags(t *testing.T) {
	// Set Manifest version similar to how the image would be built.
	ManifestSemver = "1.0.0"

	// Call the setupFlags function
	flags, ctx := setupFlags()

	// Assert the expected values
	expected := map[string]string{
		"metrics-addr":         ":8001",
		"leader-election":      "false",
		"csi-address":          "/var/run/csi.sock",
		"prefix":               constants.DefaultMigrationDomain,
		"repl-prefix":          constants.DefaultDomain,
		"worker-threads":       "2",
		"retry-interval-start": "1s",
		"retry-interval-max":   "5m0s",
		"timeout":              "5m0s",
		"probe-frequency":      "5s",
	}

	for key, expectedValue := range expected {
		if flags[key] != expectedValue {
			t.Errorf("Expected flag value for %s to be %s, but got %s", key, expectedValue, flags[key])
		}
	}

	assert.NotNil(t, flags)
	assert.NotNil(t, ctx)
}

func TestStringToTimeDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{
			name:     "valid duration",
			input:    "1h30m",
			expected: time.Hour + time.Minute*30,
		},
		{
			name:     "invalid duration",
			input:    "invalid",
			expected: time.Duration(0),
		},
		{
			name:     "empty duration",
			input:    "",
			expected: time.Duration(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := stringToTimeDuration(tt.input)
			if actual != tt.expected {
				t.Errorf("Expected %v, but got %v", tt.expected, actual)
			}
		})
	}
}

func TestStringToBoolean(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "true",
			input:    "true",
			expected: true,
		},
		{
			name:     "false",
			input:    "false",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "invalid string",
			input:    "invalid",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := stringToBoolean(tt.input)
			if actual != tt.expected {
				t.Errorf("Expected %v, but got %v", tt.expected, actual)
			}
		})
	}
}

func TestStringToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "valid integer",
			input:    "42",
			expected: 42,
		},
		{
			name:     "invalid integer",
			input:    "invalid",
			expected: 0,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := stringToInt(tt.input)
			if actual != tt.expected {
				t.Errorf("Expected %v, but got %v", tt.expected, actual)
			}
		})
	}
}

func getSampleMigrationCapabilities() csiidentity.MigrationCapabilitySet {
	capabilitySet := csiidentity.MigrationCapabilitySet{}

	capabilitySet[migration.MigrateTypes_NON_REPL_TO_REPL] = true
	capabilitySet[migration.MigrateTypes_REPL_TO_NON_REPL] = true
	capabilitySet[migration.MigrateTypes_VERSION_UPGRADE] = true
	return capabilitySet
}

func getSampleInavlidMigrationCapabilities() csiidentity.MigrationCapabilitySet {
	capabilitySet := csiidentity.MigrationCapabilitySet{}

	capabilitySet[migration.MigrateTypes_UNKNOWN_MIGRATE] = true
	return capabilitySet
}

// MockIdentityClient is a mock implementation of the csiidentity.IdentityClient interface
type MockIdentityClient struct {
	mock.Mock
}

func (m *MockIdentityClient) ProbeForever(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockIdentityClient) GetMigrationCapabilities(ctx context.Context) (csiidentity.MigrationCapabilitySet, error) {
	args := m.Called(ctx)
	return args.Get(0).(csiidentity.MigrationCapabilitySet), args.Error(1)
}

func (m *MockIdentityClient) GetReplicationCapabilities(ctx context.Context) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
	args := m.Called(ctx)
	return args.Get(0).(csiidentity.ReplicationCapabilitySet), args.Get(1).([]*replication.SupportedActions), args.Error(2)
}

func (m *MockIdentityClient) ProbeController(_ context.Context) (string, bool, error) {
	// You can add mock behavior here
	return "driver-name", true, nil
}

// MockManager is a mock implementation of the ctrl.Manager interface
type MockManager struct{ manager.Manager }

func (m *MockManager) Start(context.Context) error {
	return nil
}

func (m *MockManager) GetOptions() manager.Options {
	return manager.Options{}
}

func (m *MockManager) Add(manager.Runnable) error {
	// Implement the Add method logic
	return nil
}

func (m *MockManager) AddHealthzCheck(_ string, _ healthz.Checker) error {
	// Implement the AddHealthzCheck method logic
	return nil
}

func (m *MockManager) AddMetricsExtraHandler(_ string, _ http.Handler) error {
	// Implement the AddMetricsExtraHandler method logic
	return nil
}

func (m *MockManager) AddMetricsServerExtraHandler(_ string, _ http.Handler) error {
	// Implement the AddMetricsExtraHandler method logic
	return nil
}

func (m *MockManager) AddReadyzCheck(_ string, _ healthz.Checker) error {
	// Implement the AddReadyzCheck method logic
	return nil
}

func (m *MockManager) Elected() <-chan struct{} {
	// Implement the Elected method logic
	return nil
}

func (m *MockManager) GetAPIReader() client.Reader {
	// Implement the GetAPIReader method logic
	return nil
}

func (m *MockManager) GetCache() cache.Cache {
	// Implement the GetCache method logic
	return nil
}

func (m *MockManager) GetClient() client.Client {
	// Implement the GetClient method logic
	return nil
}

func (m *MockManager) GetConfig() *rest.Config {
	// Implement the GetConfig method logic
	return nil
}

func (m *MockManager) GetControllerOptions() ctrlruntimeconfig.Controller {
	// Implement the GetControllerOptions method logic
	return ctrlruntimeconfig.Controller{}
}

func (m *MockManager) GetFieldIndexer() client.FieldIndexer {
	// Implement the GetFieldIndexer method logic
	return nil
}

func (m *MockManager) GetEventRecorderFor(_ string) record.EventRecorder {
	// Implement the GetEventRecorderFor method logic
	return &MockEventRecorder{}
}

func (m *MockManager) GetHTTPClient() *http.Client {
	// Implement the GetHTTPClient method logic
	return nil
}

func (m *MockManager) GetRESTMapper() meta.RESTMapper {
	// Implement the GetRESTMapper method logic
	return nil
}

func (m *MockManager) GetScheme() *runtime.Scheme {
	// Implement the GetScheme method logic
	return nil
}

func (m *MockManager) GetWebhookServer() webhook.Server {
	// Implement the GetWebhookServer method logic
	return nil
}

func (m *MockManager) GetConverterRegistry() conversion.Registry {
	// Implement the GetConverterRegistry method logic
	return nil
}

func (m *MockManager) GetEventRecorder(_ string) recorder.EventRecorder {
	// Implement the GetEventRecorder method logic
	return nil
}

// MockEventRecorder is a mock implementation of the record.EventRecorder interface
type MockEventRecorder struct{}

func (m *MockEventRecorder) Event(_ runtime.Object, _, _, _ string) {}
func (m *MockEventRecorder) Eventf(_ runtime.Object, _, _, _ string, _ ...interface{}) {
}

func (m *MockEventRecorder) AnnotatedEventf(_ runtime.Object, _ map[string]string, _, _, _ string, _ ...interface{}) {
}
