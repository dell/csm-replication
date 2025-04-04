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
	"fmt"
	"testing"

	"github.com/dell/repctl/pkg/config"
)

func TestGetSupportedMaintenanceAction(t *testing.T) {
	tests := []struct {
		action   string
		expected string
		err      error
	}{
		{"resume", config.ActionResume, nil},
		{"suspend", config.ActionSuspend, nil},
		{"sync", config.ActionSync, nil},
		{"invalid", "", fmt.Errorf("not a supported action")},
	}

	for _, test := range tests {
		result, err := getSupportedMaintenanceAction(test.action)
		if result != test.expected || (err != nil && err.Error() != test.err.Error()) {
			t.Errorf("For action %s, expected %s and error %v, but got %s and error %v", test.action, test.expected, test.err, result, err)
		}
	}
}
