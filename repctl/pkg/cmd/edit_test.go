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

package cmd

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/pkg/k8s"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockCluster struct {
	k8s.ClusterInterface
	repGroups map[string]*repv1.DellCSIReplicationGroup
}

func (m *MockCluster) GetReplicationGroups(ctx context.Context, rgID string) (*repv1.DellCSIReplicationGroup, error) {
	if rg, exists := m.repGroups[rgID]; exists {
		return rg, nil
	}
	return nil, fmt.Errorf("replication group not found")
}

type MockMultiClusterConfigurator struct {
	k8s.MultiClusterConfigurator
	clusters []k8s.ClusterInterface
}

func (m *MockMultiClusterConfigurator) GetAllClusters(args []string, configFolder string) (*k8s.Clusters, error) {
	return &k8s.Clusters{Clusters: m.clusters}, nil
}

// func TestGetRGAndClusterFromRGID(t *testing.T) {
// 	mockRG := &repv1.DellCSIReplicationGroup{
// 		Status: repv1.DellCSIReplicationGroupStatus{
// 			ReplicationLinkState: repv1.ReplicationLinkState{IsSource: true},
// 		},
// 	}
// 	mockCluster := &MockCluster{repGroups: map[string]*repv1.DellCSIReplicationGroup{"rgID": mockRG}}
// 	mockConfigurator := &MockMultiClusterConfigurator{clusters: []k8s.ClusterInterface{mockCluster}}

// 	tests := []struct {
// 		configFolder string
// 		rgID         string
// 		filter       string
// 		expectedRG   *repv1.DellCSIReplicationGroup
// 		expectedErr  error
// 	}{
// 		{"configFolder", "rgID", "", mockRG, nil},
// 		{"configFolder", "rgID", "src", mockRG, nil},
// 		{"configFolder", "rgID", "tgt", nil, fmt.Errorf("no matching cluster having (tgt) RG:(rgID) found")},
// 		{"configFolder", "invalidRGID", "", nil, fmt.Errorf("no matching cluster having () RG:(invalidRGID) found")},
// 	}

// 	for _, test := range tests {
// 		_, rg, err := GetRGAndClusterFromRGID(mockConfigurator, test.configFolder, test.rgID, test.filter)
// 		assert.Equal(t, test.expectedRG, rg)
// 		if test.expectedErr != nil {
// 			assert.EqualError(t, err, test.expectedErr.Error())
// 		} else {
// 			assert.NoError(t, err)
// 		}
// 	}
// }

func TestDecodedSecret_ToSecret(t *testing.T) {
	type fields struct {
		TypeMeta   metav1.TypeMeta
		ObjectMeta metav1.ObjectMeta
		Immutable  *bool
		Data       map[string]string
		StringData map[string]string
		Type       v1.SecretType
	}
	tests := []struct {
		name   string
		fields fields
		want   *v1.Secret
	}{
		{
			name: "Successful conversion",
			fields: fields{
				TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "my-namespace"},
				Immutable:  new(bool),
				Data:       map[string]string{"key1": "value1", "key2": "value2"},
				StringData: map[string]string{"key3": "value3", "key4": "value4"},
				Type:       v1.SecretTypeOpaque,
			},
			want: &v1.Secret{
				TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "my-namespace"},
				Immutable:  new(bool),
				Data:       map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
				StringData: map[string]string{"key3": "value3", "key4": "value4"},
				Type:       v1.SecretTypeOpaque,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DecodedSecret{
				TypeMeta:   tt.fields.TypeMeta,
				ObjectMeta: tt.fields.ObjectMeta,
				Immutable:  tt.fields.Immutable,
				Data:       tt.fields.Data,
				StringData: tt.fields.StringData,
				Type:       tt.fields.Type,
			}
			if got := s.ToSecret(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DecodedSecret.ToSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecret_ToDecodedSecret(t *testing.T) {
	type fields struct {
		Secret *v1.Secret
	}
	tests := []struct {
		name   string
		fields fields
		want   *DecodedSecret
	}{
		{
			name: "Successful conversion",
			fields: fields{
				Secret: &v1.Secret{
					TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
					ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "my-namespace"},
					Immutable:  new(bool),
					Data:       map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
					StringData: map[string]string{"key3": "value3", "key4": "value4"},
					Type:       v1.SecretTypeOpaque,
				},
			},
			want: &DecodedSecret{
				TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "my-namespace"},
				Immutable:  new(bool),
				Data:       map[string]string{"key1": "value1", "key2": "value2"},
				StringData: map[string]string{"key3": "value3", "key4": "value4"},
				Type:       v1.SecretTypeOpaque,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Secret{
				Secret: tt.fields.Secret,
			}
			if got := s.ToDecodedSecret(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Secret.ToDecodedSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}
