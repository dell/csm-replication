/*
 Copyright © 2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DellCSIMigrationGroupSpec defines the desired state of DellCSIMigrationGroup
type DellCSIMigrationGroupSpec struct {
	DriverName               string            `json:"driverName"`
	SourceID                 string            `json:"sourceID"`
	TargetID                 string            `json:"targetID"`
	MigrationGroupAttributes map[string]string `json:"migrationGroupAttributes"`
}

// DellCSIMigrationGroupStatus defines the observed state of DellCSIMigrationGroup
type DellCSIMigrationGroupStatus struct {
	State      string `json:"state,omitempty"`
	LastAction string `json:"lastAction,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=mg
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="State of the CR"
// +kubebuilder:printcolumn:name="Source ID",type=string,JSONPath=`.spec.sourceID`,description="Source ID"
// +kubebuilder:printcolumn:name="Target ID",type=string,JSONPath=`.spec.targetID`,description="Target ID"

// DellCSIMigrationGroup is the Schema for the dellcsimigrationgroup API
type DellCSIMigrationGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DellCSIMigrationGroupSpec   `json:"spec,omitempty"`
	Status            DellCSIMigrationGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DellCSIMigrationGroupList contains a list of DellCSIMigrationGroup
type DellCSIMigrationGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []DellCSIMigrationGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DellCSIMigrationGroup{}, &DellCSIMigrationGroupList{})
}
