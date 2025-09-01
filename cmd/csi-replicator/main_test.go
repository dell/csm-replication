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
	"reflect"
	"testing"
	"time"

	controller "github.com/dell/csm-replication/controllers/csi-replicator"
	"github.com/dell/csm-replication/pkg/common/constants"
	repcnf "github.com/dell/csm-replication/pkg/config"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/sirupsen/logrus"
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
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

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
	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

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
				getConfig = func(_ context.Context, _ client.Client, _ repcnf.ControllerManagerOpts, _ record.EventRecorder, _ logr.Logger) (*repcnf.Config, error) {
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
				getConfig = func(_ context.Context, _ client.Client, _ repcnf.ControllerManagerOpts, _ record.EventRecorder, _ logr.Logger) (*repcnf.Config, error) {
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
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.Namespace
		wantErr bool
	}{
		{
			name: "Success",
			args: args{
				ctx: context.TODO(),
			},
			want: &v1.Namespace{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
				},
			},
			wantErr: false,
		},
		{
			name: "Error",
			args: args{
				ctx: context.TODO(),
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "Success" {
				getControllerClient = func(_ *rest.Config, _ *runtime.Scheme) (client.Client, error) {
					return fake.NewClientBuilder().WithObjects(tt.want).Build(), nil
				}
			} else if tt.name == "Error" {
				getControllerClient = func(_ *rest.Config, _ *runtime.Scheme) (client.Client, error) {
					return fake.NewClientBuilder().Build(), nil
				}
			}

			got, err := getClusterUID(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("getClusterUID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getClusterUID() = %v, want %v", got, tt.want)
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

	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	loggerConfig := logrus.New()
	mgr := &ReplicatorManager{
		Opts:    repcnf.ControllerManagerOpts{},
		Manager: mockMgr,
		config:  &repcnf.Config{},
	}

	t.Run("Success Test Case", func(_ *testing.T) {
		mgr.processConfigMapChanges(loggerConfig)
	})

	// Test case 2: Error in getUpdateConfigMapFunc
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return fmt.Errorf("config update error")
	}

	t.Run("Error in getUpdateConfigMapFunc", func(_ *testing.T) {
		mgr.processConfigMapChanges(loggerConfig)
	})

	// Test case 3: Error in ParseLevel (invalid log level)
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return nil
	}

	mgr.config.LogLevel = "invalid-log-level" // Set an invalid log level

	t.Run("Error in ParseLevel", func(_ *testing.T) {
		mgr.processConfigMapChanges(loggerConfig)
	})

	// Test case 4: Valid Log Level set in config
	getUpdateConfigMapFunc = func(_ *ReplicatorManager, _ context.Context) error {
		return nil
	}

	mgr.config.LogLevel = "info" // Set a valid log level

	t.Run("Valid Log Level", func(t *testing.T) {
		mgr.processConfigMapChanges(loggerConfig)
		if loggerConfig.GetLevel() != logrus.InfoLevel {
			t.Errorf("Expected log level to be info, but got %v", loggerConfig.GetLevel())
		}
	})
}

func TestSetupConfigMapWatcher(t *testing.T) {
	tests := []struct {
		name         string
		loggerConfig *logrus.Logger
	}{
		{
			name:         "Test with valid loggerConfig",
			loggerConfig: logrus.New(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			mgr := &ReplicatorManager{}
			mgr.setupConfigMapWatcher(tt.loggerConfig)
		})
	}
}

func TestMain(t *testing.T) {
	defaultGetConnectToCsiFunc := getConnectToCsiFunc
	defaultGetProbeForeverFunc := getProbeForeverFunc
	defaultGetReplicationCapabilitiesFunc := getReplicationCapabilitiesFunc
	defaultGetcreateReplicatorManagerFunc := getcreateReplicatorManagerFunc
	defaultGetParseLevelFunc := getParseLevelFunc
	defaultGetManagerStart := getManagerStart
	defaultGetCtrlNewManager := getCtrlNewManager
	defaultGetWorkqueueReconcileRequest := getWorkqueueReconcileRequest
	defaultGetPersistentVolumeClaimReconcilerSetupWithManager := getPersistentVolumeClaimReconcilerSetupWithManager
	defaultGetPersistentVolumeReconcilerSetupWithManager := getPersistentVolumeReconcilerSetupWithManager
	defaultGetReplicationGroupReconcilerSetupWithManager := getReplicationGroupReconcilerSetupWithManager
	defaultOSExit := osExit
	defaultSetupFlags := setupFlags

	osExitCode := 0

	after := func() {
		// Restore the original function after the test
		getConnectToCsiFunc = defaultGetConnectToCsiFunc
		getProbeForeverFunc = defaultGetProbeForeverFunc
		getReplicationCapabilitiesFunc = defaultGetReplicationCapabilitiesFunc
		getcreateReplicatorManagerFunc = defaultGetcreateReplicatorManagerFunc
		getParseLevelFunc = defaultGetParseLevelFunc
		getManagerStart = defaultGetManagerStart
		getCtrlNewManager = defaultGetCtrlNewManager
		getWorkqueueReconcileRequest = defaultGetWorkqueueReconcileRequest
		getPersistentVolumeClaimReconcilerSetupWithManager = defaultGetPersistentVolumeClaimReconcilerSetupWithManager
		getPersistentVolumeReconcilerSetupWithManager = defaultGetPersistentVolumeReconcilerSetupWithManager
		getReplicationGroupReconcilerSetupWithManager = defaultGetReplicationGroupReconcilerSetupWithManager
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
				getConnectToCsiFunc = func(_ string, _ logr.Logger) (*grpc.ClientConn, error) {
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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
				getConnectToCsiFunc = func(_ string, _ logr.Logger) (*grpc.ClientConn, error) {
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.Level(0), fmt.Errorf("unable to parse log level")
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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

				getParseLevelFunc = func(_ string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
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
