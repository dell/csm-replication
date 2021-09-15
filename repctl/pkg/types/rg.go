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

package types

import (
	"os"

	"github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/repctl/pkg/display"
	"github.com/dell/repctl/pkg/metadata"
)

type RG struct {
	Name              string `display:"Name"`
	State             string `display:"State"`
	ProtectionGroupId string
	RemoteClusterId   string `display:"rClusterID"`
	DriverName        string `display:"Driver"`
	RemoteRGName      string `display:"RemoteRG"`
	IsSource          bool   `display:"IsSource"`
	LinkState         string `display:"LinkState"`
}

type RGList struct {
	RGList []RG
}

func (r *RGList) Print() {
	// Form an empty object and create a new table writer
	t, err := display.NewTableWriter(RG{}, os.Stdout)
	if err != nil {
		return
	}
	t.PrintHeader()
	for _, obj := range r.RGList {
		t.PrintRow(obj)
	}
	t.Done()
}

func GetRG(group v1alpha1.DellCSIReplicationGroup) RG {
	remoteClusterId := group.Annotations[metadata.RemoteClusterID]
	remoteRGName := group.Annotations[metadata.RemoteReplicationGroup]
	myRG := RG{
		Name:              group.Name,
		State:             group.Status.State,
		ProtectionGroupId: group.Spec.ProtectionGroupID,
		DriverName:        group.Spec.DriverName,
		RemoteClusterId:   remoteClusterId,
		RemoteRGName:      remoteRGName,
		IsSource:          group.Status.ReplicationLinkState.IsSource,
		LinkState:         group.Status.ReplicationLinkState.State,
	}
	return myRG
}
