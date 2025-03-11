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

	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/stretchr/testify/assert"
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
