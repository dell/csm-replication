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

	"github.com/fsnotify/fsnotify"

	metricscommon "github.com/dell/csm-metrics-common/pkg/server"
	controller "github.com/dell/csm-replication/controllers/csi-replicator"
	"github.com/dell/csm-replication/pkg/common/constants"
	repcnf "github.com/dell/csm-replication/pkg/config"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	"github.com/dell/csmlog"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlcnf "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

type mockManager struct {
	manager.Manager
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

func (m *mockManager) GetConverterRegistry() conversion.Registry {
	// Implement the method as needed for your mock
	return nil
}

func (m *mockManager) GetEventRecorder(_ string) recorder.EventRecorder {
	// Implement the method as needed for your mock
	return nil
}

func TestCreateReplicatorManager(t *testing.T) {
	// Original function references
	originalGetControllerManagerOpts := getControllerManagerOpts
	originalGetConfig := getConfig

	// Reset function to reset mocks after tests
	resetMocks := func() {
		getControllerManagerOpts = originalGetControllerManagerOpts
		getConfig = originalGetConfig
	}

	// Mock manager
	mockMgr := &mockManager{}

	tests := []struct {
		name    string
		ctx     context.Context
		mgr     ctrl.Manager
		setup   func()
		want    *ReplicatorManager
		wantErr bool
	}{
		{
			name: "Successful creation of ReplicatorManager",
			ctx:  context.TODO(),
			mgr:  mockMgr,
			setup: func() {
				getControllerManagerOpts = func() repcnf.ControllerManagerOpts {
					return repcnf.ControllerManagerOpts{}
				}
				getConfig = func(_ context.Context, _ client.Client, _ repcnf.ControllerManagerOpts, _ record.EventRecorder) (*repcnf.Config, error) {
					return &repcnf.Config{}, nil
				}
			},
			want: &ReplicatorManager{
				Opts:    repcnf.ControllerManagerOpts{Mode: "sidecar"},
				Manager: mockMgr,
				config:  &repcnf.Config{},
			},
			wantErr: false,
		},
		{
			name: "Error in getting config",
			ctx:  context.TODO(),
			mgr:  mockMgr,
			setup: func() {
				getControllerManagerOpts = func() repcnf.ControllerManagerOpts {
					return repcnf.ControllerManagerOpts{}
				}
				getConfig = func(_ context.Context, _ client.Client, _ repcnf.ControllerManagerOpts, _ record.EventRecorder) (*repcnf.Config, error) {
					return nil, assert.AnError
				}
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer resetMocks() // Ensures any mocks or overrides are reset after each test

			// Setup test case specific mocks and overrides
			if tt.setup != nil {
				tt.setup()
			}

			// Call the function under test
			got, err := createReplicatorManager(tt.ctx, tt.mgr)

			// Check if the error status matches
			if (err != nil) != tt.wantErr {
				t.Errorf("createReplicatorManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Validate the response
			if !assert.Equal(t, tt.want, got) {
				t.Errorf("createReplicatorManager() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getClusterUID(t *testing.T) {
	defaultGetControllerClient := getControllerClient
	defer func() {
		getControllerClient = defaultGetControllerClient
	}()

	tests := []struct {
		name    string
		prepare func() (client.Client, error)
		wantErr bool
	}{
		{
			name: "Success",
			prepare: func() (client.Client, error) {
				return fake.NewClientBuilder().WithObjects(&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kube-system",
						UID:  "999", // fake UID
					},
				}).Build(), nil
			},
			wantErr: false,
		},
		{
			name: "NamespaceNotFound",
			prepare: func() (client.Client, error) {
				// no objects in fake client
				return fake.NewClientBuilder().Build(), nil
			},
			wantErr: true,
		},
		{
			name: "ClientError",
			prepare: func() (client.Client, error) {
				return nil, errors.New("client error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getControllerClient = func(_ *rest.Config, _ *runtime.Scheme) (client.Client, error) {
				return tt.prepare()
			}

			got, err := getClusterUID(context.TODO())
			if (err != nil) != tt.wantErr {
				t.Errorf("getClusterUID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got.Name != "kube-system" {
					t.Errorf("expected namespace kube-system, got %s", got.Name)
				}
				if string(got.UID) == "" {
					t.Errorf("expected non-empty UID")
				}
			}
		})
	}
}

func TestProcessConfigMapChanges(t *testing.T) {
	defaultGetUpdateConfigMapFunc := getUpdateConfigMapFunc
	defer func() {
		getUpdateConfigMapFunc = defaultGetUpdateConfigMapFunc
	}()

	// Test case 1: Success - no error in getUpdateConfigMapFunc
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return nil
	}

	mockMgr := &mockManager{}

	mgr := &ReplicatorManager{
		Opts:    repcnf.ControllerManagerOpts{},
		Manager: mockMgr,
		config:  &repcnf.Config{},
	}

	t.Run("Success Test Case", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})

	// Test case 2: Error in getUpdateConfigMapFunc
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return fmt.Errorf("config update error")
	}

	t.Run("Error in getUpdateConfigMapFunc", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})

	// Test case 3: Error in ParseLevel (invalid log level)
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return nil
	}

	mgr.config.LogLevel = "invalid-log-level" // Set an invalid log level

	t.Run("Error in ParseLevel", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})

	// Test case 4: Valid Log Level set in config
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return nil
	}

	mgr.config.LogLevel = "info" // Set a valid log level

	t.Run("Valid Log Level", func(t *testing.T) {
		mgr.processConfigMapChanges()
		if csmlog.GetLevel() != csmlog.InfoLevel {
			t.Errorf("Expected log level to be info, but got %v", csmlog.GetLevel())
		}
	})

	// Test case 5: Valid log format
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return nil
	}
	mgr.config.LogLevel = "info"
	mgr.config.LogFormat = "TEXT"

	t.Run("Valid Log Format", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})

	// Test case 6: Invalid log format
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return nil
	}
	mgr.config.LogFormat = "invalid"

	t.Run("Invalid Log Format", func(_ *testing.T) {
		mgr.processConfigMapChanges()
	})
}

func TestSetupConfigMapWatcher(_ *testing.T) {
	defaultWatchConfig := watchConfig
	defaultOnConfigChange := onConfigChange
	defaultGetUpdateConfigMapFunc := getUpdateConfigMapFunc
	defer func() {
		watchConfig = defaultWatchConfig
		onConfigChange = defaultOnConfigChange
		getUpdateConfigMapFunc = defaultGetUpdateConfigMapFunc
	}()

	watchConfig = func() {}
	onConfigChange = func(runner func(fsnotify.Event)) {
		runner(fsnotify.Event{})
	}
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return errors.New("config update error")
	}

	mgr := &ReplicatorManager{}
	mgr.setupConfigMapWatcher()
}

func TestMain(t *testing.T) {
	defaultGetConnectToCsiFunc := getConnectToCsiFunc
	defaultGetProbeForeverFunc := getProbeForeverFunc
	defaultGetReplicationCapabilitiesFunc := getReplicationCapabilitiesFunc
	defaultGetcreateReplicatorManagerFunc := getcreateReplicatorManagerFunc
	defaultGetManagerStart := getManagerStart
	defaultGetCtrlNewManager := getCtrlNewManager
	defaultGetWorkqueueReconcileRequest := getWorkqueueReconcileRequest
	defaultGetPersistentVolumeClaimReconcilerSetupWithManager := getPersistentVolumeClaimReconcilerSetupWithManager
	defaultGetPersistentVolumeReconcilerSetupWithManager := getPersistentVolumeReconcilerSetupWithManager
	defaultGetReplicationGroupReconcilerSetupWithManager := getReplicationGroupReconcilerSetupWithManager
	defaultGetControllerClient := getControllerClient
	defaultNewMetricsServerFunc := newMetricsServerFunc
	defaultStartMetricsServerFunc := startMetricsServerFunc
	defaultOSExit := osExit
	defaultSetupFlags := setupFlags

	osExitCode := 0

	after := func() {
		// Restore the original function after the test
		getConnectToCsiFunc = defaultGetConnectToCsiFunc
		getProbeForeverFunc = defaultGetProbeForeverFunc
		getReplicationCapabilitiesFunc = defaultGetReplicationCapabilitiesFunc
		getcreateReplicatorManagerFunc = defaultGetcreateReplicatorManagerFunc
		getManagerStart = defaultGetManagerStart
		getCtrlNewManager = defaultGetCtrlNewManager
		getWorkqueueReconcileRequest = defaultGetWorkqueueReconcileRequest
		getPersistentVolumeClaimReconcilerSetupWithManager = defaultGetPersistentVolumeClaimReconcilerSetupWithManager
		getPersistentVolumeReconcilerSetupWithManager = defaultGetPersistentVolumeReconcilerSetupWithManager
		getReplicationGroupReconcilerSetupWithManager = defaultGetReplicationGroupReconcilerSetupWithManager
		getControllerClient = defaultGetControllerClient
		newMetricsServerFunc = defaultNewMetricsServerFunc
		startMetricsServerFunc = defaultStartMetricsServerFunc
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
				getConnectToCsiFunc = func(_ string) (*grpc.ClientConn, error) {
					return &grpc.ClientConn{}, nil
				}

				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{
						replication.ReplicationCapability_RPC_CREATE_REMOTE_VOLUME:         true,
						replication.ReplicationCapability_RPC_CREATE_PROTECTION_GROUP:      true,
						replication.ReplicationCapability_RPC_DELETE_PROTECTION_GROUP:      true,
						replication.ReplicationCapability_RPC_MONITOR_PROTECTION_GROUP:     true,
						replication.ReplicationCapability_RPC_REPLICATION_ACTION_EXECUTION: true,
					}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
				getConnectToCsiFunc = func(_ string) (*grpc.ClientConn, error) {
					return &grpc.ClientConn{}, errors.New("error connecting to CSI driver")
				}

				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", errors.New("error waiting for the CSI driver to be ready")
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
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

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "error fetching replication capabilities",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, errors.New("error fetching replication capabilities")
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "ReplicationGroupReconciler - unable to create controller",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("ReplicationGroupReconciler - unable to create controller")
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "unable to parse log level",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, errors.New("error fetching replication capabilities")
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "invalid",
						},
					}, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "Unable to start manager",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, errors.New("unable to start manager")
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("ReplicationGroupReconciler - unable to create controller")
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "Failed to configure the controller manager",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, errors.New("unable to start manager")
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, errors.New("failed to configure the controller manager")
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("ReplicationGroupReconciler - unable to create controller")
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "Unable to create controller - PersistentVolumeClaim",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, errors.New("unable to start manager")
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, errors.New("failed to configure the controller manager")
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("unable to create controller PersistentVolumeClaim")
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "Unable to create controller - PersistentVolume",
			setup: func() {
				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, errors.New("unable to start manager")
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("unable to create controller PersistentVolume")
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return errors.New("problem running manager")
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
		{
			name: "Successful run with monitoring capability, cluster UID, and replication metrics",
			setup: func() {
				t.Setenv(constants.EnvReplicationMetricsEnabled, "true")
				t.Setenv(constants.EnvReplicationMetricsPort, "0")

				getConnectToCsiFunc = func(_ string) (*grpc.ClientConn, error) {
					return &grpc.ClientConn{}, nil
				}

				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{
						replication.ReplicationCapability_RPC_CREATE_REMOTE_VOLUME:         true,
						replication.ReplicationCapability_RPC_CREATE_PROTECTION_GROUP:      true,
						replication.ReplicationCapability_RPC_MONITOR_PROTECTION_GROUP:     true,
						replication.ReplicationCapability_RPC_REPLICATION_ACTION_EXECUTION: true,
					}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel:  "info",
							LogFormat: "json",
						},
					}, nil
				}

				getControllerClient = func(_ *rest.Config, _ *runtime.Scheme) (client.Client, error) {
					return fake.NewClientBuilder().WithObjects(&v1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "kube-system",
							UID:  "fake-uid",
						},
					}).Build(), nil
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getManagerStart = func(_ manager.Manager) error {
					return nil
				}

				newMetricsServerFunc = func(_ ...interface{}) *metricscommon.MetricsServer {
					return &metricscommon.MetricsServer{}
				}

				startMetricsServerFunc = func(_ *metricscommon.MetricsServer) error {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}

				setupFlags = func() flags {
					return flags{
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 0,
		},
		{
			name: "missing required replication capability",
			setup: func() {
				getConnectToCsiFunc = func(_ string) (*grpc.ClientConn, error) {
					return &grpc.ClientConn{}, nil
				}

				getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
					return "csi-driver", nil
				}

				getReplicationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
					capabilitySet := csiidentity.ReplicationCapabilitySet{
						// CREATE_REMOTE_VOLUME intentionally missing
						replication.ReplicationCapability_RPC_CREATE_PROTECTION_GROUP: true,
					}
					supportedActions := []*replication.SupportedActions{}
					return capabilitySet, supportedActions, nil
				}

				getCtrlNewManager = func(_ manager.Options) (manager.Manager, error) {
					return &mockManager{}, nil
				}

				getcreateReplicatorManagerFunc = func(_ context.Context, _ manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &repcnf.Config{
							LogLevel: "info",
						},
					}, nil
				}

				getControllerClient = func(_ *rest.Config, _ *runtime.Scheme) (client.Client, error) {
					return nil, errors.New("client error")
				}

				getWorkqueueReconcileRequest = func(_ time.Duration, _ time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
					return nil
				}

				getPersistentVolumeClaimReconcilerSetupWithManager = func(_ *controller.PersistentVolumeClaimReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getPersistentVolumeReconcilerSetupWithManager = func(_ *controller.PersistentVolumeReconciler, _ context.Context, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}

				getReplicationGroupReconcilerSetupWithManager = func(_ *controller.ReplicationGroupReconciler, _ ctrl.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
						metricsAddr:                ":8001",
						enableLeaderElection:       false,
						csiAddress:                 "/var/run/csi.sock",
						workerThreads:              2,
						retryIntervalStart:         time.Second,
						retryIntervalMax:           5 * time.Minute,
						operationTimeout:           300 * time.Second,
						pgContextKeyPrefix:         "prefix",
						domain:                     constants.DefaultDomain,
						monitoringInterval:         10 * time.Second,
						probeFrequency:             5 * time.Second,
						maxRetryDurationForActions: 10 * time.Minute,
					}
				}
			},
			expectedOsExitCode: 1,
		},
	}

	// Set Manifest version similar to how the image would be built.
	ManifestSemver = "1.0.0"

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

func TestReplicationMetricsCollectionIntervalFromEnv(t *testing.T) {
	t.Run("uses env override when valid", func(t *testing.T) {
		t.Setenv("X_CSI_REPLICATION_METRICS_COLLECTION_INTERVAL", "45s")

		got := getReplicationMetricsCollectionInterval(60 * time.Second)

		assert.Equal(t, 45*time.Second, got)
	})

	t.Run("falls back to default when env missing", func(t *testing.T) {
		got := getReplicationMetricsCollectionInterval(60 * time.Second)

		assert.Equal(t, 60*time.Second, got)
	})

	t.Run("falls back to default when env invalid", func(t *testing.T) {
		t.Setenv("X_CSI_REPLICATION_METRICS_COLLECTION_INTERVAL", "bad-value")

		got := getReplicationMetricsCollectionInterval(60 * time.Second)

		assert.Equal(t, 60*time.Second, got)
	})
}

func TestInitReplicationMetrics(t *testing.T) {
	originalNew := newMetricsServerFunc
	originalStart := startMetricsServerFunc
	defer func() {
		newMetricsServerFunc = originalNew
		startMetricsServerFunc = originalStart
	}()

	tests := []struct {
		name        string
		envEnabled  string
		envPort     string
		envCertFile string
		envKeyFile  string
		startErr    error
	}{
		{
			name:       "metrics disabled",
			envEnabled: "false",
		},
		{
			name:       "metrics enabled without TLS",
			envEnabled: "true",
			envPort:    "0",
		},
		{
			name:        "metrics enabled with TLS",
			envEnabled:  "true",
			envPort:     "8445",
			envCertFile: "/tmp/tls.crt",
			envKeyFile:  "/tmp/tls.key",
		},
		{
			name:       "metrics server start fails",
			envEnabled: "true",
			envPort:    "0",
			startErr:   errors.New("start failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(constants.EnvReplicationMetricsEnabled, tt.envEnabled)
			t.Setenv(constants.EnvReplicationMetricsPort, tt.envPort)
			t.Setenv(constants.EnvReplicationMetricsTLSCertFile, tt.envCertFile)
			t.Setenv(constants.EnvReplicationMetricsTLSKeyFile, tt.envKeyFile)

			started := make(chan struct{}, 1)
			newMetricsServerFunc = func(_ ...interface{}) *metricscommon.MetricsServer {
				return &metricscommon.MetricsServer{}
			}
			startMetricsServerFunc = func(_ *metricscommon.MetricsServer) error {
				started <- struct{}{}
				return tt.startErr
			}

			initReplicationMetrics("test-driver")

			if tt.envEnabled == "true" {
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("metrics server was not started")
				}
			}
		})
	}
}
