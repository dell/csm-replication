/*
 Copyright © 2021-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package mocks

import (
	"fmt"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/pkg/connection"
	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtime2 "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ connection.MultiClusterClient = &FakeConfig{}

// New creates new fake config with prepopulated parameters
func New(source string, targets ...string) connection.MultiClusterClient {
	config := &FakeConfig{
		sourceCluster: source,
		clusterClient: make(map[string]*connection.RemoteK8sControllerClient),
	}
	for _, target := range targets {
		obj := []runtime.Object{}
		// creating fake storage-class with replication params
		parameters := map[string]string{
			"RdfGroup":           "2",
			"RdfMode":            "ASYNC",
			"RemoteRDFGroup":     "2",
			"RemoteSYMID":        "000000000002",
			"RemoteServiceLevel": "Bronze",
			"SRP":                "SRP_1",
			"SYMID":              "000000000001",
			"ServiceLevel":       "Bronze",
			"replication.storage.dell.com/isReplicationEnabled":   "true",
			"replication.storage.dell.com/remoteClusterID":        "remote-123",
			"replication.storage.dell.com/remoteStorageClassName": "fake-sc",
		}
		var policy v1.PersistentVolumeReclaimPolicy
		policy = "Delete"

		scObj := storagev1.StorageClass{
			ObjectMeta:    metav1.ObjectMeta{Name: "fake-sc"},
			Provisioner:   "csi-fake",
			Parameters:    parameters,
			ReclaimPolicy: &policy,
		}
		obj = append(obj, &scObj)
		client, _ := fake_client.NewFakeClient(obj, nil, nil)
		config.clusterClient[target] = &connection.RemoteK8sControllerClient{
			ClusterID: target,
			Client:    client,
		}
	}
	return config
}

// FakeConfig is a structure that implements MultiClusterClient interface providing dummy functionality
type FakeConfig struct {
	sourceCluster string
	clusterClient map[string]*connection.RemoteK8sControllerClient
}

// GetClusterID returns ID of fake cluster
func (c *FakeConfig) GetClusterID() string {
	return c.sourceCluster
}

// GetConnection returns fake connection remote client
func (c *FakeConfig) GetConnection(clusterID string) (connection.RemoteClusterClient, error) {
	var (
		client *connection.RemoteK8sControllerClient
		ok     bool
	)
	if client, ok = c.clusterClient[clusterID]; !ok {
		return nil, fmt.Errorf("clusterID - %s not found", clusterID)
	}
	return client, nil
}

// NewFakeConfig returns new FakeClient with initialized fake remote clients
func NewFakeConfig(source string, targets ...string) connection.MultiClusterClient {
	config := &FakeConfig{
		sourceCluster: source,
		clusterClient: make(map[string]*connection.RemoteK8sControllerClient),
	}
	scheme1 := runtime.NewScheme()
	runtime2.Must(scheme.AddToScheme(scheme1))
	runtime2.Must(repv1.AddToScheme(scheme1))
	runtime2.Must(s1.AddToScheme(scheme1))
	for _, target := range targets {
		snapshotClass := &s1.VolumeSnapshotClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-snapshot-class",
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme1).WithObjects(snapshotClass).Build()
		config.clusterClient[target] = &connection.RemoteK8sControllerClient{
			ClusterID: target,
			Client:    fakeClient,
		}
	}
	return config
}

// NewFakeConfigForSingleCluster returns new FakeClient configured to be used in single-cluster scenario
func NewFakeConfigForSingleCluster(selfClient client.Client, source string, targets ...string) connection.MultiClusterClient {
	config := &FakeConfig{
		sourceCluster: source,
		clusterClient: make(map[string]*connection.RemoteK8sControllerClient),
	}
	scheme1 := runtime.NewScheme()
	runtime2.Must(scheme.AddToScheme(scheme1))
	runtime2.Must(repv1.AddToScheme(scheme1))
	for _, target := range targets {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme1).Build()
		config.clusterClient[target] = &connection.RemoteK8sControllerClient{
			ClusterID: target,
			Client:    fakeClient,
		}
	}
	config.clusterClient["self"] = &connection.RemoteK8sControllerClient{
		ClusterID: "self",
		Client:    selfClient,
	}
	return config
}
