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

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPVC(t *testing.T) {
	tests := []struct {
		name        string
		claim       v1.PersistentVolumeClaim
		expectedPVC PersistentVolumeClaim
	}{
		{
			name: "PVC with missing annotations",
			claim: v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvc1",
					Annotations: map[string]string{
						"remotePVC":          "",
						"remotePVCNamespace": "",
						"remotePV":           "",
						"replicationGroup":   "",
						"remoteClusterID":    "",
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					VolumeName:       "pv1",
					StorageClassName: &[]string{"standard"}[0],
				},
			},
			expectedPVC: PersistentVolumeClaim{
				Name:               "pvc1",
				PVName:             "pv1",
				Namespace:          "",
				SCName:             "standard",
				RemotePVCNamespace: "",
				RemotePVCName:      "",
				RemotePVName:       "",
				RGName:             "",
				RemoteClusterID:    "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualPVC := GetPVC(tt.claim)
			assert.Equal(t, tt.expectedPVC, actualPVC)
		})
	}
}

func TestPersistentVolumeClaimList_Print(t *testing.T) {
	pvcList := &PersistentVolumeClaimList{
		PVCList: []PersistentVolumeClaim{
			{
				Name:               "test-pvc",
				PVName:             "test-pv",
				Namespace:          "test-namespace",
				SCName:             "standard",
				RemotePVCNamespace: "remote-namespace",
				RemotePVCName:      "remote-pvc",
				RemotePVName:       "remote-pv",
				RGName:             "rg1",
				RemoteClusterID:    "cluster-A",
			},
		},
	}

	pvcList.Print()
}
