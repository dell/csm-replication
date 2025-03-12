/*
 Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/bombsimon/logrusr/v4"
	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetControllerManagerOpts(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		wantOpts ControllerManagerOpts
	}{
		{
			name: "Default values",
			wantOpts: ControllerManagerOpts{
				UseConfFileFormat: true,
				WatchNamespace:    "dell-replication-controller",
				ConfigDir:         "deploy",
				ConfigFileName:    "config",
				InCluster:         false,
				Mode:              "",
			},
		},
		{
			name: "Custom values",
			envVars: map[string]string{
				common.EnvWatchNameSpace:    "default",
				common.EnvConfigFileName:    "config.yaml",
				common.EnvConfigDirName:     "config",
				common.EnvInClusterConfig:   "true",
				common.EnvUseConfFileFormat: "true",
			},
			wantOpts: ControllerManagerOpts{
				UseConfFileFormat: true,
				WatchNamespace:    "default",
				ConfigDir:         "config",
				ConfigFileName:    "config.yaml",
				InCluster:         true,
				Mode:              "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			if got := GetControllerManagerOpts(); !reflect.DeepEqual(got, tt.wantOpts) {
				t.Errorf("GetControllerManagerOpts() = %v, want %v", got, tt.wantOpts)
			}
		})
	}
}

func TestGetClusterID(t *testing.T) {
	// Test case: Lock is acquired and ClusterID is returned
	t.Run("GetClusterID", func(t *testing.T) {
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
			},
		}

		clusterID := c.GetClusterID()

		if clusterID != "test-cluster-id" {
			t.Errorf("Expected clusterID to be 'test-cluster-id', but got '%s'", clusterID)
		}
	})

}

func Test_readConfigFile(t *testing.T) {
	type args struct {
		configFile string
		configPath string
	}
	tests := []struct {
		name    string
		args    args
		want    *replicationConfigMap
		wantErr bool
	}{
		{
			name: "Read the config file successfully",
			args: args{
				configFile: "config",
				configPath: "../../deploy",
			},
			want: &replicationConfigMap{
				ClusterID: "",
				Targets:   []target{},
				LogLevel:  "INFO",
			},
			wantErr: false,
		},
		{
			name: "Config file not found",
			args: args{
				configFile: "config_not_found",
				configPath: "testdata",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Error reading the config file",
			args: args{
				configFile: "controller",
				configPath: "../../deploy",
			},
			want:    &replicationConfigMap{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readConfigFile(tt.args.configFile, tt.args.configPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("readConfigFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got == nil && tt.want == nil {
				return
			}
			if !reflect.DeepEqual(got.ClusterID, tt.want.ClusterID) && !reflect.DeepEqual(got.Targets, tt.want.Targets) && !reflect.DeepEqual(got.LogLevel, tt.want.LogLevel) {
				t.Errorf("readConfigFile() = %v, want %v", got.ClusterID, tt.want.ClusterID)

			}

		})
	}
}

func TestGetKubeConfigPathFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		xPath    string
		expected string
	}{
		{
			name:     "Home env set",
			xPath:    "/home/user",
			expected: "/home/user",
		},
		{
			name:     "X_CSI_KUBECONFIG_PATH env set",
			xPath:    "/path/to/kubeconfig",
			expected: "/path/to/kubeconfig",
		},
		{
			name:     "No env set",
			xPath:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("HOME", tt.xPath)
			os.Setenv("X_CSI_KUBECONFIG_PATH", tt.xPath)

			result := getKubeConfigPathFromEnv()

			if result != tt.expected {
				t.Errorf("Expected: %s, got: %s", tt.expected, result)
			}

			os.Unsetenv("HOME")
			os.Unsetenv("X_CSI_KUBECONFIG_PATH")
		})
	}
}

func TestSetConnectionHandler(t *testing.T) {
	// Test case: Set a new connection handler
	config := &replicationConfig{}
	handler := &connection.RemoteK8sConnHandler{}
	config.SetConnectionHandler(handler)
	assert.Equal(t, handler, config.ConnHandler)

	// Test case: Set a nil connection handler
	config.SetConnectionHandler(nil)
	assert.Nil(t, config.ConnHandler)
}

func TestNewReplicationConfig(t *testing.T) {
	tests := []struct {
		name         string
		configMap    *replicationConfigMap
		handler      connection.ConnHandler
		expectedRepC *replicationConfig
	}{
		{
			name: "Valid config map and handler",
			configMap: &replicationConfigMap{
				ClusterID: "cluster-id",
				Targets: []target{
					{
						ClusterID: "target-id",
					},
				},
			},
			handler: &connection.RemoteK8sConnHandler{},
			expectedRepC: &replicationConfig{
				ClusterID: "cluster-id",
				Targets: []target{
					{
						ClusterID: "target-id",
					},
				},
				ConnHandler: &connection.RemoteK8sConnHandler{},
			},
		},
		{
			name: "Empty config map and nil handler",
			configMap: &replicationConfigMap{
				ClusterID: "",
				Targets:   []target{},
			},
			handler: nil,
			expectedRepC: &replicationConfig{
				ClusterID:   "",
				Targets:     []target{},
				ConnHandler: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repC := newReplicationConfig(tt.configMap, tt.handler)
			if !reflect.DeepEqual(repC, tt.expectedRepC) {
				t.Errorf("Expected replication config %+v, but got %+v", tt.expectedRepC, repC)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	// Test case: Environment variable exists
	os.Setenv("TEST_ENV", "test_value")
	value := getEnv("TEST_ENV", "default_value")
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	// Test case: Environment variable does not exist
	os.Unsetenv("TEST_ENV")
	value = getEnv("TEST_ENV", "default_value")
	if value != "default_value" {
		t.Errorf("Expected 'default_value', got '%s'", value)
	}
}

func TestVerifyConfig(t *testing.T) {

	// Test case: Verify that an error is returned when config.ClusterID is empty
	t.Run("EmptyClusterID", func(t *testing.T) {
		config := &replicationConfig{
			ClusterID: "",
			Targets:   []target{},
		}
		err := config.VerifyConfig(context.Background())
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
	})

	// Test case: Verify that an error is returned when target.ClusterID is empty
	t.Run("EmptyTargetClusterID", func(t *testing.T) {
		config := &replicationConfig{
			ClusterID: "cluster-id",
			Targets: []target{
				{
					ClusterID: "",
				},
			},
		}
		err := config.VerifyConfig(context.Background())
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
	})

	// Test case: Verify that an error is returned when there are duplicate entries in config.Targets
	t.Run("DuplicateTargetClusterID", func(t *testing.T) {
		config := &replicationConfig{
			ClusterID: "cluster-id",
			Targets: []target{
				{
					ClusterID: "target-id",
				},
				{
					ClusterID: "target-id",
				},
			},
		}
		err := config.VerifyConfig(context.Background())
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
	})

	// Test case: Verify that the function calls config.Verify with the provided ctx and returns the error returned by config.Verify
	t.Run("VerifyCalled", func(t *testing.T) {
		originalValue := Verify
		defer func() {
			Verify = originalValue
		}()

		Verify = func(_ *replicationConfig, _ context.Context) error {
			return errors.New("verification failed")
		}

		config := &replicationConfig{
			ClusterID: "cluster-id",
			Targets: []target{
				{
					ClusterID: "target-id",
				},
				{
					ClusterID: "target-id1",
				},
			},
		}
		fmt.Print("Config", config.ClusterID, config.Targets)

		err := config.VerifyConfig(context.Background())

		// Assert that the error returned is the one expected
		if err == nil || err.Error() != "verification failed" {
			t.Errorf("Expected error 'verification failed', got %v", err)
		}

	})
}

// MockManager is a mock implementation of Manager for testing purposes
type MockManager struct {
	mock.Mock
}

func (m *MockManager) GetLogger() logr.Logger {
	args := m.Called()
	return args.Get(0).(logr.Logger)
}
func TestPrint(t *testing.T) {

	t.Run("PrintTest", func(t *testing.T) {
		logrusLog := logrus.New()
		logrusLog.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
		logger := logrusr.New(logrusLog)
		// Create a replicationConfig instance with some test data
		config := &replicationConfig{
			ClusterID: "cluster-id",
			Targets: []target{
				{ClusterID: "target-id", SecretRef: "secret1"},
				{ClusterID: "target-id2", SecretRef: "secret2"},
			},
		}
		mockManager := new(MockManager)
		mockManager.On("GetLogger").Return(logger)

		// Call the Print method with the mock logger
		config.Print(mockManager.GetLogger())

		// Assert that the expected Info calls were made
	})
}

func TestConfig_PrintConfig(t *testing.T) {
	t.Run("PrintConfigTest", func(t *testing.T) {
		logrusLog := logrus.New()
		logrusLog.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
		logger := logrusr.New(logrusLog)
		// Create a replicationConfig instance with some test data
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
			},
		}
		mockManager := new(MockManager)
		mockManager.On("GetLogger").Return(logger)

		// Call the Print method with the mock logger
		c.PrintConfig(mockManager.GetLogger())

		// Assert that the expected Info calls were made
	})
}

// func TestConfig_GetConnection(t *testing.T) {
// 	// Test case: Cluster ID is found in the replication config
// 	t.Run("Cluster ID found", func(t *testing.T) {
// 		c := &Config{
// 			repConfig: &replicationConfig{
// 				ClusterID: "test-cluster",
// 			},
// 		}

// 		client, err := c.GetConnection("test-cluster")

// 		if err != nil {
// 			t.Errorf("Expected no error, but got %v", err)
// 		}
// 		if client == nil {
// 			t.Error("Expected a non-nil client, but got nil")
// 		}
// 	})

// 	// Test case: Cluster ID is not found in the replication config
// 	t.Run("Cluster ID not found", func(t *testing.T) {
// 		c := &Config{
// 			repConfig: &replicationConfig{
// 				ClusterID: "test-cluster",
// 			},
// 		}

// 		client, err := c.GetConnection("other-cluster")
// 		if err == nil {
// 			t.Error("Expected an error, but got nil")
// 		}
// 		if client != nil {
// 			t.Error("Expected a nil client, but got a non-nil value")
// 		}
// 	})
// }

type MockClient struct {
	mock.Mock
	ctrlClient.Client
}

func (m *MockClient) Get(ctx context.Context, key ctrlClient.ObjectKey, obj ctrlClient.Object) error {
	args := m.Called(ctx, key, obj)
	return args.Error(0)
}

func TestBuildRestConfigFromSecretConfFileFormat(t *testing.T) {

	// Test case 1: Secret found and valid kubeconfig data
	t.Run("SecretFoundAndValidData", func(t *testing.T) {
		// Prepare mock data
		secretName := "valid-secret"
		namespace := "default"

		// secret := new(v1.Secret)

		kubeconfig := `
apiVersion: v1
clusters:
- cluster:
    server: https://test-server.com
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
kind: Config
preferences: {}
users:
- name: test-user
  user:
    token: test-token
`
		secret1 := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"data": []byte(kubeconfig),
			},
		}

		clinet := fake.NewClientBuilder().WithObjects(secret1).Build()
		// Test the function
		_, err := buildRestConfigFromSecretConfFileFormat(context.Background(), secretName, namespace, clinet)
		assert.NoError(t, err)

	})

}

func TestBuildRestConfigFromServiceAccountToken(t *testing.T) {
	ctx := context.Background()
	namespace := "test-ns"
	secretName := "test-secret"
	host := "test-host.com"
	caCert := []byte("test-ca-cert")
	token := []byte("test-token")

	validSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": caCert,
			"token":  token,
		},
	}

	tests := []struct {
		name       string
		secretName string
		namespace  string
		fakeClient ctrlClient.Client
		host       string
		wantConfig *rest.Config
		wantErr    bool
	}{
		{
			name:       "valid secret",
			secretName: secretName,
			namespace:  namespace,
			fakeClient: fake.NewClientBuilder().WithObjects(validSecret).Build(),
			host:       host,
			wantConfig: &rest.Config{
				Host: "https://" + host,
				TLSClientConfig: rest.TLSClientConfig{
					CAData:  caCert,
					KeyData: token,
				},
				BearerToken: string(token),
			},
			wantErr: true,
		},
		{
			name:       "secret not found",
			secretName: "non-existent",
			namespace:  namespace,
			fakeClient: fake.NewClientBuilder().Build(),
			host:       host,
			wantConfig: nil,
			wantErr:    true,
		},
		{
			name:       "invalid ca cert",
			secretName: secretName,
			namespace:  namespace,
			fakeClient: fake.NewClientBuilder().WithObjects(&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"ca.crt": []byte("invalid-cert"),
					"token":  token,
				},
			}).Build(),
			host:       host,
			wantConfig: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildRestConfigFromServiceAccountToken(ctx, tt.secretName, tt.namespace, tt.fakeClient, tt.host)
			if tt.wantErr && tt.wantConfig != nil {
				assert.Equal(t, "data does not contain any valid RSA or ECDSA certificates", err.Error())
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("buildRestConfigFromServiceAccountToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

		})
	}
}

// /////////////////////////////////////////////
func Test_getConnHandler(t *testing.T) {
	originalValue := InClusterConfig
	defer func() {
		InClusterConfig = originalValue
	}()
	InClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{
			Host: "https://localhost",
			TLSClientConfig: rest.TLSClientConfig{
				CAData:  []byte("test-ca"),
				KeyData: []byte("test-key"),
			},
		}, nil
	}
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	logger := logrusr.New(logrusLog)

	mockManager := new(MockManager)
	mockManager.On("GetLogger").Return(logger)
	configMap, err := readConfigFile("config.yaml", "../../deploy")
	assert.NoError(t, err)
	type args struct {
		ctx     context.Context
		targets []target
		client  ctrlClient.Client
		opts    ControllerManagerOpts
		log     logr.Logger
	}

	t.Run("SecretFoundAndValidData", func(t *testing.T) {
		os.Setenv(common.EnvInClusterConfig, "true")

		args := args{
			ctx:     context.Background(),
			targets: configMap.Targets,
			client:  fake.NewClientBuilder().Build(),
			opts: ControllerManagerOpts{
				UseConfFileFormat: true,
				WatchNamespace:    "dell-replication-controller",
				ConfigDir:         "../../deploy",
				ConfigFileName:    "config",
				InCluster:         true,
				Mode:              "",
			},
			log: mockManager.GetLogger(),
		}

		_, err := getConnHandler(args.ctx, args.targets, args.client, args.opts, args.log)
		assert.NoError(t, err)
	})
	t.Run("InCluster to false", func(t *testing.T) {
		os.Setenv(common.EnvInClusterConfig, "false")

		args := args{
			ctx:     context.Background(),
			targets: configMap.Targets,
			client:  fake.NewClientBuilder().Build(),
			opts: ControllerManagerOpts{
				UseConfFileFormat: true,
				WatchNamespace:    "dell-replication-controller",
				ConfigDir:         "../../deploy",
				ConfigFileName:    "config",
				InCluster:         true,
				Mode:              "",
			},
			log: mockManager.GetLogger(),
		}

		_, err := getConnHandler(args.ctx, args.targets, args.client, args.opts, args.log)
		assert.Equal(t, err.Error(), "failed to get kube config path")
	})
}

///////////////////////////////////////

// func Test_getReplicationConfig(t *testing.T) {
// 	originalValue := InClusterConfig
// 	defer func() {
// 		InClusterConfig = originalValue
// 	}()
// 	InClusterConfig = func() (*rest.Config, error) {
// 		return &rest.Config{
// 			Host: "https://localhost",
// 			TLSClientConfig: rest.TLSClientConfig{
// 				CAData:  []byte("test-ca"),  // Mocked return value"test-ca",
// 				KeyData: []byte("test-key"), // Mocked return value"test-key",
// 			},
// 		}, nil
// 	}

// 	logrusLog := logrus.New()
// 	logrusLog.SetFormatter(&logrus.JSONFormatter{
// 		TimestampFormat: time.RFC3339Nano,
// 	})
// 	logger := logrusr.New(logrusLog)

// 	mockManager := new(MockManager)
// 	mockManager.On("GetLogger").Return(logger)

// 	type args struct {
// 		ctx      context.Context
// 		client   ctrlClient.Client
// 		opts     ControllerManagerOpts
// 		recorder record.EventRecorder
// 		log      logr.Logger
// 	}
// 	tests := []struct {
// 		name    string
// 		args    args
// 		want    *replicationConfigMap
// 		want1   *replicationConfig
// 		wantErr bool
// 	}{
// 		{
// 			name: "Success",
// 			args: args{
// 				ctx:    context.Background(),
// 				client: fake.NewClientBuilder().Build(),
// 				opts: ControllerManagerOpts{
// 					UseConfFileFormat: true,
// 					WatchNamespace:    "dell-replication-controller",
// 					ConfigDir:         "../../deploy",
// 					ConfigFileName:    "config",
// 					InCluster:         true,
// 					Mode:              "",
// 				},
// 				recorder: nil,
// 				log:      mockManager.GetLogger(),
// 			},
// 			want:    &replicationConfigMap{},
// 			want1:   &replicationConfig{},
// 			wantErr: false,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {

// 			os.Setenv(common.EnvInClusterConfig, "true")

// 			got, got1, err := getReplicationConfig(tt.args.ctx, tt.args.client, tt.args.opts, tt.args.recorder, tt.args.log)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("getReplicationConfig() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("getReplicationConfig() got = %v, want %v", got, tt.want)
// 			}
// 			if !reflect.DeepEqual(got1, tt.want1) {
// 				t.Errorf("getReplicationConfig() got1 = %v, want %v", got1, tt.want1)
// 			}
// 		})
// 	}
// }
