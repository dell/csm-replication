package main

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	repcnf "github.com/dell/csm-replication/pkg/config"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
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
