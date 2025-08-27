/*
 *
 * Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package connection

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestConnect(t *testing.T) {
	// Test case: Successful connection
	address := "unix:///path/to/socket"
	log := logr.Discard()

	conn, err := Connect(address, log)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	// Test case: Missing prefix
	address = "/path/to/socket"
	conn, err = Connect(address, log)
	assert.NoError(t, err)
	assert.NotNil(t, conn)
}
