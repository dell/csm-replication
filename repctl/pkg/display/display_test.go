/*
 Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package display_test

import (
	"bytes"
	"fmt"
	"github.com/dell/repctl/pkg/display"
	"github.com/dell/repctl/pkg/types"
	"github.com/stretchr/testify/suite"
	"testing"
)

type DisplayTestSuite struct {
	suite.Suite
}

func (suite *DisplayTestSuite) TestTableWriter() {
	sc := types.StorageClass{
		Name:            "test-replication",
		Provisioner:     "test.dellemc.com",
		RemoteSCName:    "test-replication",
		LocalClusterId:  "cluster-A",
		RemoteClusterId: "cluster-B",
		Valid:           "true",
	}

	res := bytes.NewBufferString("")
	table, err := display.NewTableWriter(sc, res)
	suite.NoError(err)

	table.PrintHeader()
	table.PrintRow(sc)
	table.Done()

	fmt.Println(res.String())
	suite.Equal(
		"+-------------------------+\n| StorageClass            |\n+-------------------------+\nName\t\t\tDriver\t\t\tRemoteSC Name\t\tLocal Cluster\tRemote Cluster\tValid\t\ntest-replication\ttest.dellemc.com\ttest-replication\tcluster-A\tcluster-B\ttrue\t\n",
		res.String())
}

func TestDisplayTestSuite(t *testing.T) {
	suite.Run(t, new(DisplayTestSuite))
}
