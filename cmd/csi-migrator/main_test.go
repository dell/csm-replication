/*
 Copyright © 2024-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"fmt"
	"net/http"
	"testing"

	"github.com/dell/csm-replication/pkg/config"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcnf "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// // Mock types for testing
// type MockCSIConnection struct{}
// type MockIdentityClient struct{}
// type MockMigrationClient struct{}
// type MockMigratorManager struct{}

// func TestMainFunction(t *testing.T) {
// 	// Set up flags (using default test values)
// 	metricsAddr := ":8001"
// 	enableLeaderElection := false
// 	workerThreads := 2
// 	retryIntervalStart := time.Second
// 	retryIntervalMax := 5 * time.Minute
// 	operationTimeout := 300 * time.Second
// 	probeFrequency := 5 * time.Second
// 	domain := "default-domain"
// 	replicationDomain := "default-repl-domain"
// 	maxRetryDurationForActions := 10 * time.Minute

// 	// Mock context
// 	ctx := context.Background()

// 	// Mock CSI connection
// 	csiConn := &MockCSIConnection{}
// 	identityClient := &MockIdentityClient{}
// 	migrationClient := &MockMigrationClient{}

// 	// Mock manager setup
// 	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
// 		Scheme:                     nil, // Provide a mock or actual scheme here if needed
// 		Metrics:                    server.Options{BindAddress: metricsAddr},
// 		WebhookServer:              webhook.NewServer(webhook.Options{Port: 8443}),
// 		LeaderElection:             enableLeaderElection,
// 		LeaderElectionResourceLock: "leases",
// 		LeaderElectionID:           "test-leader-election-id",
// 	})
// 	require.NoError(t, err)

// 	// Mock Migrator Manager
// 	migratorMgr := &MockMigratorManager{}

// 	// Initialize components
// 	err = initializeComponents(ctx, mgr, csiConn, identityClient, migrationClient, migratorMgr, "info", domain, replicationDomain, workerThreads, retryIntervalStart, retryIntervalMax, operationTimeout, probeFrequency, maxRetryDurationForActions)
// 	require.NoError(t, err)

// 	// Verify initialization
// 	assert.NotNil(t, mgr)
// 	assert.NotNil(t, csiConn)
// 	assert.NotNil(t, identityClient)
// 	assert.NotNil(t, migrationClient)
// 	assert.NotNil(t, migratorMgr)
// }

// func initializeComponents(ctx context.Context, mgr ctrl.Manager, csiConn *MockCSIConnection, identityClient *MockIdentityClient, migrationClient *MockMigrationClient, migratorMgr *MockMigratorManager, logLevel, domain, replicationDomain string, workerThreads int, retryIntervalStart, retryIntervalMax, operationTimeout, probeFrequency, maxRetryDurationForActions time.Duration) error {
// 	// Simulate initialization logic
// 	// If any error occurs here, we can check it against specific mock expectations
// 	return nil
// }

// // Mock function implementations for the mock types
// func (m *MockCSIConnection) Connect(address string) error {
// 	// Simulate a successful connection
// 	return nil
// }

// func (m *MockIdentityClient) ProbeForever(ctx context.Context) (string, error) {
// 	// Simulate a successful probe
// 	return "mock-driver", nil
// }

// func (m *MockIdentityClient) GetMigrationCapabilities(ctx context.Context) (map[string]bool, error) {
// 	// Simulate returning migration capabilities
// 	return map[string]bool{"migrator": true}, nil
// }

// func (m *MockMigrationClient) Migrate(ctx context.Context) error {
// 	// Simulate a migration operation
// 	return nil
// }

// func (m *MockMigratorManager) SetupConfigMapWatcher(logger logrus.Logger) {
// 	// Simulate setting up a config map watcher
// }

// func (m *MockMigratorManager) Config() *Config {
// 	// Simulate fetching a config
// 	return &Config{LogLevel: "info"}
// }

type mockManager struct {
	logger logr.Logger
}

func (m *mockManager) GetLogger() logr.Logger {
	return m.logger
}

func (m *mockManager) Add(_ manager.Runnable) error {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) AddHealthzCheck(_ string, _ healthz.Checker) error {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) AddMetricsServerExtraHandler(_ string, _ http.Handler) error {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) AddReadyzCheck(_ string, _ healthz.Checker) error {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) Elected() <-chan struct{} {
	// Implement the method as needed for your mock
	return make(chan struct{})
}

func (m *mockManager) GetControllerOptions() ctrlcnf.Controller {
	// Implement the method as needed for your mock
	return ctrlcnf.Controller{}
}

type mockServer struct {
	mux *http.ServeMux
}

func (m *mockServer) NeedLeaderElection() bool {
	return false
}

func (m *mockServer) Register(path string, hook http.Handler) {
	if m.mux == nil {
		m.mux = http.NewServeMux()
	}
	m.mux.Handle(path, hook)
}

func (m *mockServer) Start(_ context.Context) error {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockServer) StartedChecker() healthz.Checker {
	return healthz.Ping
}

func (m *mockServer) WebhookMux() *http.ServeMux {
	return m.mux
}

func (m *mockManager) GetWebhookServer() webhook.Server {
	// Implement the method as needed for your mock
	return &mockServer{}
}

func (m *mockManager) Start(_ context.Context) error {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetAPIReader() client.Reader {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetCache() cache.Cache {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetConfig() *rest.Config {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetClient() client.Client {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetEventRecorderFor(_ string) record.EventRecorder {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetFieldIndexer() client.FieldIndexer {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetHTTPClient() *http.Client {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetRESTMapper() meta.RESTMapper {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetScheme() *runtime.Scheme {
	// Implement the method as needed for your mock
	return nil
}

func TestProcessConfigMapChanges(t *testing.T) {
	defaultGetUpdateConfigMapFunc := getUpdateConfigMapFunc
	defer func() {
		getUpdateConfigMapFunc = defaultGetUpdateConfigMapFunc
	}()

	// Test case 1: Success - no error in getUpdateConfigMapFunc
	getUpdateConfigMapFunc = func(_ *MigratorManager, _ context.Context) error {
		return nil
	}

	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	loggerConfig := logrus.New()
	migrator := &MigratorManager{
		Opts:    config.ControllerManagerOpts{},
		Manager: mockMgr,
		config:  &config.Config{},
	}

	t.Run("Success Test Case", func(_ *testing.T) {
		migrator.processConfigMapChanges(loggerConfig)
	})

	// Test case 2: Error in getUpdateConfigMapFunc
	getUpdateConfigMapFunc = func(_ *MigratorManager, _ context.Context) error {
		return fmt.Errorf("config update error")
	}

	t.Run("Error in getUpdateConfigMapFunc", func(_ *testing.T) {
		migrator.processConfigMapChanges(loggerConfig)
	})

	// Test case 3: Error in ParseLevel (invalid log level)
	getUpdateConfigMapFunc = func(_ *MigratorManager, _ context.Context) error {
		return nil
	}

	migrator.config.LogLevel = "invalid-log-level" // Set an invalid log level

	t.Run("Error in ParseLevel", func(_ *testing.T) {
		migrator.processConfigMapChanges(loggerConfig)
	})

	// Test case 4: Valid Log Level set in config
	getUpdateConfigMapFunc = func(_ *MigratorManager, _ context.Context) error {
		return nil
	}

	migrator.config.LogLevel = "info" // Set a valid log level

	t.Run("Valid Log Level", func(t *testing.T) {
		migrator.processConfigMapChanges(loggerConfig)
		if loggerConfig.GetLevel() != logrus.InfoLevel {
			t.Errorf("Expected log level to be info, but got %v", loggerConfig.GetLevel())
		}
	})
}

func TestCreateMigratorManager(t *testing.T) {
	// Define a default function for getConfigFunc
	defaultGetConfigFunc := getConfigFunc
	defer func() {
		getConfigFunc = defaultGetConfigFunc
	}()

	// Test case 1: Successful creation of MigratorManager
	getConfigFunc = func(_ context.Context, _ config.ControllerManagerOpts, _ logr.Logger) (*config.Config, error) {
		// Mock the successful config return
		return &config.Config{LogLevel: "info"}, nil
	}

	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	t.Run("Success Test Case", func(t *testing.T) {
		migratorManager, err := createMigratorManager(context.Background(), mockMgr)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if migratorManager == nil {
			t.Fatalf("Expected MigratorManager to be non-nil")
		}
		// You can add more assertions here if needed
	})

	// Test case 2: Error in getConfigFunc
	getConfigFunc = func(_ context.Context, _ config.ControllerManagerOpts, _ logr.Logger) (*config.Config, error) {
		// Mock an error return from getConfigFunc
		return nil, fmt.Errorf("failed to get config")
	}

	t.Run("Error in getConfigFunc", func(t *testing.T) {
		migratorManager, err := createMigratorManager(context.Background(), mockMgr)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if migratorManager != nil {
			t.Fatalf("Expected MigratorManager to be nil, but got non-nil")
		}
		if err.Error() != "failed to get config" {
			t.Fatalf("Expected error 'failed to get config', got %v", err)
		}
	})

	// Test case 3: Nil Manager passed to createMigratorManager
	t.Run("Nil Manager", func(t *testing.T) {
		migratorManager, err := createMigratorManager(context.Background(), nil)
		if err == nil {
			t.Fatalf("Expected error when manager is nil, got nil")
		}
		if migratorManager != nil {
			t.Fatalf("Expected MigratorManager to be nil, but got non-nil")
		}
	})

	// Test case 4: Check if the correct mode is set in options
	t.Run("Correct Mode Set", func(t *testing.T) {
		getConfigFunc = func(_ context.Context, _ config.ControllerManagerOpts, _ logr.Logger) (*config.Config, error) {
			// Return config with any log level
			return &config.Config{LogLevel: "info"}, nil
		}

		mockMgr := &mockManager{
			logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
		}

		migratorManager, err := createMigratorManager(context.Background(), mockMgr)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if migratorManager.Opts.Mode != "sidecar" {
			t.Fatalf("Expected mode to be 'sidecar', got %s", migratorManager.Opts.Mode)
		}
	})
}

func TestSetupConfigMapWatcher(t *testing.T) {
	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	loggerConfig := logrus.New()
	migrator := &MigratorManager{
		Opts:    config.ControllerManagerOpts{},
		Manager: mockMgr,
		config:  &config.Config{},
	}
	migrator.setupConfigMapWatcher(loggerConfig)
}
