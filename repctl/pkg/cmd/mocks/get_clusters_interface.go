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

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/dell/repctl/pkg/cmd (interfaces: GetClustersInterface)
//
// Generated by this command:
//
//	mockgen -destination=mocks/get_clusters_interface.go -package mocks . GetClustersInterface
//

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	k8s "github.com/dell/repctl/pkg/k8s"
	gomock "go.uber.org/mock/gomock"
)

// MockGetClustersInterface is a mock of GetClustersInterface interface.
type MockGetClustersInterface struct {
	ctrl     *gomock.Controller
	recorder *MockGetClustersInterfaceMockRecorder
	isgomock struct{}
}

// MockGetClustersInterfaceMockRecorder is the mock recorder for MockGetClustersInterface.
type MockGetClustersInterfaceMockRecorder struct {
	mock *MockGetClustersInterface
}

// NewMockGetClustersInterface creates a new mock instance.
func NewMockGetClustersInterface(ctrl *gomock.Controller) *MockGetClustersInterface {
	mock := &MockGetClustersInterface{ctrl: ctrl}
	mock.recorder = &MockGetClustersInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGetClustersInterface) EXPECT() *MockGetClustersInterfaceMockRecorder {
	return m.recorder
}

// GetAllClusters mocks base method.
func (m *MockGetClustersInterface) GetAllClusters(clusterIDs []string, configDir string) (*k8s.Clusters, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAllClusters", clusterIDs, configDir)
	ret0, _ := ret[0].(*k8s.Clusters)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAllClusters indicates an expected call of GetAllClusters.
func (mr *MockGetClustersInterfaceMockRecorder) GetAllClusters(clusterIDs, configDir any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAllClusters", reflect.TypeOf((*MockGetClustersInterface)(nil).GetAllClusters), clusterIDs, configDir)
}
