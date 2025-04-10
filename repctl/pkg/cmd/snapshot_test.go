/*
 Copyright © 2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockConfig struct {
	mock.Mock
}

func (m *MockConfig) GetString(key string) string {
	args := m.Called(key)
	return args.String(0)
}

func (m *MockConfig) GetBool(key string) bool {
	args := m.Called(key)
	return args.Bool(0)
}

func (m *MockConfig) BindPFlag(key string, flag *pflag.Flag) error {
	args := m.Called(key, flag)
	return args.Error(0)
}

func TestGetSnapshotCommand(t *testing.T) {

	clusterPath = "/.repctl/testdata/clusters"

	originalGetClustersFolderPathFunction := getClustersFolderPathFunction
	defer func() {
		getClustersFolderPathFunction = originalGetClustersFolderPathFunction
	}()

	getClustersFolderPathFunction = func(path string) (string, error) {
		return clusterPath, nil
	}

	// Initialize viper and mock configuration
	mockConfig := new(MockConfig)
	viper.Set("target", "test-cluster")
	viper.Set("sn-namespace", "test-namespace")
	viper.Set("sn-class", "test-class")
	viper.Set("snapshot-wait", true)
	mockConfig.On("GetString", "target").Return("test-cluster")
	mockConfig.On("GetString", "sn-namespace").Return("test-namespace")
	mockConfig.On("GetString", "sn-class").Return("test-class")
	mockConfig.On("GetBool", "snapshot-wait").Return(true)

	// Prepare the snapshot command
	cmd := GetSnapshotCommand()

	// Setup input flags for the command
	cmd.Flags().Set("at", "test-cluster")
	cmd.Flags().Set("sn-namespace", "test-namespace")
	cmd.Flags().Set("sn-class", "test-class")
	cmd.Flags().Set("wait", "true")

	// Execute the command
	err := cmd.Execute()
	assert.Nil(t, err)

	// Test the behavior of your function
	mockConfig.AssertExpectations(t)
}
