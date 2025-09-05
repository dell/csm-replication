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
	"testing"

	"github.com/dell/repctl/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestSetupRepctlCommand(t *testing.T) {
	// Initialize viper with test values
	viper.Set(config.ReplicationPrefix, "test.prefix")

	// Call the function
	repctl := setupRepctlCommand()

	// Assert the command properties
	assert.Equal(t, "repctl", repctl.Use)
	assert.Equal(t, "repctl is CLI tool for managing replication in Kubernetes", repctl.Short)
	assert.Equal(t, "repctl is CLI tool for managing replication in Kubernetes", repctl.Long)
	assert.Equal(t, "v1.13.0", repctl.Version)

	// Assert the persistent flags
	clustersFlag := repctl.PersistentFlags().Lookup("clusters")
	assert.NotNil(t, clustersFlag)
	assert.Equal(t, "remote cluster id", clustersFlag.Usage)

	driverFlag := repctl.PersistentFlags().Lookup("driver")
	assert.NotNil(t, driverFlag)
	assert.Equal(t, "name of the driver", driverFlag.Usage)

	rgFlag := repctl.PersistentFlags().Lookup("rg")
	assert.NotNil(t, rgFlag)
	assert.Equal(t, "replication group name", rgFlag.Usage)

	prefixFlag := repctl.PersistentFlags().Lookup("prefix")
	assert.NotNil(t, prefixFlag)
	assert.Equal(t, "prefix for replication (default is replication.storage.dell.com)", prefixFlag.Usage)

	verboseFlag := repctl.PersistentFlags().Lookup("verbose")
	assert.NotNil(t, verboseFlag)
	assert.Equal(t, "enables verbosity and debug output", verboseFlag.Usage)

	// Assert the commands
	expectedCommands := []string{
		"cluster", "create", "failover", "failback", "reprotect",
		"swap", "exec", "edit", "migrate", "snapshot",
	}
	for _, cmdName := range expectedCommands {
		cmd := repctl.Commands()
		found := false
		for _, c := range cmd {
			if c.Use == cmdName {
				found = true
				break
			}
		}
		assert.True(t, found, "command %s not found", cmdName)
	}
}

func TestMain(t *testing.T) {
	originalGetRepctlCommandExecFunction := getRepctlCommandExecFunction
	defaultOSExit := osExit
	osExitCode := 0
	osExit = func(code int) {
		osExitCode = code
	}
	defer func() {
		getRepctlCommandExecFunction = originalGetRepctlCommandExecFunction
		osExit = defaultOSExit
	}()

	tests := []struct {
		name             string
		mockExecFunc     func(*cobra.Command) error
		expectedExitCode int
	}{
		{
			name: "successful execution",
			mockExecFunc: func(_ *cobra.Command) error {
				return nil
			},
			expectedExitCode: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the exit code
			osExitCode = 0

			// Mock the command execution function
			getRepctlCommandExecFunction = tt.mockExecFunc
			// Call the main function
			main()

			// Assert the exit code
			if osExitCode != tt.expectedExitCode {
				t.Errorf("Expected osExitCode: %v, but got osExitCode: %v", tt.expectedExitCode, osExitCode)
			}
		})
	}
}
