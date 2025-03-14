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

func (m *mockSecretController) Start(ctx context.Context) error {
	return nil
}
func (m *mockSecretController) Watch(src source.TypedSource[reconcile.Request]) error {
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
