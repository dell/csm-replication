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
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Mock for os.ReadDir
func mockReadDirSuccess(name string) ([]fs.DirEntry, error) {
	return []fs.DirEntry{
		mockDirEntry{name: "host0"},
		mockDirEntry{name: "host1"},
	}, nil
}

type mockDirEntry struct {
	name string
}

func (m mockDirEntry) Name() string               { return m.name }
func (m mockDirEntry) IsDir() bool                { return false }
func (m mockDirEntry) Type() fs.FileMode          { return 0 }
func (m mockDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestRescanNode_Success(t *testing.T) {
	// Save the original os.ReadDir function
	originalReadDir := os.ReadDir
	// Mock os.ReadDir
	OsReader = mockReadDirSuccess
	// Restore the original os.ReadDir function after the test
	defer func() { OsReader = originalReadDir }()

	ctx := context.Background()
	err := RescanNode(ctx)
	assert.NoError(t, err)
}

// Mock for os.ReadDir to simulate failure
func mockReadDirFailure(name string) ([]fs.DirEntry, error) {
	return nil, errors.New("failed to read directory")
}

func TestRescanSCSIHostAll_ReadDirFailure(t *testing.T) {
	// Save the original os.ReadDir function
	originalReadDir := os.ReadDir
	// Mock os.ReadDir to simulate failure
	OsReader = mockReadDirFailure
	// Restore the original os.ReadDir function after the test
	defer func() { OsReader = originalReadDir }()

	ctx := context.Background()
	err := RescanNode(ctx)
	assert.Error(t, err)
	assert.Equal(t, "failed to read directory", err.Error())
}
