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
	v1 "k8s.io/api/core/v1"
	"os"
)

type PersistentVolumeClaim struct {
	Name               string `display:"Name"`
	PVName             string
	SCName             string `display:"SC"`
	RGName             string `display:"RG"`
	Namespace          string `display:"Namespace"`
	RemotePVCName      string `display:"rPVC"`
	RemotePVName       string `display:"rPV"`
	RemotePVCNamespace string `display:"rPVCNamespace"`
	RemoteClusterId    string `display:"RemoteClusterID"`
}

type PersistentVolumeClaimList struct {
	PVCList []PersistentVolumeClaim
}

func (p *PersistentVolumeClaimList) Print() {
	// Form an empty object and create a new table writer
	t, err := display.NewTableWriter(PersistentVolumeClaim{}, os.Stdout)
	if err != nil {
		return
	}
	t.PrintHeader()
	for _, obj := range p.PVCList {
		t.PrintRow(obj)
	}
	t.Done()
}

func GetPVC(claim v1.PersistentVolumeClaim) PersistentVolumeClaim {
	remotepvcName := claim.Annotations[metadata.RemotePVC]
	remoteNamespace := claim.Annotations[metadata.RemotePVCNamespace]
	remotepvName := claim.Annotations[metadata.RemotePV]
	rgName := claim.Annotations[metadata.ReplicationGroup]
	remoteClusterId := claim.Annotations[metadata.RemoteClusterID]
	pvc := PersistentVolumeClaim{
		Name:               claim.Name,
		PVName:             claim.Spec.VolumeName,
		Namespace:          claim.Namespace,
		SCName:             *claim.Spec.StorageClassName,
		RemotePVCNamespace: remoteNamespace,
		RemotePVCName:      remotepvcName,
		RemotePVName:       remotepvName,
		RGName:             rgName,
		RemoteClusterId:    remoteClusterId,
	}
	return pvc
}
