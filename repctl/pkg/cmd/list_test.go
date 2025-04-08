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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ListTestSuite struct {
	suite.Suite
	testDataFolder string
}

func (suite *ListTestSuite) SetupSuite() {
	curUser, err := os.UserHomeDir()
	suite.NoError(err)

	curUser = filepath.Join(curUser, folderPath)
	curUserPath, err := filepath.Abs(curUser)
	suite.NoError(err)

	suite.testDataFolder = curUserPath
}

func (suite *ListTestSuite) TestGetListCommand() {
	cmd := GetListCommand()

	// Test the command usage
	suite.Equal("get", cmd.Use)
	suite.Equal("lists different resources in clusters with configured replication", cmd.Short)

	// Test the persistent flags
	allFlag := cmd.PersistentFlags().Lookup("all")
	suite.NotNil(allFlag)
	suite.Equal("show all objects (overrides other filters)", allFlag.Usage)

	rnFlag := cmd.PersistentFlags().Lookup("rn")
	suite.NotNil(rnFlag)
	suite.Equal("remote namespace", rnFlag.Usage)

	rcFlag := cmd.PersistentFlags().Lookup("rc")
	suite.NotNil(rcFlag)
	suite.Equal("remote cluster id", rcFlag.Usage)

	subCommands := cmd.Commands()
	suite.NotEmpty(subCommands)
	for _, subCmd := range subCommands {
		suite.NotNil(subCmd)
	}
}

func TestListTestSuite(t *testing.T) {
	suite.Run(t, new(ListTestSuite))
}
