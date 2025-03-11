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
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"testing"

	controller "github.com/dell/csm-replication/controllers/csi-migrator"
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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcnf "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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

// Mock implementation of the client interface
type mockClient struct{}

func (m *mockClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return nil
}

func (m *mockClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return nil
}

func (m *mockClient) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return nil
}

// Mock implementation of the rest.Config interface
type mockRestConfig struct{}

func (m *mockRestConfig) Host() string {
	return ""
}

func (m *mockRestConfig) UserAgent() string {
	return ""
}

func (m *mockRestConfig) Wrap(rt http.RoundTripper) http.RoundTripper {
	return nil
}

func (m *mockRestConfig) WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return nil
}

func (m *mockRestConfig) Dial(network, addr string) (net.Conn, error) {
	return nil, nil
}

func (m *mockRestConfig) TLSClientConfig() *tls.Config {
	return nil
}

func (m *mockRestConfig) QPS() float32 {
	return 0
}

func (m *mockRestConfig) Burst() int {
	return 0
}

func (m *mockClient) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return nil
}

func (m *mockClient) DeleteAllOf(_ context.Context, _ client.Object, opts ...client.DeleteAllOfOption) error {
	// Implement the DeleteAllOf method here
	return nil
}

func (m *mockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	// Implement the GroupVersionKindFor method here
	return schema.GroupVersionKind{}, nil
}

func (m *mockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	// Implement the IsObjectNamespaced method here
	return false, nil
}

func (m *mockClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	// Implement the List method here
	return nil
}

func (m *mockClient) Patch(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
	// Implement the Patch method here
	return nil
}

func (m *mockClient) RESTMapper() meta.RESTMapper {
	// Implement the RESTMapper method here
	return nil
}

func (m *mockClient) Scheme() *runtime.Scheme {
	// Implement the Scheme method here
	return nil
}

func (m *mockClient) Status() client.StatusWriter {
	return nil
}

func (m *mockClient) SubResource(subResource string) client.SubResourceClient {
	return nil
}

func TestMain(t *testing.T) {
	defaultGetConnectToCsiFunc := getConnectToCsiFunc
	defaultGetProbeForeverFunc := getProbeForeverFunc
	defaultGetMigrationCapabilitiesFunc := getMigrationCapabilitiesFunc
	defaultGetcreateMigratorManagerFunc := getcreateMigratorManagerFunc
	defaultGetParseLevelFunc := getParseLevelFunc

	defer func() {
		// Restore the original function after the test
		getConnectToCsiFunc = defaultGetConnectToCsiFunc
		getProbeForeverFunc = defaultGetProbeForeverFunc
		getMigrationCapabilitiesFunc = defaultGetMigrationCapabilitiesFunc
		getcreateMigratorManagerFunc = defaultGetcreateMigratorManagerFunc
		getParseLevelFunc = defaultGetParseLevelFunc
	}()

	// Mock the getConnectToCsiFunc to return a successful connection
	getConnectToCsiFunc = func(_ string, _ logr.Logger) (*grpc.ClientConn, error) {
		// Return a mock *grpc.ClientConn
		return &grpc.ClientConn{}, nil
	}

	getProbeForeverFunc = func(_ context.Context, _ csiidentity.Identity) (string, error) {
		return "csi-driver", nil
	}

	getMigrationCapabilitiesFunc = func(_ context.Context, _ csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
		// Populate the MigrationCapabilitySet with mock data (map with MigrateTypes as keys and bool as values)
		capabilitySet := csiidentity.MigrationCapabilitySet{
			migration.MigrateTypes(migration.MigrateTypes_VERSION_UPGRADE):  true,
			migration.MigrateTypes(migration.MigrateTypes_REPL_TO_NON_REPL): true,
		}

		return capabilitySet, nil
	}

	getcreateMigratorManagerFunc = func(ctx context.Context, mgr manager.Manager, logrusLog *logrus.Logger) (*MigratorManager, error) {
		return &MigratorManager{}, nil
	}

	getParseLevelFunc = func(level string) (logrus.Level, error) {
		return logrus.Level(0), fmt.Errorf("unable to parse log level: %s", level)
	}

	main()

}
