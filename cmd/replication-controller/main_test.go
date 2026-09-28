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

	metricscommon "github.com/dell/csm-metrics-common/pkg/server"
	repController "github.com/dell/csm-replication/controllers/replication-controller"
	"github.com/dell/csm-replication/pkg/common/constants"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csmlog"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sync/singleflight"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcnf "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

type mockManager struct {
	manager.Manager
	client              client.Client
	scheme              *runtime.Scheme
	eventRec            record.EventRecorder
	config              *config.Config
	singleflight        singleflight.Group
	controllerName      string
	controllerGroupName string
	// reconciler          *controller.PersistentVolumeReconciler
}

func TestMainFlow(_ *testing.T) {
	originalSetupFlags := setupFlags
	originalCreateManagerInstance := createManagerInstance
	originalSetupControllerManager := setupControllerManager
	originalGetSecretController := getSecretController
	originalPersistentVolumeRecon := getPersistentVolumeReconciler
	originalReplicationGroupRecon := getReplicationGroupReconciler
	originalPersistentVolumeClaimRecon := getPersistentVolumeClaimReconciler
	originalGetManagerStart := getManagerStart
	originalOsExit := osExit
	defer func() {
		setupFlags = originalSetupFlags
		createManagerInstance = originalCreateManagerInstance
		setupControllerManager = originalSetupControllerManager
		getSecretController = originalGetSecretController
		getPersistentVolumeReconciler = originalPersistentVolumeRecon
		getReplicationGroupReconciler = originalReplicationGroupRecon
		getPersistentVolumeClaimReconciler = originalPersistentVolumeClaimRecon
		getManagerStart = originalGetManagerStart
		osExit = originalOsExit
	}()

	setupFlags = func() (map[string]string, context.Context) {
		return map[string]string{
			"metrics-addr":                 ":8081",
			"leader-election":              "false",
			"prefix":                       "replication.storage.dell.com",
			"worker-threads":               "2",
			"retry-interval-start":         "1s",
			"retry-interval-max":           "5m0s",
			"disable-pvc-remap":            "false",
			"enable-kubevirt-pvc-remap":    "false",
			"allow-pvc-creation-on-target": "false",
		}, context.Background()
	}

	createManagerInstance = func(_ map[string]string) manager.Manager {
		return &mockManager{}
	}

	setupControllerManager = func(_ context.Context, mgr manager.Manager) *ControllerManager {
		return &ControllerManager{
			Manager: mgr,
			config: &config.Config{
				LogLevel:  "info",
				LogFormat: "json",
			},
		}
	}

	getSecretController = func(*ControllerManager) error {
		return nil
	}
	getPersistentVolumeReconciler = func(_ *repController.PersistentVolumeReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
		return nil
	}
	getReplicationGroupReconciler = func(_ *repController.ReplicationGroupReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
		return nil
	}
	getPersistentVolumeClaimReconciler = func(_ *repController.PersistentVolumeClaimReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
		return nil
	}
	getManagerStart = func(_ manager.Manager) error {
		return nil
	}
	osExit = func(int) {}

	main()
}

func TestCreateControllerManager(t *testing.T) {
	originalGetConnectionControllerClient := getConnectionControllerClient
	originalGetConfig := getConfig
	originalGetConfigPrintConfig := getConfigPrintConfig
	originalGetCtrlNewManager := getCtrlNewManager
	defer func() {
		getConnectionControllerClient = originalGetConnectionControllerClient
		getConfig = originalGetConfig
		getConfigPrintConfig = originalGetConfigPrintConfig
		getCtrlNewManager = originalGetCtrlNewManager
	}()

	t.Run("error fetching client", func(t *testing.T) {
		getConnectionControllerClient = func(_ *runtime.Scheme) (client.Client, error) {
			return nil, errors.New("client fail")
		}
		_, err := createControllerManager(context.Background(), &mockManager{})
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		getConnectionControllerClient = func(_ *runtime.Scheme) (client.Client, error) {
			return nil, nil
		}
		getConfig = func(_ context.Context, _ client.Client, _ config.ControllerManagerOpts, _ record.EventRecorder) (*config.Config, error) {
			return &config.Config{}, nil
		}
		printed := false
		getConfigPrintConfig = func(_ *config.Config) {
			printed = true
		}
		getCtrlNewManager = func(_ ctrl.Options) (manager.Manager, error) {
			return &mockManager{}, nil
		}

		mgr, err := createControllerManager(context.Background(), &mockManager{})
		assert.NoError(t, err)
		assert.NotNil(t, mgr)
		assert.True(t, printed)
	})
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

type mockSecretController struct {
	controller.Controller
	mock.Mock
}

func (m *mockSecretController) Start(_ context.Context) error {
	return nil
}

func (m *mockSecretController) Watch(_ source.TypedSource[reconcile.Request]) error {
	return nil
}

func (m *mockSecretController) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(reconcile.Result), args.Error(1)
}

func TestControllerManager_reconcileSecretUpdates(t *testing.T) {
	// Saving original function
	defaultGetUpdateConfigOnSecretEvent := getUpdateConfigOnSecretEvent

	after := func() {
		getUpdateConfigOnSecretEvent = defaultGetUpdateConfigOnSecretEvent
	}

	tests := []struct {
		name        string
		setup       func()
		updateError error
		expectedErr bool
	}{
		{
			name: "Config update is successful",
			setup: func() {
				getUpdateConfigOnSecretEvent = func(_ *ControllerManager, _ context.Context, _ reconcile.Request, _ record.EventRecorder) error {
					return nil
				}
			},
			updateError: nil,
			expectedErr: false,
		},
		{
			name: "Config update fails",
			setup: func() {
				getUpdateConfigOnSecretEvent = func(_ *ControllerManager, _ context.Context, _ reconcile.Request, _ record.EventRecorder) error {
					return errors.New("failed to update config")
				}
			},
			updateError: errors.New("failed to update config"),
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockMgr := &mockManager{}

			mockSecretController := &mockSecretController{}

			tt.setup()
			defer after()
			ctx := context.Background()
			mgr := &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			}
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
			}

			result, err := mgr.reconcileSecretUpdates(ctx, request)

			assert.Equal(t, tt.expectedErr, err != nil)
			assert.Equal(t, reconcile.Result{}, result)
		})
	}
}

func TestStartSecretControllerHelper(t *testing.T) {
	original := getSecretController
	defer func() {
		getSecretController = original
	}()

	ctrlMgr := &ControllerManager{
		config: &config.Config{},
	}

	called := false
	getSecretController = func(*ControllerManager) error {
		called = true
		return nil
	}
	startSecretController(ctrlMgr)
	assert.True(t, called)

	getSecretController = func(*ControllerManager) error {
		return errors.New("boom")
	}
	startSecretController(ctrlMgr)
}

func TestControllerManager_startSecretController(t *testing.T) {
	after := func() {}

	tests := []struct {
		name        string
		setup       func()
		expectedErr bool
	}{
		{
			name: "Success",
			setup: func() {
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockMgr := &mockManager{}

			mockSecretController := &mockSecretController{}

			tt.setup()
			defer after()

			mgr := &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			}

			err := mgr.startSecretController()

			assert.Equal(t, tt.expectedErr, err != nil)
		})
	}
}

func TestControllerManager_processConfigMapChanges(t *testing.T) {
	defaultGetUpdateConfigMap := getUpdateConfigMap

	after := func() {
		getUpdateConfigMap = defaultGetUpdateConfigMap
	}

	tests := []struct {
		name          string
		setup         func()
		expectedLevel csmlog.Level
		expectedError error
	}{
		{
			name:          "Error parsing the config",
			setup:         func() {},
			expectedLevel: csmlog.InfoLevel,
		},
		{
			name: "Success",
			setup: func() {
				getUpdateConfigMap = func(_ *ControllerManager, _ context.Context, _ record.EventRecorder) error {
					return nil
				}
			},
			expectedLevel: csmlog.InfoLevel,
		},
		{
			name: "Success with valid log format",
			setup: func() {
				getUpdateConfigMap = func(_ *ControllerManager, _ context.Context, _ record.EventRecorder) error {
					return nil
				}
			},
			expectedLevel: csmlog.InfoLevel,
		},
		{
			name: "Success with invalid log format",
			setup: func() {
				getUpdateConfigMap = func(_ *ControllerManager, _ context.Context, _ record.EventRecorder) error {
					return nil
				}
			},
			expectedLevel: csmlog.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockMgr := &mockManager{}

			mockSecretController := &mockSecretController{}

			tt.setup()
			defer after()

			cfg := &config.Config{}
			if tt.name == "Success with valid log format" {
				cfg.LogFormat = "TEXT"
			} else if tt.name == "Success with invalid log format" {
				cfg.LogFormat = "invalid"
			}

			mgr := &ControllerManager{
				Manager:          mockMgr,
				config:           cfg,
				SecretController: mockSecretController,
			}

			mgr.processConfigMapChanges()

			assert.Equal(t, tt.expectedLevel, csmlog.GetLevel())
		})
	}
}

func TestControllerManager_setupConfigMapWatcher(t *testing.T) {
	tests := []struct {
		name          string
		setup         func()
		expectedError error
	}{
		{
			name: "Success",
			setup: func() {
				viper.Set("LogLevel", "info")
			},
			expectedError: nil,
		},
		{
			name: "Error parsing log level",
			setup: func() {
				viper.Set("LogLevel", "info")
				viper.Set("LogLevel", "invalid")
			},
			expectedError: fmt.Errorf("error parsing the config: unable to parse log level"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			mockMgr := &mockManager{}

			mgr := &ControllerManager{
				Opts:    config.ControllerManagerOpts{},
				Manager: mockMgr,
				config:  &config.Config{},
			}

			tt.setup()

			mgr.setupConfigMapWatcher()
		})
	}
}

func TestControllerManager_createControllerManager(t *testing.T) {
	defaultGetConfig := getConfig
	defaultGetConfigPrintConfig := getConfigPrintConfig
	defaultGetConnectionControllerClient := getConnectionControllerClient

	after := func() {
		getConfig = defaultGetConfig
		getConfigPrintConfig = defaultGetConfigPrintConfig
		getConnectionControllerClient = defaultGetConnectionControllerClient
	}

	tests := []struct {
		name                      string
		setup                     func()
		expectedControllerManager *ControllerManager
		expectedError             bool
	}{
		{
			name: "Failed getConnectionControllerClient",
			setup: func() {
				getConnectionControllerClient = func(_ *runtime.Scheme) (client.Client, error) {
					return nil, errors.New("error getting connection controller client")
				}
			},
			expectedControllerManager: nil,
			expectedError:             true,
		},
		{
			name: "Failed createControllerManager",
			setup: func() {
				getConnectionControllerClient = func(_ *runtime.Scheme) (client.Client, error) {
					return nil, nil
				}
				getConfig = func(_ context.Context, _ client.Client, _ config.ControllerManagerOpts, _ record.EventRecorder) (*config.Config, error) {
					return &config.Config{}, errors.New("error getting config")
				}
			},
			expectedControllerManager: nil,
			expectedError:             true,
		},
		{
			name: "Success createControllerManager",
			setup: func() {
				getConnectionControllerClient = func(_ *runtime.Scheme) (client.Client, error) {
					return nil, nil
				}
				getConfig = func(_ context.Context, _ client.Client, _ config.ControllerManagerOpts, _ record.EventRecorder) (*config.Config, error) {
					return &config.Config{}, nil
				}
				getConfigPrintConfig = func(_ *config.Config) {}
			},
			expectedControllerManager: &ControllerManager{
				Opts: config.ControllerManagerOpts{
					UseConfFileFormat: true,
					WatchNamespace:    "dell-replication-controller",
					ConfigDir:         "deploy",
					ConfigFileName:    "config",
					InCluster:         false,
					Mode:              "controller",
				},
				config: &config.Config{},
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockMgr := &mockManager{}

			tt.setup()
			defer after()

			mgr, err := createControllerManager(context.Background(), mockMgr)

			assert.Equal(t, tt.expectedError, err != nil)

			if !tt.expectedError {
				assert.Equal(t, tt.expectedControllerManager.Opts, mgr.Opts)
				assert.Equal(t, tt.expectedControllerManager.config, mgr.config)
			}
		})
	}
}

func TestSetupFlags(t *testing.T) {
	// Call the setupFlags function
	flags, ctx := setupFlags()

	// Assert the expected values
	expected := map[string]string{
		"metrics-addr":         ":8081",
		"leader-election":      "false",
		"prefix":               constants.DefaultDomain,
		"worker-threads":       "2",
		"retry-interval-start": "1s",
		"retry-interval-max":   "5m0s",
	}

	for key, expectedValue := range expected {
		if flags[key] != expectedValue {
			t.Errorf("Expected flag value for %s to be %s, but got %s", key, expectedValue, flags[key])
		}
	}

	assert.NotNil(t, flags)
	assert.NotNil(t, ctx)
}

func TestProcessLogFormat(t *testing.T) {
	tests := []struct {
		name      string
		logFormat string
	}{
		{
			name:      "Valid log format TEXT",
			logFormat: "TEXT",
		},
		{
			name:      "Valid log format JSON",
			logFormat: "JSON",
		},
		{
			name:      "Invalid log format",
			logFormat: "invalid",
		},
		{
			name:      "Empty log format",
			logFormat: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			setLogFormat(tt.logFormat)
		})
	}
}

func TestProcessLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		expected csmlog.Level
	}{
		{
			name:     "Valid log level",
			logLevel: "info",
			expected: csmlog.InfoLevel,
		},
		{
			name:     "Invalid log level",
			logLevel: "invalid",
			expected: csmlog.InfoLevel,
		},
		{
			name:     "Empty log level",
			logLevel: "",
			expected: csmlog.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setLogLevel(tt.logLevel)
			actual := csmlog.GetLevel()
			if actual != tt.expected {
				t.Errorf("Expected log level: %v, but got: %v", tt.expected, actual)
			}
		})
	}
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

func TestStartManager(t *testing.T) {
	originalGetManagerStart := getManagerStart
	originalOsExit := osExit

	after := func() {
		getManagerStart = originalGetManagerStart
		osExit = originalOsExit
	}

	tests := []struct {
		name    string
		manager manager.Manager
		setup   func()
		wantErr bool
	}{
		{
			name:    "Manager is nil",
			manager: nil,
			wantErr: true,
		},
		{
			name:    "Manager is not nil",
			manager: &mockManager{},
			setup: func() {
				getManagerStart = func(_ manager.Manager) error {
					return errors.New("problem running manager")
				}
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
			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}
			if tt.name == "Manager is nil" {
				// Checked for Panic here as the panic is due to chain function calls and actual function is covered as expected
				// And actual code for the chained functions are covered on specific test cases related to that function
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("The code did not panic")
					}
				}()
			}
			startManager(tt.manager)
			if tt.name == "Manager is not nil" {
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, but got %d", exitCode)
				}
			}
		})
	}
}

func TestCreatePersistentVolumeReconciler(t *testing.T) {
	mockMgr := &mockManager{}

	mockSecretController := &mockSecretController{}
	originalGetPersistentVolumeReconciler := getPersistentVolumeReconciler
	originalOsExit := osExit

	after := func() {
		getPersistentVolumeReconciler = originalGetPersistentVolumeReconciler
		osExit = originalOsExit
	}
	tests := []struct {
		name           string
		manager        manager.Manager
		controllerMgr  *ControllerManager
		domain         string
		workerThreads  int
		expRateLimiter workqueue.TypedRateLimiter[reconcile.Request]
		setup          func()
		wantErr        bool
	}{
		{
			name:           "Manager is nil",
			manager:        nil,
			controllerMgr:  nil,
			domain:         "abc",
			workerThreads:  2,
			expRateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			wantErr:        false,
		},
		{
			name:    "Manager is not nil",
			manager: &mockManager{},
			controllerMgr: &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			},
			domain:         "abc",
			workerThreads:  2,
			expRateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			setup: func() {
				getPersistentVolumeReconciler = func(_ *repController.PersistentVolumeReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:    "Manager is not nil and expected error",
			manager: &mockManager{},
			controllerMgr: &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			},
			domain:         "abc",
			workerThreads:  3,
			expRateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			setup: func() {
				getPersistentVolumeReconciler = func(_ *repController.PersistentVolumeReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("problem running manager")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			if tt.setup != nil {
				tt.setup()
			}
			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}
			if tt.name == "Manager is nil" {
				// Checked for Panic here as the panic is due to chain function calls and actual function is covered as expected
				// And actual code for the chained functions are covered on specific test cases related to that function
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("The code did not panic")
					}
				}()
			}
			createPersistentVolumeReconciler(tt.manager, tt.controllerMgr, tt.domain, tt.workerThreads, tt.expRateLimiter)
			if tt.name == "Manager is not nil" {
				if exitCode != 0 {
					t.Errorf("Expected exit code 0, but got %d", exitCode)
				}
			}
			if tt.name == "Manager is not nil and expected error" {
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, but got %d", exitCode)
				}
			}
		})
	}
}

func TestCreateReplicationGroupReconciler(t *testing.T) {
	mockMgr := &mockManager{}

	mockSecretController := &mockSecretController{}
	originalGetReplicationGroupReconciler := getReplicationGroupReconciler
	originalOsExit := osExit

	after := func() {
		getReplicationGroupReconciler = originalGetReplicationGroupReconciler
		osExit = originalOsExit
	}
	tests := []struct {
		name           string
		manager        manager.Manager
		controllerMgr  *ControllerManager
		domain         string
		workerThreads  int
		expRateLimiter workqueue.TypedRateLimiter[reconcile.Request]
		setup          func()
		wantErr        bool
	}{
		{
			name:           "Manager is nil",
			manager:        nil,
			controllerMgr:  nil,
			domain:         "abc",
			workerThreads:  2,
			expRateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			wantErr:        false,
		},
		{
			name:    "Manager is not nil",
			manager: &mockManager{},
			controllerMgr: &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			},
			domain:         "abc",
			workerThreads:  2,
			expRateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			setup: func() {
				getReplicationGroupReconciler = func(_ *repController.ReplicationGroupReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:    "Manager is not nil and expected error",
			manager: &mockManager{},
			controllerMgr: &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			},
			domain:         "abc",
			workerThreads:  3,
			expRateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			setup: func() {
				getReplicationGroupReconciler = func(_ *repController.ReplicationGroupReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("problem running manager")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			if tt.setup != nil {
				tt.setup()
			}
			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}
			if tt.name == "Manager is nil" {
				// Checked for Panic here as the panic is due to chain function calls and actual function is covered as expected
				// And actual code for the chained functions are covered on specific test cases related to that function
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("The code did not panic")
					}
				}()
			}
			createReplicationGroupReconciler(tt.manager, tt.controllerMgr, tt.domain, tt.workerThreads, tt.expRateLimiter, false, false)
			if tt.name == "Manager is not nil" {
				if exitCode != 0 {
					t.Errorf("Expected exit code 0, but got %d", exitCode)
				}
			}
			if tt.name == "Manager is not nil and expected error" {
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, but got %d", exitCode)
				}
			}
		})
	}
}

func TestCreatePersistentVolumeClaimReconciler(t *testing.T) {
	mockMgr := &mockManager{}

	mockSecretController := &mockSecretController{}
	originalGetPersistentVolumeClaimReconciler := getPersistentVolumeClaimReconciler
	originalOsExit := osExit

	after := func() {
		getPersistentVolumeClaimReconciler = originalGetPersistentVolumeClaimReconciler
		osExit = originalOsExit
	}
	tests := []struct {
		name                     string
		manager                  manager.Manager
		controllerMgr            *ControllerManager
		domain                   string
		workerThreads            int
		expRateLimiter           workqueue.TypedRateLimiter[reconcile.Request]
		setup                    func()
		wantErr                  bool
		allowPVCCreationOnTarget bool
	}{
		{
			name:                     "Manager is nil",
			manager:                  nil,
			controllerMgr:            nil,
			domain:                   "abc",
			workerThreads:            2,
			expRateLimiter:           workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			wantErr:                  false,
			allowPVCCreationOnTarget: false,
		},
		{
			name:    "Manager is not nil",
			manager: &mockManager{},
			controllerMgr: &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			},
			domain:         "abc",
			workerThreads:  2,
			expRateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			setup: func() {
				getPersistentVolumeClaimReconciler = func(_ *repController.PersistentVolumeClaimReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return nil
				}
			},
			wantErr:                  false,
			allowPVCCreationOnTarget: false,
		},
		{
			name:    "Manager is not nil and expected error",
			manager: &mockManager{},
			controllerMgr: &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			},
			domain:         "abc",
			workerThreads:  3,
			expRateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			setup: func() {
				getPersistentVolumeClaimReconciler = func(_ *repController.PersistentVolumeClaimReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
					return errors.New("problem running manager")
				}
			},
			wantErr:                  true,
			allowPVCCreationOnTarget: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			if tt.setup != nil {
				tt.setup()
			}
			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}
			if tt.name == "Manager is nil" {
				// Checked for Panic here as the panic is due to chain function calls and actual function is covered as expected
				// And actual code for the chained functions are covered on specific test cases related to that function
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("The code did not panic")
					}
				}()
			}
			createPersistentVolumeClaimReconciler(tt.manager, tt.controllerMgr, tt.domain, tt.workerThreads, tt.expRateLimiter, tt.allowPVCCreationOnTarget)
			if tt.name == "Manager is not nil" {
				if exitCode != 0 {
					t.Errorf("Expected exit code 0, but got %d", exitCode)
				}
			}
			if tt.name == "Manager is not nil and expected error" {
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, but got %d", exitCode)
				}
			}
		})
	}
}

func TestStartSecretController(t *testing.T) {
	mockMgr := &mockManager{}

	mockSecretController := &mockSecretController{}

	originalgetSecretController := getSecretController
	originalOsExit := osExit

	after := func() {
		getSecretController = originalgetSecretController
		osExit = originalOsExit
	}

	tests := []struct {
		name          string
		controllerMgr *ControllerManager
		setup         func()
		expectedErr   bool
	}{
		{
			name: "Success",
			controllerMgr: &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			},
			setup: func() {
				getSecretController = func(_ *ControllerManager) error {
					return nil
				}
			},
			expectedErr: false,
		},
		{
			name: "Error",
			controllerMgr: &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			},
			setup: func() {
				getSecretController = func(_ *ControllerManager) error {
					return errors.New("failed to setup secret controller. Continuing")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			defer after()

			startSecretController(tt.controllerMgr)
		})
	}
}

func TestSetupControllerManager(t *testing.T) {
	mockMgr := &mockManager{}

	ctx := context.Background()
	originalOsExit := osExit

	after := func() {
		osExit = originalOsExit
	}

	tests := []struct {
		name        string
		mgr         manager.Manager
		setup       func()
		expectedErr bool
	}{
		{
			name:        "Success",
			mgr:         mockMgr,
			setup:       func() {},
			expectedErr: false,
		},
		{
			name:        "Failure",
			mgr:         mockMgr,
			setup:       func() {},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer after()
			if tt.setup != nil {
				tt.setup()
			}
			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}

			setupControllerManager(ctx, tt.mgr)

			if tt.name == "Failure" {
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, but got %d", exitCode)
				}
			}
		})
	}
}

func TestCreateManagerInstance(t *testing.T) {
	tests := []struct {
		name      string
		flagMap   map[string]string
		wantError bool
	}{
		{
			name:      "Successful creation of manager.Manager",
			flagMap:   map[string]string{"metrics-addr": ":8080", "leader-election": "true"},
			wantError: false,
		},
		{
			name:      "Error in getCtrlNewManager",
			flagMap:   map[string]string{"metrics-addr": ":8080", "leader-election": "true"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetCtrlNewManager := getCtrlNewManager
			originalOsExit := osExit
			after := func() {
				osExit = originalOsExit
				getCtrlNewManager = originalGetCtrlNewManager
			}

			getCtrlNewManager = func(_ ctrl.Options) (manager.Manager, error) {
				if tt.wantError {
					return nil, errors.New("error getting manager")
				}
				return &mockManager{}, nil
			}

			// Override osExit to capture the exit code
			var exitCode int
			osExit = func(code int) {
				exitCode = code
			}

			defer after()

			_ = createManagerInstance(tt.flagMap)

			if tt.wantError {
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, but got %d", exitCode)
				}
			}
		})
	}
}

func TestMain(t *testing.T) {
	defaultCreateManagerInstance := createManagerInstance
	defaultOSExit := osExit
	defaultSetupFlags := setupFlags
	defaultGetCtrlNewManager := getCtrlNewManager
	defaultSetupControllerManager := setupControllerManager
	defaultGetManagerStart := getManagerStart

	osExitCode := 0

	after := func() {
		// Restore the original function after the test
		createManagerInstance = defaultCreateManagerInstance
		osExit = defaultOSExit
		setupFlags = defaultSetupFlags
		getCtrlNewManager = defaultGetCtrlNewManager
		setupControllerManager = defaultSetupControllerManager
		getManagerStart = defaultGetManagerStart
	}

	tests := []struct {
		name               string
		setup              func()
		expectedOsExitCode int
	}{
		{
			name: "Manager instance is nil",
			setup: func() {
				setupFlags = func() (map[string]string, context.Context) {
					flagMap := make(map[string]string)
					return flagMap, context.Background()
				}

				createManagerInstance = func(_ map[string]string) manager.Manager {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}
			},
			expectedOsExitCode: 0,
		},
		{
			name: "Manager is nil",
			setup: func() {
				setupFlags = func() (map[string]string, context.Context) {
					return map[string]string{"metrics-addr": ":8080", "leader-election": "true"}, context.Background()
				}

				mockMgr := &mockManager{}

				createManagerInstance = func(_ map[string]string) manager.Manager {
					return mockMgr
				}

				setupControllerManager = func(_ context.Context, _ manager.Manager) *ControllerManager {
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

			initReplicationMetrics()

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

func TestSetupConfigMapWatcher(_ *testing.T) {
	originalWatch := watchConfigFunc
	originalOnChange := onConfigChangeFunc
	originalUpdateConfigMap := getUpdateConfigMap
	defer func() {
		watchConfigFunc = originalWatch
		onConfigChangeFunc = originalOnChange
		getUpdateConfigMap = originalUpdateConfigMap
	}()

	watchConfigFunc = func() {}
	onConfigChangeFunc = func(runner func(fsnotify.Event)) {
		runner(fsnotify.Event{})
	}
	getUpdateConfigMap = func(_ *ControllerManager, _ context.Context, _ record.EventRecorder) error {
		return errors.New("config update error")
	}

	mgr := &ControllerManager{Manager: &mockManager{}}
	mgr.setupConfigMapWatcher()
}
