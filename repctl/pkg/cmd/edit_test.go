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

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"
)

type MockCluster struct {
	k8s.ClusterInterface
	repGroups map[string]*repv1.DellCSIReplicationGroup
}

func (m *MockCluster) GetReplicationGroups(ctx context.Context, rgID string) (*repv1.DellCSIReplicationGroup, error) {
	if rg, exists := m.repGroups[rgID]; exists {
		return rg, nil
	}
	return nil, fmt.Errorf("replication group not found")
}

type MockMultiClusterConfigurator struct {
	k8s.MultiClusterConfigurator
	clusters []k8s.ClusterInterface
}

func (m *MockMultiClusterConfigurator) GetAllClusters(args []string, configFolder string) (*k8s.Clusters, error) {
	return &k8s.Clusters{Clusters: m.clusters}, nil
}

func TestDecodedSecret_ToSecret(t *testing.T) {
	type fields struct {
		TypeMeta   metav1.TypeMeta
		ObjectMeta metav1.ObjectMeta
		Immutable  *bool
		Data       map[string]string
		StringData map[string]string
		Type       v1.SecretType
	}
	tests := []struct {
		name   string
		fields fields
		want   *v1.Secret
	}{
		{
			name: "Successful conversion",
			fields: fields{
				TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "my-namespace"},
				Immutable:  new(bool),
				Data:       map[string]string{"key1": "value1", "key2": "value2"},
				StringData: map[string]string{"key3": "value3", "key4": "value4"},
				Type:       v1.SecretTypeOpaque,
			},
			want: &v1.Secret{
				TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "my-namespace"},
				Immutable:  new(bool),
				Data:       map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
				StringData: map[string]string{"key3": "value3", "key4": "value4"},
				Type:       v1.SecretTypeOpaque,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DecodedSecret{
				TypeMeta:   tt.fields.TypeMeta,
				ObjectMeta: tt.fields.ObjectMeta,
				Immutable:  tt.fields.Immutable,
				Data:       tt.fields.Data,
				StringData: tt.fields.StringData,
				Type:       tt.fields.Type,
			}
			if got := s.ToSecret(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DecodedSecret.ToSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecret_ToDecodedSecret(t *testing.T) {
	type fields struct {
		Secret *v1.Secret
	}
	tests := []struct {
		name   string
		fields fields
		want   *DecodedSecret
	}{
		{
			name: "Successful conversion",
			fields: fields{
				Secret: &v1.Secret{
					TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
					ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "my-namespace"},
					Immutable:  new(bool),
					Data:       map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
					StringData: map[string]string{"key3": "value3", "key4": "value4"},
					Type:       v1.SecretTypeOpaque,
				},
			},
			want: &DecodedSecret{
				TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "my-namespace"},
				Immutable:  new(bool),
				Data:       map[string]string{"key1": "value1", "key2": "value2"},
				StringData: map[string]string{"key3": "value3", "key4": "value4"},
				Type:       v1.SecretTypeOpaque,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Secret{
				Secret: tt.fields.Secret,
			}
			if got := s.ToDecodedSecret(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Secret.ToDecodedSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSecret(t *testing.T) {
	// Save original functions
	originalReadFile := readFile
	originalUnmarshalYAML := unmarshalYAML

	// Restore original functions after test
	defer func() {
		readFile = originalReadFile
		unmarshalYAML = originalUnmarshalYAML
	}()

	tests := []struct {
		name        string
		setup       func()
		expectedErr bool
	}{
		{
			name: "Success - valid YAML",
			setup: func() {
				readFile = func(path string) ([]byte, error) {
					return []byte("key: value"), nil
				}
				unmarshalYAML = func(content []byte, v interface{}, opts ...yaml.JSONOpt) error {
					*v.(*DecodedSecret) = DecodedSecret{}
					return nil
				}
			},
			expectedErr: false,
		},
		{
			name: "Error - readFile fails",
			setup: func() {
				readFile = func(path string) ([]byte, error) {
					return nil, fmt.Errorf("read file error")
				}
			},
			expectedErr: true,
		},
		{
			name: "Error - unmarshalYAML fails",
			setup: func() {
				readFile = func(path string) ([]byte, error) {
					return []byte("key: value"), nil
				}
				unmarshalYAML = func(content []byte, v interface{}, opts ...yaml.JSONOpt) error {
					return fmt.Errorf("unmarshal error")
				}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			// Call the function
			secret, err := parseSecret("test.yaml")

			// Assertions
			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, secret)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, secret)
			}
		})
	}
}

func TestObjectYAML(t *testing.T) {
	// Save original functions
	originalJSONMarshal := jsonMarshal
	originalJSONToYAML := jsonToYAML

	// Restore original functions after test
	defer func() {
		jsonMarshal = originalJSONMarshal
		jsonToYAML = originalJSONToYAML
	}()

	tests := []struct {
		name     string
		setup    func()
		input    interface{}
		expected string
	}{
		{
			name: "Success - valid object",
			setup: func() {
				jsonMarshal = func(v interface{}) ([]byte, error) {
					return []byte(`{"key":"value"}`), nil
				}
				jsonToYAML = func(j []byte) ([]byte, error) {
					return []byte("key: value\n"), nil
				}
			},
			input:    map[string]string{"key": "value"},
			expected: "key: value\n",
		},
		{
			name: "Error - jsonMarshal fails",
			setup: func() {
				jsonMarshal = func(v interface{}) ([]byte, error) {
					return nil, fmt.Errorf("json marshal error")
				}
			},
			input:    map[string]string{"key": "value"},
			expected: "json marshal error",
		},
		{
			name: "Error - jsonToYAML fails",
			setup: func() {
				jsonMarshal = func(v interface{}) ([]byte, error) {
					return []byte(`{"key":"value"}`), nil
				}
				jsonToYAML = func(j []byte) ([]byte, error) {
					return nil, fmt.Errorf("yaml conversion error")
				}
			},
			input:    map[string]string{"key": "value"},
			expected: "yaml conversion error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			// Call the function
			result := objectYAML(tt.input)

			// Assertions
			assert.Equal(t, tt.expected, result)
		})
	}
}

type EditTestSuite struct {
	suite.Suite
	testDataFolder string
}

func (suite *EditTestSuite) SetupSuite() {
	curUser, err := os.UserHomeDir()
	suite.NoError(err)

	curUser = filepath.Join(curUser, folderPath)
	curUserPath, err := filepath.Abs(curUser)
	suite.NoError(err)

	suite.testDataFolder = curUserPath
	_ = repv1.AddToScheme(scheme.Scheme)

	metadata.Init("replication.storage.dell.com")
}

func TestEditTestSuite(t *testing.T) {
	suite.Run(t, new(EditTestSuite))
}
func (suite *EditTestSuite) TestGetEditCommand() {
	cmd := GetEditCommand()

	// Test the command usage
	suite.Equal("edit", cmd.Use)
	suite.Equal("edit different resources in clusters", cmd.Short)

	// Test the subcommands
	subCommands := cmd.Commands()
	suite.NotEmpty(subCommands)
	for _, subCmd := range subCommands {
		suite.NotNil(subCmd)
	}

	cmd.Run(nil, []string{"test"})
}

func (suite *EditTestSuite) TestEditSecretCommand() {
	tests := []struct {
		name                      string
		getClustersFolderPath     func(string) (string, error)
		getMultiConfigClusters    func(mc *k8s.MultiClusterConfigurator, clusterIDs []string, configDir string) (*k8s.Clusters, error)
		getSecretFunction         func(cluster k8s.ClusterInterface, ctx context.Context, secretNamespace string, secretName string) (*v1.Secret, error)
		getUpdateSecretFunction   func(cluster k8s.ClusterInterface, ctx context.Context, secret *v1.Secret) error
		secretName                string
		secretNamespace           string
		expectedOutputContains    []string
		expectedOutputNotContains []string
	}{
		{
			name: "Successful edit secret",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			getMultiConfigClusters: func(mc *k8s.MultiClusterConfigurator, clusterIDs []string, configDir string) (*k8s.Clusters, error) {
				return &k8s.Clusters{
					Clusters: []k8s.ClusterInterface{
						&k8s.Cluster{
							ClusterID: "",
						},
					},
				}, nil
			},
			getSecretFunction: func(cluster k8s.ClusterInterface, ctx context.Context, secretNamespace string, secretName string) (*v1.Secret, error) {
				return &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: secretNamespace,
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				}, nil
			},
			getUpdateSecretFunction: func(cluster k8s.ClusterInterface, ctx context.Context, secret *v1.Secret) error {
				return nil
			},
			secretName:                "test-secret",
			secretNamespace:           "default",
			expectedOutputContains:    []string{""},
			expectedOutputNotContains: []string{"error"},
		},
	}

	for _, tt := range tests {
		suite.Suite.T().Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			originalGetMultiConfigClusters := getMultiConfigClusters
			defer func() {
				getMultiConfigClusters = originalGetMultiConfigClusters
			}()

			getMultiConfigClusters = tt.getMultiConfigClusters

			originalGetSecretFunction := getSecretFunction
			defer func() {
				getSecretFunction = originalGetSecretFunction
			}()

			getSecretFunction = tt.getSecretFunction

			originalGetUpdateSecretFunction := getUpdateSecretFunction
			defer func() {
				getUpdateSecretFunction = originalGetUpdateSecretFunction
			}()

			getUpdateSecretFunction = tt.getUpdateSecretFunction

			viper.Set("namespace", tt.secretNamespace)
			defer viper.Reset()

			cmd := editSecretCommand()

			// Mock the editor interaction
			originalEditor := os.Getenv("EDITOR")
			defer os.Setenv("EDITOR", originalEditor)
			os.Setenv("EDITOR", "true") // Mock editor command

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			cmd.Run(nil, []string{tt.secretName})

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout
			for _, expected := range tt.expectedOutputContains {
				assert.Contains(t, string(out), expected)
			}

			for _, notExpected := range tt.expectedOutputNotContains {
				assert.NotContains(t, string(out), notExpected)
			}
		})
	}
}
