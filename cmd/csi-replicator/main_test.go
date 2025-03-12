package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/dell/csm-replication/pkg/config"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlcnf "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
		name           string
		loggerConfig   *logrus.Logger
		expectedOutput string
	}{
		{
			name:           "Test with valid loggerConfig",
			loggerConfig:   &logrus.Logger{},
			expectedOutput: "Started ConfigMap Watcher",
		},
		{
			name:           "Test with nil loggerConfig",
			loggerConfig:   nil,
			expectedOutput: "Started ConfigMap Watcher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			log.SetOutput(&output)

			mgr := &ReplicatorManager{}
			mgr.setupConfigMapWatcher(tt.loggerConfig)

			if !strings.Contains(output.String(), tt.expectedOutput) {
				t.Errorf("Expected output: %s, but got: %s", tt.expectedOutput, output.String())
			}
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

	after := func() {
		// Restore the original functions after the test
		getConnectToCsiFunc = defaultGetConnectToCsiFunc
		getProbeForeverFunc = defaultGetProbeForeverFunc
		getReplicationCapabilitiesFunc = defaultGetReplicationCapabilitiesFunc
		getcreateReplicatorManagerFunc = defaultGetcreateReplicatorManagerFunc
		getParseLevelFunc = defaultGetParseLevelFunc
		getManagerStart = defaultGetManagerStart
	}

	tests := []struct {
		name  string
		setup func()
	}{
		{
			name: "Successful execution",
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
				getcreateReplicatorManagerFunc = func(ctx context.Context, mgr manager.Manager) (*ReplicatorManager, error) {
					return &ReplicatorManager{
						config: &config.Config{
							LogLevel: "info",
						},
					}, nil
				}
				getParseLevelFunc = func(level string) (logrus.Level, error) {
					return logrus.InfoLevel, nil
				}
				getManagerStart = func(_ manager.Manager) error {
					return nil
				}
			},
		},
		// {
		// 	name: "CSI connection failure",
		// 	setup: func() {
		// 		getConnectToCsiFunc = func(_ string, _ logr.Logger) (*grpc.ClientConn, error) {
		// 			return nil, fmt.Errorf("connection error")
		// 		}
		// 	},
		// },
		// Add more test cases as needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			tt.setup()
			main()
		})
	}
}
