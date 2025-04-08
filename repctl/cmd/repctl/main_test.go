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
	"bytes"
	"testing"
)

func TestMainFunc(t *testing.T) {
	repctl := setupRepctlCommand()

	// Test cases
	tests := []struct {
		name       string
		args       []string
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "Unknown command",
			args:       []string{"repctl", "unknown"},
			wantOutput: "Error: unknown command \"repctl\" for \"repctl\"\nRun 'repctl --help' for usage.\n",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up a test output buffer
			out := &bytes.Buffer{}

			// Set the arguments
			repctl.SetArgs(tt.args)

			// Set the output
			repctl.SetOutput(out)

			// Execute the command
			err := repctl.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("repctl.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check the output
			if out.String() != tt.wantOutput {
				t.Errorf("repctl.Execute() output = %v, want %v", out.String(), tt.wantOutput)
			}
		})
	}
}
