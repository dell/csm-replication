/*
 Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DellCSIReplicationGroupSpec defines the desired state of DellCSIReplicationGroup
type DellCSIReplicationGroupSpec struct {
	DriverName                      string            `json:"driverName"`
	RequestParametersClass          string            `json:"requestParametersClass,omitempty"`
	Action                          string            `json:"action"`
	RemoteClusterID                 string            `json:"remoteClusterId"`
	ProtectionGroupID               string            `json:"protectionGroupId"`
	ProtectionGroupAttributes       map[string]string `json:"protectionGroupAttributes,omitempty"`
	RemoteProtectionGroupID         string            `json:"remoteProtectionGroupId"`
	RemoteProtectionGroupAttributes map[string]string `json:"remoteProtectionGroupAttributes,omitempty"`
}

// DellCSIReplicationGroupStatus defines the observed state of DellCSIReplicationGroup
type DellCSIReplicationGroupStatus struct {
	State                string               `json:"state,omitempty"`
	RemoteState          string               `json:"remoteState,omitempty"`
	ReplicationLinkState ReplicationLinkState `json:"replicationLinkState,omitempty"`
	LastAction           LastAction           `json:"lastAction,omitempty"`
	Conditions           []LastAction         `json:"conditions,omitempty"`
}

// LastAction - Stores the last updated action
type LastAction struct {
	// Condition is the last known condition of the Custom Resource
	Condition string `json:"condition,omitempty"`

	// FirstFailure is the first time this action failed
	FirstFailure *metav1.Time `json:"firstFailure,omitempty"`

	// Time is the time stamp for the last action update
	Time *metav1.Time `json:"time,omitempty"`

	// ErrorMessage is the last error message associated with the condition
	ErrorMessage string `json:"errorMessage,omitempty"`

	// ActionAttributes content unique on response to an action
	ActionAttributes map[string]string `json:"actionAttributes,omitempty"`
}

// ReplicationLinkState - Stores the Replication Link State
type ReplicationLinkState struct {
	// State is the last reported state of the Replication Link
	State string `json:"state,omitempty"`
	// IsSource indicates if this site is primary
	IsSource bool `json:"isSource"`

	// LastSuccessfulUpdate is the time stamp for the last state update
	LastSuccessfulUpdate *metav1.Time `json:"lastSuccessfulUpdate,omitempty"`

	// ErrorMessage is the last error message associated with the link state
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=rg
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="State of the CR"
// +kubebuilder:printcolumn:name="PG ID",type=string,JSONPath=`.spec.protectionGroupId`,description="Protection Group ID"
// +kubebuilder:printcolumn:name="Link State",type=string,JSONPath=`.status.replicationLinkState.state`,description="Replication Link State"
// +kubebuilder:printcolumn:name="Last LinkState Update",type=string,JSONPath=`.status.replicationLinkState.lastSuccessfulUpdate`,description="Replication Link State"

// DellCSIReplicationGroup is the Schema for the dellcsireplicationgroups API
type DellCSIReplicationGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DellCSIReplicationGroupSpec   `json:"spec,omitempty"`
	Status DellCSIReplicationGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DellCSIReplicationGroupList contains a list of DellCSIReplicationGroup
type DellCSIReplicationGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []DellCSIReplicationGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DellCSIReplicationGroup{}, &DellCSIReplicationGroupList{})
}
