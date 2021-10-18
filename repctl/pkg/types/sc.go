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

// StorageClass represents Storage Class k8s resource
type StorageClass struct {
	Name            string `display:"Name"`
	Provisioner     string `display:"Driver"`
	RemoteSCName    string `display:"RemoteSC Name"`
	LocalClusterID  string `display:"Local Cluster"`
	RemoteClusterID string `display:"Remote Cluster"`
	Parameters      map[string]string
	Valid           string `display:"Valid"`
}

// SCList list of StorageClass objects
type SCList struct {
	SCList []StorageClass
}

// Print prints list of storage classes to stdout as a table
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

// GetSC converts k8s StorageClass type to custom representation
func GetSC(sc storagev1.StorageClass, clusterID string, repEnabled bool) StorageClass {
	mysc := StorageClass{}
	params := sc.Parameters
	mysc.Name = sc.Name
	mysc.Provisioner = sc.Provisioner
	mysc.Parameters = params
	if repEnabled {
		mysc.LocalClusterID = clusterID
		remoteClusterID := params[metadata.RemoteClusterID]
		mysc.Valid = "true"
		if remoteClusterID == "" {
			mysc.Valid = "false"
		} else {
			mysc.RemoteClusterID = remoteClusterID
		}
		remoteSCName := params[metadata.RemoteSCName]
		if remoteSCName == "" {
			mysc.Valid = "false"
		} else {
			mysc.RemoteSCName = remoteSCName
		}
	} else {
		mysc.RemoteSCName = "N/A"
		mysc.LocalClusterID = "N/A"
		mysc.RemoteClusterID = "N/A"
		mysc.Valid = "N/A"
	}
	return mysc
}
