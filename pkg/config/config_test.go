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
	"bytes"
	"context"
	"errors"
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
	"k8s.io/client-go/tools/record"
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
		{
			name: "Custom values EnvUseConfFileFormat false",
			envVars: map[string]string{
				common.EnvWatchNameSpace:    "default",
				common.EnvConfigFileName:    "config.yaml",
				common.EnvConfigDirName:     "config",
				common.EnvInClusterConfig:   "true",
				common.EnvUseConfFileFormat: "false",
			},
			wantOpts: ControllerManagerOpts{
				UseConfFileFormat: false,
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
	t.Run("PrintTest", func(_ *testing.T) {
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
	t.Run("PrintConfigTest", func(_ *testing.T) {
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
	})
}

func TestGetConnection(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		originalValue := GetConnection
		defer func() {
			GetConnection = originalValue
		}()

		GetConnection = func(_ *Config, _ string) (connection.RemoteClusterClient, error) {
			return nil, errors.New("verification failed")
		}

		config := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster",
			},
		}
		_, err := config.GetConnection("test-cluster")
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
	})
}

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
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"data": []byte(kubeconfig),
			},
		}

		clinet := fake.NewClientBuilder().WithObjects(secret).Build()
		// Test the function
		got, err := buildRestConfigFromSecretConfFileFormat(context.Background(), secretName, namespace, clinet)
		assert.Equal(t, "https://test-server.com", got.Host)
		assert.NoError(t, err)
	})
}

func TestBuildRestConfigFromServiceAccountToken(t *testing.T) {
	ctx := context.Background()
	namespace := "test-ns"
	secretName := "test-secret"
	host := "test-host.com"
	invalidCert := []byte("test-ca-cert")
	token := []byte("test-token")

	validSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": invalidCert,
			"token":  token,
		},
	}

	validCaCert := []byte(`-----BEGIN CERTIFICATE-----
MIIDBTCCAe2gAwIBAgIIUvG0tSrxeiUwDQYJKoZIhvcNAQELBQAwFTETMBEGA1UE
AxMKa3ViZXJuZXRlczAeFw0yNTAyMDQxMTA4MzRaFw0zNTAyMDIxMTEzMzRaMBUx
EzARBgNVBAMTCmt1YmVybmV0ZXMwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
AoIBAQDHqyWictrHZ46mqDsqRfUAX4MdLINMv0NYr1NfFES7QderULRTttzaQGF3
oXjVEU6lmP8hGYRbD9ItKehq3sFhiobOPH9kTQ4dKYRB76gAOfjxNCBfBhaTZG0S
hkz8916u11eghH2dgXfa/s1TOcbxJXPJt8nXNbTFTEaKjB+DWRhBX/WfjN+faOVX
b5HWJmDVIwlq7bxgrLk5WPkiQrsQshoij9YmgpuN5I0Y9xNymsWY1QHfM2wFeFEJ
rDGlj6zuceyRkVboifD9R2A5eyS++xgRKIo/p6e8L/OLA+FfrUcdIksQ7sQ2eNl/
Etc8ThOrgw2Yi5p5BgkrKUbMEZXnAgMBAAGjWTBXMA4GA1UdDwEB/wQEAwICpDAP
BgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBSejZWfFjRx4XI2uwCiHt5LYS6G5DAV
BgNVHREEDjAMggprdWJlcm5ldGVzMA0GCSqGSIb3DQEBCwUAA4IBAQDDmIuMHSR4
MB9MsiLae2NN9rOdUjY8hKJ61VtMJJeP285JQhXJZcnzCEsRfnwJ1tellBSOOIwE
H1E9vl0vKuKOgQxTjcxEHPEE3dKhbVB4/+UzNRfb8goCFerEWqsFbUAPmXKdWLhG
s2Y3hDfHf5rcUWQmY4GgxIMBzaLMUUvscoLmSHw9tsxTlNb90XFjiGHc3r0Uml6S
gxAiT89/TrtsQLUFzX3F1yVsBw/LatdpcIg/oeaQUa8J2CyLDuddA96XJdx/m00W
+iWkJlfBAs19qqaRj9E0jY//z/acMP2wq6Ed2vfDfSFHmYldpfLytmbs35HkcOkp
ggcetQ4yvATR
-----END CERTIFICATE-----`)

	validSecret1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": validCaCert,
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
			name:       "valid secret with invalid ca cert",
			secretName: secretName,
			namespace:  namespace,
			fakeClient: fake.NewClientBuilder().WithObjects(validSecret).Build(),
			host:       host,
			wantConfig: &rest.Config{
				Host: "https://" + host,
				TLSClientConfig: rest.TLSClientConfig{
					CAData:  invalidCert,
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
		{
			name:       "valid secret",
			secretName: secretName,
			namespace:  namespace,
			fakeClient: fake.NewClientBuilder().WithObjects(validSecret1).Build(),
			host:       host,
			wantConfig: &rest.Config{
				Host: "https://" + host,
				TLSClientConfig: rest.TLSClientConfig{
					CAData:  validCaCert,
					KeyData: token,
				},
				BearerToken: string(token),
			},
			wantErr: false,
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

func Test_getConnHandler(t *testing.T) {
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

	t.Run("SecretFoundAndValidDataWithTargets", func(t *testing.T) {
		os.Setenv(common.EnvInClusterConfig, "true")
		secretName := "valid-secret"
		namespace := "default"

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
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"data": []byte(kubeconfig),
			},
		}

		clinet1 := fake.NewClientBuilder().WithObjects(secret).Build()
		args := args{
			ctx: context.Background(),
			targets: []target{
				{
					ClusterID: "test-cluster", SecretRef: "valid-secret",
				},
			},
			client: clinet1,
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
		assert.Error(t, err)
	})
	t.Run("SecretFoundAndValidDataWithTargetsUseConfFileFormatToFalse", func(t *testing.T) {
		os.Setenv(common.EnvInClusterConfig, "true")

		args := args{
			ctx: context.Background(),
			targets: []target{
				{
					ClusterID: "target-id",
				},
			},
			client: fake.NewClientBuilder().Build(),
			opts: ControllerManagerOpts{
				UseConfFileFormat: false,
				WatchNamespace:    "dell-replication-controller",
				ConfigDir:         "../../deploy",
				ConfigFileName:    "config",
				InCluster:         true,
				Mode:              "",
			},
			log: mockManager.GetLogger(),
		}

		_, err := getConnHandler(args.ctx, args.targets, args.client, args.opts, args.log)
		assert.Error(t, err)
	})
	t.Run("SecretFoundAndValidData", func(t *testing.T) {
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
	t.Run("InCluster to false with kubeconfig path", func(t *testing.T) {
		os.Setenv(common.EnvInClusterConfig, "false")
		os.Setenv("X_CSI_KUBECONFIG_PATH", "/path/to/kubeconfig")

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
		assert.Error(t, err)
	})

	t.Run("InCluster to true", func(t *testing.T) {
		os.Setenv(common.EnvInClusterConfig, "true")
		originalValue2 := InClusterConfig
		defer func() {
			InClusterConfig = originalValue2
		}()
		InClusterConfig = func() (*rest.Config, error) {
			return nil, errors.New("verification failed")
		}

		args := args{
			ctx:     context.Background(),
			targets: []target{},
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
		assert.Error(t, err)
	})
}

func Test_buildRestConfigFromCustomFormat(t *testing.T) {
	// Test case: Secret not found
	t.Run("SecretNotFound", func(t *testing.T) {
		secretName := "invalid-secret"
		namespace := "default"
		client := fake.NewClientBuilder().Build()

		_, err := buildRestConfigFromCustomFormat(context.Background(), secretName, namespace, client)

		if err == nil {
			t.Error("Expected an error, but got nil")
		}
	})

	// Test case: Secret found and valid data
	t.Run("SecretFoundAndValidData", func(t *testing.T) {
		secretName := "valid-secret"
		namespace := "default"
		secretData := map[string][]byte{
			"server":     []byte("https://example.com"),
			"ca":         []byte("test-ca"),
			"clientkey":  []byte("test-clientkey"),
			"clientcert": []byte("test-clientcert"),
		}
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: secretData,
		}
		client := fake.NewClientBuilder().WithObjects(secret).Build()

		config, err := buildRestConfigFromCustomFormat(context.Background(), secretName, namespace, client)
		if err != nil {
			t.Errorf("Expected no error, but got %v", err)
		}
		if config.Host != "https://example.com" {
			t.Errorf("Expected host to be 'https://example.com', but got %s", config.Host)
		}
		if !bytes.Equal(config.TLSClientConfig.CAData, secretData["ca"]) {
			t.Errorf("Expected CA data to be %s, but got %s", secretData["ca"], config.TLSClientConfig.CAData)
		}
		if !bytes.Equal(config.TLSClientConfig.KeyData, secretData["clientkey"]) {
			t.Errorf("Expected client key data to be %s, but got %s", secretData["clientkey"], config.TLSClientConfig.KeyData)
		}
		if !bytes.Equal(config.TLSClientConfig.CertData, secretData["clientcert"]) {
			t.Errorf("Expected client cert data to be %s, but got %s", secretData["clientcert"], config.TLSClientConfig.CertData)
		}
	})
}

func Test_getReplicationConfig(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	logger := logrusr.New(logrusLog)

	mockManager := new(MockManager)
	mockManager.On("GetLogger").Return(logger)

	log := mockManager.GetLogger()
	t.Run("SuccessModeEmpty", func(t *testing.T) {
		ctx := context.Background()
		ConfgMap := &replicationConfigMap{
			ClusterID: "",
			Targets:   []target{},
			LogLevel:  "INFO",
		}
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "controller",
		}

		got, got1, err := getReplicationConfig(ctx, nil, opts, nil, log)
		if (got1 == nil) && (err != nil) {
			t.Errorf("getReplicationConfig() error = %v and wantErr is ni, got1 = %v and wamt1= ", err, nil)
			return
		}
		if !reflect.DeepEqual(got.ClusterID, ConfgMap.ClusterID) && !reflect.DeepEqual(got.LogLevel, ConfgMap.LogLevel) {
			t.Errorf("getReplicationConfig() got = %v, want %v", got, ConfgMap)
		}
	})
	t.Run("ErrorWithZeroTargets", func(t *testing.T) {
		ctx := context.Background()
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "",
		}

		// os.Setenv(common.EnvInClusterConfig, "true")
		client := fake.NewClientBuilder().Build()
		got, got1, err := getReplicationConfig(ctx, client, opts, nil, log)
		if (got != nil) && (got1 != nil) && (err == nil) {
			t.Errorf("getReplicationConfig() error = %v and wantErr is nil, got1 = %v and wamt1= ", err, nil)
			return
		}
	})
	t.Run("SuccessWithTargetsWithNoVerifyErr", func(t *testing.T) {
		originalValue1 := InClusterConfig
		defer func() {
			InClusterConfig = originalValue1
		}()
		InClusterConfig = func() (*rest.Config, error) {
			return &rest.Config{
				Host: "https://localhost",
				TLSClientConfig: rest.TLSClientConfig{
					CAData:  []byte("test-ca"),  // Mocked return value"test-ca",
					KeyData: []byte("test-key"), // Mocked return value"test-key",
				},
			}, nil
		}
		originalValue := Verify
		defer func() {
			Verify = originalValue
		}()

		Verify = func(_ *replicationConfig, _ context.Context) error {
			return errors.New("verification failed")
		}
		os.Setenv(common.EnvInClusterConfig, "true")
		secretName := "valid-secret"
		namespace := "default"

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
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"data": []byte(kubeconfig),
			},
		}

		client := fake.NewClientBuilder().WithObjects(secret).Build()
		ctx := context.Background()
		// ConfgMap := &replicationConfigMap{
		// 	ClusterID: "test-cluster-id",
		// 	Targets: []target{
		// 		{ClusterID: "target-id", SecretRef: "secret1"},
		// 		{ClusterID: "target-id2", SecretRef: "secret2"},
		// 	},
		// }

		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "controller",
		}
		fakeRecorder := record.NewFakeRecorder(100)
		os.Setenv(common.EnvInClusterConfig, "true")
		got, got1, err := getReplicationConfig(ctx, client, opts, fakeRecorder, log)
		assert.Error(t, err)
		assert.Nil(t, got)
		assert.Nil(t, got)
		if (got != nil) && (got1 != nil) && (err != nil) {
			t.Errorf("getReplicationConfig() error = %v and wantErr is ni, got1 = %v and wamt1= ", err, nil)
			return
		}
	})
	t.Run("SuccessWithTargetsWithElseCase", func(t *testing.T) {
		originalValue1 := InClusterConfig
		defer func() {
			InClusterConfig = originalValue1
		}()
		InClusterConfig = func() (*rest.Config, error) {
			return &rest.Config{
				Host: "https://localhost",
				TLSClientConfig: rest.TLSClientConfig{
					CAData:  []byte("test-ca"),  // Mocked return value"test-ca",
					KeyData: []byte("test-key"), // Mocked return value"test-key",
				},
			}, nil
		}

		configFilePath := "../../deploy/config.yaml"

		// 1. Read the original content
		originalContent, err := os.ReadFile(configFilePath)
		if err != nil {
			t.Fatalf("Failed to read original config file: %v", err)
		}

		// 2. Write the new content
		newContent := []byte(`
clusterId: "my-cluster"
targets:
CSI_LOG_LEVEL: "INFO"`)
		err = os.WriteFile(configFilePath, newContent, 0o600) // 0600: read/write for owner
		if err != nil {
			t.Fatalf("Failed to write new config content: %v", err)
		}

		// 3. Defer the restore operation
		defer func() {
			err := os.WriteFile(configFilePath, originalContent, 0o600)
			if err != nil {
				t.Errorf("Failed to restore original config content: %v", err)
			}
		}()
		originalValue := Verify
		defer func() {
			Verify = originalValue
		}()

		Verify = func(_ *replicationConfig, _ context.Context) error {
			return nil
		}
		os.Setenv(common.EnvInClusterConfig, "true")
		isInInvalidState = true
		secretName := "valid-secret"
		namespace := "default"

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
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"data": []byte(kubeconfig),
			},
		}

		client := fake.NewClientBuilder().WithObjects(secret).Build()
		ctx := context.Background()

		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "controller",
		}
		fakeRecorder := record.NewFakeRecorder(100)
		os.Setenv(common.EnvInClusterConfig, "true")
		got, got1, err := getReplicationConfig(ctx, client, opts, fakeRecorder, log)
		assert.Error(t, err)
		assert.Nil(t, got)
		assert.Nil(t, got)
		if (got != nil) && (got1 != nil) && (err != nil) {
			t.Errorf("getReplicationConfig() error = %v and wantErr is ni, got1 = %v and wamt1= ", err, nil)
			return
		}
	})

	t.Run("SuccessWithTargetsNoControllerWithNoVerifyErr", func(t *testing.T) {
		originalValue1 := InClusterConfig
		defer func() {
			InClusterConfig = originalValue1
		}()
		InClusterConfig = func() (*rest.Config, error) {
			return &rest.Config{
				Host: "https://localhost",
				TLSClientConfig: rest.TLSClientConfig{
					CAData:  []byte("test-ca"),  // Mocked return value"test-ca",
					KeyData: []byte("test-key"), // Mocked return value"test-key",
				},
			}, nil
		}
		originalValue := Verify
		defer func() {
			Verify = originalValue
		}()

		Verify = func(_ *replicationConfig, _ context.Context) error {
			return errors.New("verification failed")
		}
		os.Setenv(common.EnvInClusterConfig, "true")
		secretName := "valid-secret"
		namespace := "default"

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
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"data": []byte(kubeconfig),
			},
		}

		client := fake.NewClientBuilder().WithObjects(secret).Build()
		ctx := context.Background()

		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "",
		}
		fakeRecorder := record.NewFakeRecorder(100)
		os.Setenv(common.EnvInClusterConfig, "true")
		_, _, err := getReplicationConfig(ctx, client, opts, fakeRecorder, log)
		assert.NoError(t, err)
	})
}

func TestConfig_updateConfig(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	logger := logrusr.New(logrusLog)

	mockManager := new(MockManager)
	mockManager.On("GetLogger").Return(logger)

	// client1 := fake.NewClientBuilder().Build()
	log := mockManager.GetLogger()

	t.Run("Success", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "",
		}
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
				Targets: []target{
					{ClusterID: "target-id", SecretRef: "secret1"},
					{ClusterID: "target-id2", SecretRef: "secret2"},
				},
			},
		}
		ctx := context.Background()

		err := c.updateConfig(ctx, nil, opts, nil, log)
		if err != nil {
			t.Errorf("Config.updateConfig() error = %v, wantErr nil", err)
		}
	})
	t.Run("ErrorCallingGetReplicationConfig", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "default",
			ConfigDir:         "testdata",
			ConfigFileName:    "config_not_found",
			InCluster:         true,
			Mode:              "",
		}
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
				Targets: []target{
					{ClusterID: "target-id", SecretRef: "secret1"},
					{ClusterID: "target-id2", SecretRef: "secret2"},
				},
			},
		}
		ctx := context.Background()

		err := c.updateConfig(ctx, nil, opts, nil, log)

		if err == nil {
			t.Errorf("Config.updateConfig() error = %v, wantErr nil", err)
		}
	})
}

func TestConfig_UpdateConfigMap(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	logger := logrusr.New(logrusLog)

	mockManager := new(MockManager)
	mockManager.On("GetLogger").Return(logger)

	log := mockManager.GetLogger()

	t.Run("Success", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "",
		}
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
				Targets: []target{
					{ClusterID: "target-id", SecretRef: "secret1"},
					{ClusterID: "target-id2", SecretRef: "secret2"},
				},
			},
		}
		ctx := context.Background()

		err := c.UpdateConfigMap(ctx, nil, opts, nil, log)
		if err != nil {
			t.Errorf("Config.UpdateConfigMap() error = %v, wantErr nil", err)
		}
	})

	t.Run("ErrorCallingUpdateConfig", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "default",
			ConfigDir:         "testdata",
			ConfigFileName:    "config_not_found",
			InCluster:         true,
			Mode:              "",
		}
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
				Targets: []target{
					{ClusterID: "target-id", SecretRef: "secret1"},
					{ClusterID: "target-id2", SecretRef: "secret2"},
				},
			},
		}
		ctx := context.Background()

		err := c.UpdateConfigMap(ctx, nil, opts, nil, log)

		if err == nil {
			t.Errorf("Config.UpdateConfigMap() error = %v, wantErr non-nil", err)
		}
	})
}

func TestConfig_UpdateConfigOnSecretEvent(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	logger := logrusr.New(logrusLog)

	mockManager := new(MockManager)
	mockManager.On("GetLogger").Return(logger)

	log := mockManager.GetLogger()

	t.Run("RelevantSecret_UpdatesConfig", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "",
		}
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
				Targets: []target{
					{ClusterID: "target-id", SecretRef: "secret1"},
					{ClusterID: "target-id2", SecretRef: "secret2"},
				},
			},
		}
		ctx := context.Background()
		secretName := "secret1"

		err := c.UpdateConfigOnSecretEvent(ctx, nil, opts, secretName, nil, log)
		if err != nil {
			t.Errorf("Config.UpdateConfigOnSecretEvent() error = %v, wantErr nil", err)
		}
	})

	t.Run("IrrelevantSecret_IgnoresEvent", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "",
		}
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
				Targets: []target{
					{ClusterID: "target-id", SecretRef: "secret1"},
					{ClusterID: "target-id2", SecretRef: "secret2"},
				},
			},
		}
		ctx := context.Background()
		secretName := "secret3" // Secret not in targets

		err := c.UpdateConfigOnSecretEvent(ctx, nil, opts, secretName, nil, log)
		if err != nil {
			t.Errorf("Config.UpdateConfigOnSecretEvent() error = %v, wantErr nil", err)
		}
	})

	t.Run("ErrorInUpdateConfig_ReturnsError", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "default",
			ConfigDir:         "testdata",
			ConfigFileName:    "config_not_found",
			InCluster:         true,
			Mode:              "",
		}
		c := &Config{
			repConfig: &replicationConfig{
				ClusterID: "test-cluster-id",
				Targets: []target{
					{ClusterID: "target-id", SecretRef: "secret1"},
					{ClusterID: "target-id2", SecretRef: "secret2"},
				},
			},
		}
		ctx := context.Background()
		secretName := "secret1"

		err := c.UpdateConfigOnSecretEvent(ctx, nil, opts, secretName, nil, log)

		if err == nil {
			t.Errorf("Config.UpdateConfigOnSecretEvent() error = %v, wantErr non-nil", err)
		}
	})
}

func TestConfig_GetConfig(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	logger := logrusr.New(logrusLog)

	mockManager := new(MockManager)
	mockManager.On("GetLogger").Return(logger)

	log := mockManager.GetLogger()

	t.Run("Success", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "dell-replication-controller",
			ConfigDir:         "../../deploy",
			ConfigFileName:    "config",
			InCluster:         true,
			Mode:              "",
		}

		ctx := context.Background()

		got, err := GetConfig(ctx, nil, opts, nil, log)
		if err != nil {
			t.Errorf("Config.updateConfig() error = %v, wantErr nil", err)
		}
		assert.Equal(t, got.LogLevel, "INFO")
	})
	t.Run("ErrorCallingGetReplicationConfig", func(t *testing.T) {
		opts := ControllerManagerOpts{
			UseConfFileFormat: true,
			WatchNamespace:    "default",
			ConfigDir:         "testdata",
			ConfigFileName:    "config_not_found",
			InCluster:         true,
			Mode:              "",
		}

		ctx := context.Background()

		_, err := GetConfig(ctx, nil, opts, nil, log)

		if err == nil {
			t.Errorf("Config.updateConfig() error = %v, wantErr nil", err)
		}
	})
}
