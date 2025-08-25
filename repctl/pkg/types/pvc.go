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

package types

import (
	"os"

	"github.com/dell/repctl/pkg/display"
	"github.com/dell/repctl/pkg/metadata"
	v1 "k8s.io/api/core/v1"
)

// PersistentVolumeClaim represents Persistent Volume Claim k8s resource
type PersistentVolumeClaim struct {
	Name               string `display:"Name"`
	PVName             string
	SCName             string `display:"SC"`
	RGName             string `display:"RG"`
	Namespace          string `display:"Namespace"`
	RemotePVCName      string `display:"rPVC"`
	RemotePVName       string `display:"rPV"`
	RemotePVCNamespace string `display:"rPVCNamespace"`
	RemoteClusterID    string `display:"RemoteClusterID"`
}

// PersistentVolumeClaimList list of PersistentVolumeClaim objects
type PersistentVolumeClaimList struct {
	PVCList []PersistentVolumeClaim
}

// Print prints list of persistent volumes claims to stdout as a table
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

// GetPVC converts k8s PersistentVolumeClaim type to custom representation
func GetPVC(claim v1.PersistentVolumeClaim) PersistentVolumeClaim {
	remotepvcName := claim.Annotations[metadata.RemotePVC]
	remoteNamespace := claim.Annotations[metadata.RemotePVCNamespace]
	remotepvName := claim.Annotations[metadata.RemotePV]
	rgName := claim.Annotations[metadata.ReplicationGroup]
	remoteClusterID := claim.Annotations[metadata.RemoteClusterID]
	pvc := PersistentVolumeClaim{
		Name:               claim.Name,
		PVName:             claim.Spec.VolumeName,
		Namespace:          claim.Namespace,
		SCName:             *claim.Spec.StorageClassName,
		RemotePVCNamespace: remoteNamespace,
		RemotePVCName:      remotepvcName,
		RemotePVName:       remotepvName,
		RGName:             rgName,
		RemoteClusterID:    remoteClusterID,
	}
	return pvc
}
