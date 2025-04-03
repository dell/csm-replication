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

package utils

import (
	"testing"
)

func TestIsStringInSlice(t *testing.T) {
	// Test case: Empty slice
	slice := []string{}
	str := "test"
	if IsStringInSlice(str, slice) {
		t.Errorf("IsStringInSlice(%s, %v) = true; want false", str, slice)
	}

	// Test case: Slice containing the target string
	slice = []string{"test"}
	if !IsStringInSlice(str, slice) {
		t.Errorf("IsStringInSlice(%s, %v) = false; want true", str, slice)
	}
}
