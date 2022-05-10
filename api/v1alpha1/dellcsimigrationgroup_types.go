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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DellCSIMigrationGroupSpec defines the desired state of DellCSIMigrationGroup
type DellCSIMigrationGroupSpec struct {
	DriverName    string `json:"driverName"`
	SourceArrayID string `json:"sourceArrayID"`
	TargetArrayID string `json:"targetArrayId"`
	LastAction    string `json:"lastAction"`
	State         string `json:"state"`
}

type DellCSIMigrationGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DellCSIMigrationGroupSpec `json:"spec,omitempty"`
}

// DellCSIMigrationGroupList contains a list of DellCSIMigrationGroup
type DellCSIMigrationGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []DellCSIMigrationGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DellCSIMigrationGroup{}, &DellCSIMigrationGroupList{})
}
