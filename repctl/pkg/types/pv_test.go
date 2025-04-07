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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPV(t *testing.T) {
	tests := []struct {
		name             string
		persistentVolume *v1.PersistentVolume
		expectedPV       PersistentVolume
		expectedError    error
	}{
		{
			name:             "Nil PersistentVolume",
			persistentVolume: nil,
			expectedPV:       PersistentVolume{},
			expectedError:    fmt.Errorf("unexpected error - nil object encountered"),
		},
		{
			name: "PersistentVolume with missing annotations",
			persistentVolume: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv1",
					Annotations: map[string]string{
						"remotePVCNamespace": "",
						"remotePVC":          "",
						"replicationGroup":   "",
						"resourceRequest":    "",
					},
				},
				Spec: v1.PersistentVolumeSpec{
					StorageClassName:              "standard",
					PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
					AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				},
			},
			expectedPV: PersistentVolume{
				Name:               "pv1",
				RGName:             "",
				SCName:             "standard",
				Requests:           v1.ResourceList(nil),
				RemotePVCName:      "",
				RemotePVCNameSpace: "N/A",
				ReclaimPolicy:      v1.PersistentVolumeReclaimRetain,
				AccessMode:         []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				Labels:             map[string]string(nil),
				Annotations: map[string]string{
					"remotePVCNamespace": "",
					"remotePVC":          "",
					"replicationGroup":   "",
					"resourceRequest":    "",
				},
			},
			expectedError: nil,
		},
		{
			name: "PersistentVolume with ClaimRef",
			persistentVolume: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv2",
					Annotations: map[string]string{
						"remotePVCNamespace": "",
						"remotePVC":          "",
						"replicationGroup":   "",
						"resourceRequest":    "",
					},
				},
				Spec: v1.PersistentVolumeSpec{
					StorageClassName:              "standard",
					PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
					AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					ClaimRef: &v1.ObjectReference{
						Name:      "pvc2",
						Namespace: "namespace2",
					},
				},
			},
			expectedPV: PersistentVolume{
				Name:               "pv2",
				RGName:             "",
				SCName:             "standard",
				Requests:           v1.ResourceList(nil),
				RemotePVCName:      "",
				RemotePVCNameSpace: "N/A",
				PVCName:            "pvc2",
				PVCNameSpace:       "namespace2",
				ReclaimPolicy:      v1.PersistentVolumeReclaimRetain,
				AccessMode:         []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				Labels:             map[string]string(nil),
				Annotations: map[string]string{
					"remotePVCNamespace": "",
					"remotePVC":          "",
					"replicationGroup":   "",
					"resourceRequest":    "",
				},
			},
			expectedError: nil,
		},
		{
			name: "PersistentVolume with valid resourceRequest annotation",
			persistentVolume: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv3",
					Annotations: map[string]string{
						"remotePVCNamespace": "remote-namespace",
						"remotePVC":          "remote-pvc",
						"replicationGroup":   "rg3",
						"resourceRequest":    `{"storage":"10Gi"}`,
					},
				},
				Spec: v1.PersistentVolumeSpec{
					StorageClassName:              "standard",
					PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
					AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				},
			},
			expectedPV: PersistentVolume{
				Name:   "pv3",
				RGName: "",
				SCName: "standard",
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("10Gi"),
				},
				RemotePVCName:      "remote-pvc",
				RemotePVCNameSpace: "N/A",
				ReclaimPolicy:      v1.PersistentVolumeReclaimRetain,
				AccessMode:         []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				Labels:             map[string]string(nil),
				Annotations: map[string]string{
					"remotePVCNamespace": "remote-namespace",
					"remotePVC":          "remote-pvc",
					"replicationGroup":   "rg3",
					"resourceRequest":    `{"storage":"10Gi"}`,
				},
			},
			expectedError: nil,
		},
		// {
		// 	name: "PersistentVolume with invalid resourceRequest annotation",
		// 	persistentVolume: &v1.PersistentVolume{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name: "pv4",
		// 			Annotations: map[string]string{
		// 				"remotePVCNamespace": "remote-namespace",
		// 				"remotePVC":          "remote-pvc",
		// 				"replicationGroup":   "rg4",
		// 				"resourceRequest":    `{"invalid":"data"}`,
		// 			},
		// 		},
		// 		Spec: v1.PersistentVolumeSpec{
		// 			StorageClassName:              "standard",
		// 			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
		// 			AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
		// 		},
		// 	},
		// 	expectedPV: PersistentVolume{},
		// 	expectedError: fmt.Errorf("Failed to unmarshal json for resource requirements. Error: %s", "json: cannot unmarshal object into Go value of type v1.ResourceList"),
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualPV, err := GetPV(tt.persistentVolume)
			assert.Equal(t, tt.expectedPV, actualPV)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestPersistentVolumeList_Print(t *testing.T) {
	pvList := &PersistentVolumeList{
		PVList: []PersistentVolume{
			{
				Name:               "test-pv",
				RGName:             "rg1",
				SCName:             "standard",
				PVCName:            "test-pvc",
				PVCNameSpace:       "test-namespace",
				RemotePVCName:      "remote-pvc",
				RemotePVCNameSpace: "remote-namespace",
				Requests:           v1.ResourceList{},
				AccessMode:         []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				VolumeMode:         &[]v1.PersistentVolumeMode{v1.PersistentVolumeFilesystem}[0],
				ReclaimPolicy:      v1.PersistentVolumeReclaimRetain,
				Labels:             map[string]string{},
				Annotations: map[string]string{
					"remotePVCNamespace": "remote-namespace",
					"remotePVC":          "remote-pvc",
					"replicationGroup":   "rg1",
					"resourceRequest":    "",
				},
			},
		},
	}
	pvList.Print()
}
