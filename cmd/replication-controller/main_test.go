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

	repController "github.com/dell/csm-replication/controllers/replication-controller"
	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/sirupsen/logrus"
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
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
	// reconciler          *controller.PersistentVolumeReconciler
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

type mockSecretController struct {
	mock.Mock
	logger logr.Logger
}

func (m *mockSecretController) GetLogger() logr.Logger {
	return m.logger
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
	defaultGetSecretControllerLogger := getSecretControllerLogger
	defaultGetUpdateConfigOnSecretEvent := getUpdateConfigOnSecretEvent

	after := func() {
		getSecretControllerLogger = defaultGetSecretControllerLogger
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
				getSecretControllerLogger = func(_ *ControllerManager, _ reconcile.Request) logr.Logger {
					return logr.Logger{}
				}
				getUpdateConfigOnSecretEvent = func(_ *ControllerManager, _ context.Context, _ reconcile.Request, _ record.EventRecorder, _ logr.Logger) error {
					return nil
				}
			},
			updateError: nil,
			expectedErr: false,
		},
		{
			name: "Config update fails",
			setup: func() {
				getSecretControllerLogger = func(_ *ControllerManager, _ reconcile.Request) logr.Logger {
					return logr.Logger{}
				}
				getUpdateConfigOnSecretEvent = func(_ *ControllerManager, _ context.Context, _ reconcile.Request, _ record.EventRecorder, _ logr.Logger) error {
					return errors.New("failed to update config")
				}
			},
			updateError: errors.New("failed to update config"),
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockMgr := &mockManager{
				logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
			}

			mockSecretController := &mockSecretController{
				logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
			}

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
			mockMgr := &mockManager{
				logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
			}

			mockSecretController := &mockSecretController{
				logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
			}

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
		loggerConfig  *logrus.Logger
		expectedLevel logrus.Level
		expectedError error
	}{
		{
			name:  "Error parsing the config",
			setup: func() {},
			loggerConfig: &logrus.Logger{
				Level: logrus.InfoLevel,
			},
			expectedLevel: logrus.InfoLevel,
		},
		{
			name: "Success",
			setup: func() {
				getUpdateConfigMap = func(_ *ControllerManager, _ context.Context, _ record.EventRecorder) error {
					return nil
				}
			},
			loggerConfig: &logrus.Logger{
				Level: logrus.InfoLevel,
			},
			expectedLevel: logrus.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockMgr := &mockManager{
				logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
			}

			mockSecretController := &mockSecretController{
				logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
			}

			tt.setup()
			defer after()

			mgr := &ControllerManager{
				Manager:          mockMgr,
				config:           &config.Config{},
				SecretController: mockSecretController,
			}

			mgr.processConfigMapChanges(tt.loggerConfig)

			assert.Equal(t, tt.expectedLevel, tt.loggerConfig.GetLevel())
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
		t.Run(tt.name, func(t *testing.T) {
			mockMgr := &mockManager{
				logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
			}

			loggerConfig := logrus.New()
			mgr := &ControllerManager{
				Opts:    config.ControllerManagerOpts{},
				Manager: mockMgr,
				config:  &config.Config{},
			}

			tt.setup()

			mgr.setupConfigMapWatcher(loggerConfig)
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
				getConfig = func(_ context.Context, _ client.Client, _ config.ControllerManagerOpts, _ record.EventRecorder, _ logr.Logger) (*config.Config, error) {
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
				getConfig = func(_ context.Context, _ client.Client, _ config.ControllerManagerOpts, _ record.EventRecorder, _ logr.Logger) (*config.Config, error) {
					return &config.Config{}, nil
				}
				getConfigPrintConfig = func(__ *config.Config, _ logr.Logger) {}
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
			mockMgr := &mockManager{
				logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
			}

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
	flags, setupLog, logrusLog, ctx := setupFlags()

	// Assert the expected values
	expected := map[string]string{
		"metrics-addr":         ":8081",
		"leader-election":      "false",
		"prefix":               common.DefaultDomain,
		"worker-threads":       "2",
		"retry-interval-start": "1s",
		"retry-interval-max":   "5m0s",
	}

	for key, expectedValue := range expected {
		if flags[key] != expectedValue {
			t.Errorf("Expected flag value for %s to be %s, but got %s", key, expectedValue, flags[key])
		}
	}

	assert.NotNil(t, setupLog)
	assert.NotNil(t, logrusLog)
	assert.NotNil(t, ctx)
}

func TestProcessLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		expected logrus.Level
	}{
		{
			name:     "Valid log level",
			logLevel: "info",
			expected: logrus.InfoLevel,
		},
		{
			name:     "Invalid log level",
			logLevel: "invalid",
			expected: logrus.InfoLevel,
		},
		{
			name:     "Empty log level",
			logLevel: "",
			expected: logrus.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logrusLog := logrus.New()
			processLogLevel(tt.logLevel, logrusLog)
			actual := logrusLog.GetLevel()
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
		name     string
		manager  manager.Manager
		setupLog logr.Logger
		setup    func()
		wantErr  bool
	}{
		{
			name:     "Manager is nil",
			manager:  nil,
			setupLog: ctrl.Log.WithName("test-logger"),
			wantErr:  true,
		},
		{
			name:    "Manager is not nil",
			manager: &mockManager{},
			setup: func() {
				getManagerStart = func(_ manager.Manager) error {
					return errors.New("problem running manager")
				}
			},
			setupLog: ctrl.Log.WithName("test-logger"),
			wantErr:  false,
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
			startManager(tt.manager, tt.setupLog)
			if tt.name == "Manager is not nil" {
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, but got %d", exitCode)
				}
			}
		})
	}
}

func TestCreatePersistentVolumeReconciler(t *testing.T) {
	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	mockSecretController := &mockSecretController{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}
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
		setupLog       logr.Logger
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
			setupLog:       ctrl.Log.WithName("test-logger"),
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
			setupLog:       ctrl.Log.WithName("test-logger"),
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
			setupLog:       ctrl.Log.WithName("test-logger"),
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
			createPersistentVolumeReconciler(tt.manager, tt.controllerMgr, tt.domain, tt.workerThreads, tt.expRateLimiter, tt.setupLog)
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
	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	mockSecretController := &mockSecretController{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}
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
		setupLog       logr.Logger
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
			setupLog:       ctrl.Log.WithName("test-logger"),
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
			setupLog:       ctrl.Log.WithName("test-logger"),
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
			setupLog:       ctrl.Log.WithName("test-logger"),
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
			createReplicationGroupReconciler(tt.manager, tt.controllerMgr, tt.domain, tt.workerThreads, tt.expRateLimiter, tt.setupLog)
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
	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	mockSecretController := &mockSecretController{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}
	originalGetPersistentVolumeClaimReconciler := getPersistentVolumeClaimReconciler
	originalOsExit := osExit

	after := func() {
		getPersistentVolumeClaimReconciler = originalGetPersistentVolumeClaimReconciler
		osExit = originalOsExit
	}
	tests := []struct {
		name           string
		manager        manager.Manager
		controllerMgr  *ControllerManager
		domain         string
		workerThreads  int
		expRateLimiter workqueue.TypedRateLimiter[reconcile.Request]
		setupLog       logr.Logger
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
			setupLog:       ctrl.Log.WithName("test-logger"),
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
			setupLog:       ctrl.Log.WithName("test-logger"),
			setup: func() {
				getPersistentVolumeClaimReconciler = func(_ *repController.PersistentVolumeClaimReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
			setupLog:       ctrl.Log.WithName("test-logger"),
			setup: func() {
				getPersistentVolumeClaimReconciler = func(_ *repController.PersistentVolumeClaimReconciler, _ manager.Manager, _ workqueue.TypedRateLimiter[reconcile.Request], _ int) error {
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
			createPersistentVolumeClaimReconciler(tt.manager, tt.controllerMgr, tt.domain, tt.workerThreads, tt.expRateLimiter, tt.setupLog)
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
	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	mockSecretController := &mockSecretController{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	originalgetSecretController := getSecretController
	originalOsExit := osExit

	after := func() {
		getSecretController = originalgetSecretController
		osExit = originalOsExit
	}

	tests := []struct {
		name          string
		controllerMgr *ControllerManager
		setupLog      logr.Logger
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
			setupLog: ctrl.Log.WithName("test-logger"),
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
			setupLog: ctrl.Log.WithName("test-logger"),
			setup: func() {
				getSecretController = func(_ *ControllerManager) error {
					return errors.New("failed to setup secret controller. Continuing")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			defer after()

			startSecretController(tt.controllerMgr, tt.setupLog)
		})
	}
}

func TestSetupControllerManager(t *testing.T) {
	mockMgr := &mockManager{
		logger: funcr.New(func(prefix, args string) { t.Logf("%s: %s", prefix, args) }, funcr.Options{}),
	}

	ctx := context.Background()
	originalOsExit := osExit

	after := func() {
		osExit = originalOsExit
	}

	tests := []struct {
		name        string
		mgr         manager.Manager
		setupLog    logr.Logger
		setup       func()
		expectedErr bool
	}{
		{
			name:        "Success",
			mgr:         mockMgr,
			setupLog:    ctrl.Log.WithName("test-logger"),
			setup:       func() {},
			expectedErr: false,
		},
		{
			name:        "Failure",
			mgr:         mockMgr,
			setupLog:    ctrl.Log.WithName("test-logger"),
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

			setupControllerManager(ctx, tt.mgr, tt.setupLog)

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

			getCtrlNewManager = func(opts ctrl.Options) (manager.Manager, error) {
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

	osExitCode := 0

	after := func() {
		// Restore the original function after the test
		createManagerInstance = defaultCreateManagerInstance
		osExit = defaultOSExit
		setupFlags = defaultSetupFlags
	}

	tests := []struct {
		name               string
		setup              func()
		expectedOsExitCode int
	}{
		{
			name: "Manager is nil",
			setup: func() {
				setupFlags = func() (map[string]string, logr.Logger, *logrus.Logger, context.Context) {
					flagMap := make(map[string]string)
					return flagMap, logr.Logger{}, logrus.New(), context.Background()
				}

				createManagerInstance = func(flagMap map[string]string) manager.Manager {
					return nil
				}

				osExit = func(code int) {
					osExitCode = code
				}
			},
			expectedOsExitCode: 0,
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
