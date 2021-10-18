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
	"encoding/json"
	"fmt"
	"github.com/dell/repctl/pkg/display"
	"github.com/dell/repctl/pkg/metadata"
	v1 "k8s.io/api/core/v1"
	"os"
)

// PersistentVolume represents Persistent Volume k8s resource
type PersistentVolume struct {
	Name               string `display:"Name"`
	RGName             string `display:"RG"`
	SCName             string `display:"SC"`
	PVCName            string `display:"PVC"`
	PVCNameSpace       string `display:"PVCNamespace"`
	RemotePVCName      string `display:"rPVC"`
	RemotePVCNameSpace string `display:"rPVCNamespace"`
	Requests           v1.ResourceList
	AccessMode         []v1.PersistentVolumeAccessMode
	VolumeMode         *v1.PersistentVolumeMode
	ReclaimPolicy      v1.PersistentVolumeReclaimPolicy
	Labels             map[string]string
	Annotations        map[string]string
}

// GetPV converts k8s PersistentVolume type to custom representation
func GetPV(persistentVolume *v1.PersistentVolume) (PersistentVolume, error) {
	// Get values from annotations
	// Get namespace annotation
	if persistentVolume == nil {
		return PersistentVolume{}, fmt.Errorf("unexpected error - nil object encountered")
	}
	rPVCNameSpace := persistentVolume.Annotations[metadata.RemotePVCNamespace]
	// Get the PVC name
	rPVCName := persistentVolume.Annotations[metadata.RemotePVC]
	rgName := persistentVolume.Annotations[metadata.ReplicationGroup]
	requests := persistentVolume.Annotations[metadata.ResourceRequest]
	var requestsResReq v1.ResourceList
	if requests != "" {
		err := json.Unmarshal([]byte(requests), &requestsResReq)
		if err != nil {
			fmt.Printf("Failed to unmarshal json for resource requirements. Error: %s\n", err.Error())
			return PersistentVolume{}, err
		}
	}
	if rPVCNameSpace == "" {
		rPVCNameSpace = "N/A"
	}
	pv := PersistentVolume{
		Name:               persistentVolume.Name,
		RGName:             rgName,
		SCName:             persistentVolume.Spec.StorageClassName,
		Requests:           requestsResReq,
		RemotePVCName:      rPVCName,
		RemotePVCNameSpace: rPVCNameSpace,
		ReclaimPolicy:      persistentVolume.Spec.PersistentVolumeReclaimPolicy,
		VolumeMode:         persistentVolume.Spec.VolumeMode,
		AccessMode:         persistentVolume.Spec.AccessModes,
		Labels:             persistentVolume.Labels,
		Annotations:        persistentVolume.Annotations,
	}
	if persistentVolume.Spec.ClaimRef != nil {
		pv.PVCName = persistentVolume.Spec.ClaimRef.Name
		pv.PVCNameSpace = persistentVolume.Spec.ClaimRef.Namespace
	}
	return pv, nil
}

// PersistentVolumeList list of PersistentVolume objects
type PersistentVolumeList struct {
	PVList []PersistentVolume
}

// Print prints list of persistent volumes to stdout as a table
func (p *PersistentVolumeList) Print() {
	// Form an empty object and create a new table writer
	t, err := display.NewTableWriter(PersistentVolume{}, os.Stdout)
	if err != nil {
		return
	}
	t.PrintHeader()
	for _, obj := range p.PVList {
		t.PrintRow(obj)
	}
	t.Done()
}
