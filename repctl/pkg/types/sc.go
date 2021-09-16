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
	"github.com/dell/repctl/pkg/display"
	"github.com/dell/repctl/pkg/metadata"
	storagev1 "k8s.io/api/storage/v1"
	"os"
)

type StorageClass struct {
	Name            string `display:"Name"`
	Provisioner     string `display:"Driver"`
	RemoteSCName    string `display:"RemoteSC Name"`
	LocalClusterId  string `display:"Local Cluster"`
	RemoteClusterId string `display:"Remote Cluster"`
	Parameters      map[string]string
	Valid           string `display:"Valid"`
}

type SCList struct {
	SCList []StorageClass
}

func (s *SCList) Print() {
	// Form an empty object and create a new table writer
	t, err := display.NewTableWriter(StorageClass{}, os.Stdout)
	if err != nil {
		return
	}
	t.PrintHeader()
	for _, obj := range s.SCList {
		t.PrintRow(obj)
	}
	t.Done()
}

func GetSC(sc storagev1.StorageClass, clusterId string, repEnabled bool) StorageClass {
	mysc := StorageClass{}
	params := sc.Parameters
	mysc.Name = sc.Name
	mysc.Provisioner = sc.Provisioner
	mysc.Parameters = params
	if repEnabled {
		mysc.LocalClusterId = clusterId
		remoteClusterId := params[metadata.RemoteClusterID]
		mysc.Valid = "true"
		if remoteClusterId == "" {
			mysc.Valid = "false"
		} else {
			mysc.RemoteClusterId = remoteClusterId
		}
		remoteSCName := params[metadata.RemoteSCName]
		if remoteSCName == "" {
			mysc.Valid = "false"
		} else {
			mysc.RemoteSCName = remoteSCName
		}
	} else {
		mysc.RemoteSCName = "N/A"
		mysc.LocalClusterId = "N/A"
		mysc.RemoteClusterId = "N/A"
		mysc.Valid = "N/A"
	}
	return mysc
}
