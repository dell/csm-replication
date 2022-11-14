/*
 Copyright © 2021-2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package connection

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var debugUnitTest = false

func TestPending(t *testing.T) {
	tests := []struct {
		npending     int
		maxpending   int
		differentIDs bool
		errormsg     string
	}{
		{npending: 2,
			maxpending:   1,
			differentIDs: true,
			errormsg:     "overload",
		},
		{npending: 4,
			maxpending:   5,
			differentIDs: true,
			errormsg:     "none",
		},
		{npending: 2,
			maxpending:   5,
			differentIDs: false,
			errormsg:     "pending",
		},
		{npending: 0,
			maxpending:   1,
			differentIDs: false,
			errormsg:     "none",
		},
	}
	for _, test := range tests {
		pendState := &PendingState{
			MaxPending: test.maxpending,
			Log:        zap.New(),
		}
		for i := 0; i < test.npending; i++ {
			id := strconv.Itoa(i)
			if test.differentIDs == false {
				id = "same"
			}
			var vid RgIDType
			vid = RgIDType(id)
			err := vid.CheckAndUpdatePendingState(pendState)
			if debugUnitTest {
				fmt.Printf("test %v err %v\n", test, err)
			}
			if i+1 == test.npending {
				if test.errormsg == "none" {
					if err != nil {
						t.Error("Expected no error but got: " + err.Error())
					}
				} else {
					if err != nil && !strings.Contains(err.Error(), test.errormsg) {
						t.Error("Didn't get expected error: " + test.errormsg)
					}
				}
			}
		}
		for i := 0; i <= test.maxpending; i++ {
			id := strconv.Itoa(i)
			if test.differentIDs == false {
				id = "same"
			}
			var vid RgIDType
			vid = RgIDType(id)
			vid.ClearPending(pendState)
		}
	}
}
