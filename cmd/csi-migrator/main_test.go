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

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	controller "github.com/dell/csm-replication/controllers/csi-migrator"
	"github.com/dell/csm-replication/pkg/common/constants"
	"github.com/dell/csm-replication/pkg/config"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	"github.com/dell/dell-csi-extensions/migration"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcnf "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type mockManager struct {
	logger              logr.Logger
	client              client.Client
	scheme              *runtime.Scheme
	eventRec            record.EventRecorder
	config              *config.Config
	singleflight        singleflight.Group
	controllerName      string
	controllerGroupName string
	reconciler          *controller.PersistentVolumeReconciler
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

func TestMain(t *testing.T) {
	defaultGetConnectToCsiFunc := getConnectToCsiFunc
	defaultGetProbeForeverFunc := getProbeForeverFunc
	defaultGetMigrationCapabilitiesFunc := getMigrationCapabilitiesFunc
	defaultGetcreateMigratorManagerFunc := getcreateMigratorManagerFunc
	defaultGetParseLevelFunc := getParseLevelFunc
	defaultGetManagerStart := getManagerStart
	defaultGetCtrlNewManager := getCtrlNewManager
	defaultGetWorkqueueReconcileRequest := getWorkqueueReconcileRequest
	defaultGetPersistentVolumeReconcilerSetupWithManager := getPersistentVolumeReconcilerSetupWithManager
	defaultGetMigrationGroupReconcilerSetupWithManager := getMigrationGroupReconcilerSetupWithManager
	defaultOSExit := osExit
	defaultSetupFlags := setupFlags

	osExitCode := 0

	after := func() {
		// Restore the original function after the test
		getConnectToCsiFunc = defaultGetConnectToCsiFunc
		getProbeForeverFunc = defaultGetProbeForeverFunc
		getMigrationCapabilitiesFunc = defaultGetMigrationCapabilitiesFunc
		getcreateMigratorManagerFunc = defaultGetcreateMigratorManagerFunc
		getParseLevelFunc = defaultGetParseLevelFunc
		getManagerStart = defaultGetManagerStart
		getCtrlNewManager = defaultGetCtrlNewManager
		getWorkqueueReconcileRequest = defaultGetWorkqueueReconcileRequest
		getPersistentVolumeReconcilerSetupWithManager = defaultGetPersistentVolumeReconcilerSetupWithManager
		getMigrationGroupReconcilerSetupWithManager = defaultGetMigrationGroupReconcilerSetupWithManager
		osExit = defaultOSExit
		setupFlags = defaultSetupFlags
	}

	tests := []struct {
		name               string
		setup              func()
		expectedOsExitCode int
	}{
		{
			name: "Successful run of main function",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{
						migration.MigrateTypes(migration.MigrateTypes_VERSION_UPGRADE):  true,
						migration.MigrateTypes(migration.MigrateTypes_REPL_TO_NON_REPL): true,
					}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(level string) (logrus.Level, error) {
					return logrus.Level(0), fmt.Errorf("unable to parse log level: %s", level)
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}
			},
			expectedOsExitCode: 0,
		},
		{
			name: "failed to connect to CSI driver",
			setup: func() {
				getConnectToCsiFunc = func(_ string, _ logr.Logger) (*grpc.ClientConn, error) {
					return &grpc.ClientConn{}, errors.New("error connecting to CSI driver")
				}

				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", errors.New("error waiting for the CSI driver to be ready")
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:          ":8001",
						enableLeaderElection: false,
						csiAddress:           "/var/run/csi.sock",
						workerThreads:        2,
						retryIntervalStart:   time.Second,
						retryIntervalMax:     5 * time.Minute,
						operationTimeout:     300 * time.Second,
						domain:               constants.DefaultMigrationDomain,
						replicationDomain:    constants.DefaultDomain,
						probeFrequency:       5 * time.Second,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "error waiting for the CSI driver to be ready",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", errors.New("error waiting for the CSI driver to be ready")
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:          ":8001",
						enableLeaderElection: false,
						csiAddress:           "/var/run/csi.sock",
						workerThreads:        2,
						retryIntervalStart:   time.Second,
						retryIntervalMax:     5 * time.Minute,
						operationTimeout:     300 * time.Second,
						domain:               constants.DefaultMigrationDomain,
						replicationDomain:    constants.DefaultDomain,
						probeFrequency:       5 * time.Second,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "error fetching migration capabilities",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{}

					return capabilitySet, errors.New("error fetching migration capabilities")
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:          ":8001",
						enableLeaderElection: false,
						csiAddress:           "/var/run/csi.sock",
						workerThreads:        2,
						retryIntervalStart:   time.Second,
						retryIntervalMax:     5 * time.Minute,
						operationTimeout:     300 * time.Second,
						domain:               constants.DefaultMigrationDomain,
						replicationDomain:    constants.DefaultDomain,
						probeFrequency:       5 * time.Second,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "driver doesn't support migration",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:          ":8001",
						enableLeaderElection: false,
						csiAddress:           "/var/run/csi.sock",
						workerThreads:        2,
						retryIntervalStart:   time.Second,
						retryIntervalMax:     5 * time.Minute,
						operationTimeout:     300 * time.Second,
						domain:               constants.DefaultMigrationDomain,
						replicationDomain:    constants.DefaultDomain,
						probeFrequency:       5 * time.Second,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "unknown capability advertised",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{
						migration.MigrateTypes_UNKNOWN_MIGRATE: true,
					}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:          ":8001",
						enableLeaderElection: false,
						csiAddress:           "/var/run/csi.sock",
						workerThreads:        2,
						retryIntervalStart:   time.Second,
						retryIntervalMax:     5 * time.Minute,
						operationTimeout:     300 * time.Second,
						domain:               constants.DefaultMigrationDomain,
						replicationDomain:    constants.DefaultDomain,
						probeFrequency:       5 * time.Second,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "unable to start manager",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{
						migration.MigrateTypes(migration.MigrateTypes_VERSION_UPGRADE):  true,
						migration.MigrateTypes(migration.MigrateTypes_REPL_TO_NON_REPL): true,
					}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, errors.New("unable to start manager")
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:          ":8001",
						enableLeaderElection: false,
						csiAddress:           "/var/run/csi.sock",
						workerThreads:        2,
						retryIntervalStart:   time.Second,
						retryIntervalMax:     5 * time.Minute,
						operationTimeout:     300 * time.Second,
						domain:               constants.DefaultMigrationDomain,
						replicationDomain:    constants.DefaultDomain,
						probeFrequency:       5 * time.Second,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "failed to configure the migrator manager",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{
						migration.MigrateTypes(migration.MigrateTypes_VERSION_UPGRADE):  true,
						migration.MigrateTypes(migration.MigrateTypes_REPL_TO_NON_REPL): true,
					}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, errors.New("failed to configure the migrator manager")
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.Level(0), fmt.Errorf("unable to parse log level")
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:          ":8001",
						enableLeaderElection: false,
						csiAddress:           "/var/run/csi.sock",
						workerThreads:        2,
						retryIntervalStart:   time.Second,
						retryIntervalMax:     5 * time.Minute,
						operationTimeout:     300 * time.Second,
						domain:               constants.DefaultMigrationDomain,
						replicationDomain:    constants.DefaultDomain,
						probeFrequency:       5 * time.Second,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "PersistentVolumeReconciler - unable to create controller",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{
						migration.MigrateTypes(migration.MigrateTypes_VERSION_UPGRADE):  true,
						migration.MigrateTypes(migration.MigrateTypes_REPL_TO_NON_REPL): true,
					}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("PersistentVolumeReconciler - unable to create controller")
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:          ":8001",
						enableLeaderElection: false,
						csiAddress:           "/var/run/csi.sock",
						workerThreads:        2,
						retryIntervalStart:   time.Second,
						retryIntervalMax:     5 * time.Minute,
						operationTimeout:     300 * time.Second,
						domain:               constants.DefaultMigrationDomain,
						replicationDomain:    constants.DefaultDomain,
						probeFrequency:       5 * time.Second,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "MigrationGroupReconciler - unable to create controller",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{
						migration.MigrateTypes(migration.MigrateTypes_VERSION_UPGRADE):  true,
						migration.MigrateTypes(migration.MigrateTypes_REPL_TO_NON_REPL): true,
					}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.Level(0), nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("MigrationGroupReconciler - unable to create controller")
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "Problem running manager",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
					capabilitySet := csiidentity.MigrationCapabilitySet{
						migration.MigrateTypes(migration.MigrateTypes_VERSION_UPGRADE):  true,
						migration.MigrateTypes(migration.MigrateTypes_REPL_TO_NON_REPL): true,
					}

					return capabilitySet, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateMigratorManagerFunc = func(_ context.Context, _ manager.Manager) (*MigratorManager, error) {
					return &MigratorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.Level(0), nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getMigrationGroupReconcilerSetupWithManager = func(_ *controller.MigrationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return errors.New("problem running manager")
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{}
				}
			},
			expectedOsExitCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			defer after()
			tt.setup()
			main()
		})
		if osExitCode != tt.expectedOsExitCode {
			t.Errorf("Expected osExitCode: %v, but got osExitCode: %v", tt.expectedOsExitCode, osExitCode)
		}
		osExitCode = 0
	}
}
