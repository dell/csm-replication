/*
Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.

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

// revive:disable:var-naming
package utils

// revive:enable:var-naming

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	fakeclient "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/csm-replication/test/mock-server/server"
	"github.com/dell/csm-replication/test/mock-server/stub"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/fatih/color"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1 "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	// FakePVName name of PV
	FakePVName = "fake-pv"
	// FakePVCName name of PVC
	FakePVCName = "fake-pvc"
	// FakeSCName name of SC
	FakeSCName = "fake-sc"
	// FakeDriverName name of driver
	FakeDriverName = "fake-csi-driver"
	// FakeRemoteSCName name of remote SC
	FakeRemoteSCName = "fake-remote-sc"
	// FakeRGName name of remote RG
	FakeRGName = "fake-rg"
	// FakeNamespaceName name of namespace
	FakeNamespaceName = "fake-namespace"
	// Self this cluster
	Self = "self"
	// RemoteClusterID id remote cluster
	RemoteClusterID = "remoteCluster"
	// SourceClusterID id source cluster
	SourceClusterID = "sourceCluster"
	// LocalPGID id of local protection group
	LocalPGID = "l-group-id"
	// RemotePGID id of remote protection group
	RemotePGID = "r-group-id"
	// ContextPrefix fake csi replication prefix
	ContextPrefix = "csi-fake"
)

// Driver mock implementation of the driver
type Driver struct {
	DriverName      string
	StorageClass    string
	RemoteClusterID string
	RemoteSCName    string
	SourceClusterID string
	Namespace       string
	RGName          string
	PVName          string
}

// GetDefaultDriver returns default mock implementation of the driver
func GetDefaultDriver() Driver {
	return Driver{
		DriverName:      FakeDriverName,
		StorageClass:    FakeSCName,
		RemoteClusterID: RemoteClusterID,
		RemoteSCName:    FakeRemoteSCName,
		SourceClusterID: SourceClusterID,
		Namespace:       FakeNamespaceName,
		RGName:          FakeRGName,
		PVName:          FakePVName,
	}
}

// Common contains common resources
type Common struct {
	Namespace string
}

var log logr.Logger

// MockServer mock grpc server
var MockServer *grpc.Server

// Scheme runtime Scheme
var Scheme = runtime.NewScheme()

// Only initialize schemes once to prevent race condition during testing
var schemesInitialized = false

// PVCName name of testing PVC
const PVCName = "test-pvc"

// InitializeSchemes inits client-go and replication v1 schemes
func InitializeSchemes() {
	if !schemesInitialized {
		utilruntime.Must(corev1.AddToScheme(Scheme))
		utilruntime.Must(repv1.AddToScheme(Scheme))
		schemesInitialized = true
	}
	// +kubebuilder:scaffold:scheme
}

// RunServer launches mock grpc server
func RunServer(stubsPath string) {
	log.Info("RUNNING MOCK SERVER")
	const (
		csiAddress       = "localhost:4772"
		defaultStubsPath = "../mock-server/stubs"
		apiPort          = "4771"
	)
	if len(stubsPath) == 0 {
		stubsPath = defaultStubsPath
	}
	// run admin stub server
	stub.RunStubServer(stub.Options{
		StubPath: stubsPath,
		Port:     apiPort,
		BindAddr: "0.0.0.0",
	})
	var protocol string
	if strings.Contains(csiAddress, ":") {
		protocol = "tcp"
	} else {
		protocol = "unix"
	}
	lis, err := net.Listen(protocol, csiAddress)
	if err != nil {
		log.Error(err, "failed to listen on address", "address", csiAddress)
		os.Exit(1)
	}

	MockServer = grpc.NewServer()

	replication.RegisterReplicationServer(MockServer, &server.Replication{})

	fmt.Printf("Serving gRPC on %s\n", csiAddress)
	errChan := make(chan error)

	// run blocking call in a separate goroutine, report errors via channel
	go func() {
		if err := MockServer.Serve(lis); err != nil {
			errChan <- err
		}
	}()
}

// ValidateAnnotations validates that given annotations are correct
func ValidateAnnotations(annotations map[string]string, t *testing.T) {
	t.Log("Annotations:", annotations)
	// To validate whether annotations are added or not properly
	assert.NotEqual(t, len(annotations), 0, "Annotations should not be nil")

	if len(annotations) > 0 {
		value, found := annotations[controllers.RemoteStorageClassAnnotation]
		assert.Equal(t, found, true, "Annotation %s not found", controllers.RemoteStorageClassAnnotation)
		assert.NotNil(t, value, "Annotation value:%s", value)

		value, found = annotations[controllers.RemoteVolumeAnnotation]
		assert.Equal(t, found, true, "Annotation %s not found", controllers.RemoteVolumeAnnotation)
		assert.NotNil(t, value, "Annotation value:%s", value)

		value, found = annotations[controllers.RemoteClusterID]
		assert.Equal(t, found, true, "Annotation %s not found", controllers.RemoteClusterID)
		assert.NotNil(t, value, "Annotation value:%s", value)

		value, found = annotations[controllers.ReplicationGroup]
		assert.Equal(t, found, true, "Annotation %s not found", controllers.ReplicationGroup)
		assert.NotNil(t, value, "Annotation value:%s", value)
	}
}

// ValidateRemotePVAnnotations validates that given PV annotations are correct
func ValidateRemotePVAnnotations(annotations map[string]string, t *testing.T) {
	t.Log("Annotations:", annotations)
	// To validate whether annotations are added or not properly
	assert.NotEqual(t, len(annotations), 0, "Annotations should not be nil")

	if len(annotations) > 0 {
		_, found := annotations[controllers.RemotePV]
		assert.Equal(t, found, true, "Annotation %s not found", controllers.RemotePV)
	}
}

// ValidateRemoteRGAnnotations validates that given RG annotations are correct
func ValidateRemoteRGAnnotations(annotations map[string]string, t *testing.T) {
	t.Log("Annotations:", annotations)
	// To validate whether annotations are added or not properly
	assert.NotEqual(t, len(annotations), 0, "Annotations should not be nil")

	if len(annotations) > 0 {
		_, found := annotations[controllers.RemoteReplicationGroup]
		assert.Equal(t, found, true, "Annotation %s not found", controllers.RemoteReplicationGroup)
	}
}

// GetConfigFromFile creates *rest.Config object from provided config path
func GetConfigFromFile(kubeconfig string) (*rest.Config, error) {
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		} else {
			return nil, fmt.Errorf("can not find config file in home directory, please explicitly specify it using flags")
		}
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// GetFakeClient returns fake k8s client
func GetFakeClient() client.Client {
	InitializeSchemes()
	fakeClient := fake.NewClientBuilder().WithScheme(Scheme).Build()
	return fakeClient
}

// GetFakeClientWithObjects returns fake k8s client and populates it with given objects
func GetFakeClientWithObjects(initObjs ...client.Object) client.Client {
	InitializeSchemes()
	fakeClient := fake.NewClientBuilder().WithScheme(Scheme).WithObjects(initObjs...).WithStatusSubresource(initObjs...).Build()
	return fakeClient
}

// GetPVObj returns PersistentVolume testing object
func GetPVObj(name, volHandle, provisionerName, scName string, volAttributes map[string]string) *v1.PersistentVolume {
	pvObj := v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:           provisionerName,
					VolumeHandle:     volHandle,
					FSType:           "ext4",
					VolumeAttributes: volAttributes,
				},
			},
			StorageClassName: scName,
		},
		Status: v1.PersistentVolumeStatus{Phase: v1.VolumeBound},
	}
	return &pvObj
}

// GetPVCObj returns PersistentVolumeClaim testing object
func GetPVCObj(pvcName string, namespace string, sc string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &sc,
			AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("3Gi"),
				},
			},
		},
	}
}

// GetParams returns correct map of replication parameters
func GetParams(remoteClusterID, remoteSCName string) map[string]string {
	params := map[string]string{
		"param": "val",
		"replication.storage.dell.com/isReplicationEnabled":   "true",
		"replication.storage.dell.com/remoteClusterID":        remoteClusterID,
		"replication.storage.dell.com/remoteStorageClassName": remoteSCName,
	}
	return params
}

// GetReplicationEnabledSC returns replication enabled StorageClass testing object
func GetReplicationEnabledSC(provisionerName, scName, remoteSCName, remoteClusterID string) *storagev1.StorageClass {
	scObj := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: scName},
		Provisioner: provisionerName,
		Parameters:  GetParams(remoteClusterID, remoteSCName),
	}
	return &scObj
}

// GetNonReplicationEnabledSC returns usual StorageClass testing object
func GetNonReplicationEnabledSC(provisionerName, scName string) *storagev1.StorageClass {
	scObj := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: scName},
		Provisioner: provisionerName,
		Parameters:  map[string]string{},
	}
	return &scObj
}

// GetReplicationEnabledSCWithMetroMode returns replication enabled StorageClass testing object with Metro mode
func GetReplicationEnabledSCWithMetroMode(provisionerName, scName, modeParamName string) *storagev1.StorageClass {
	scObj := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: scName},
		Provisioner: provisionerName,
		Parameters: map[string]string{
			"param": "val",
			"replication.storage.dell.com/isReplicationEnabled": "true",
			"replication.storage.dell.com/" + modeParamName:     "Metro",
		},
	}
	return &scObj
}

// GetRGObj returns DellCSIReplicationGroup testing object
func GetRGObj(name, driverName, remoteClusterID, pgID, remotePGID string, params,
	remoteParams map[string]string,
) *repv1.DellCSIReplicationGroup {
	replicationGroup := repv1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "",
		},
		Spec: repv1.DellCSIReplicationGroupSpec{
			DriverName:                      driverName,
			RemoteClusterID:                 remoteClusterID,
			ProtectionGroupAttributes:       params,
			ProtectionGroupID:               pgID,
			RemoteProtectionGroupID:         remotePGID,
			RemoteProtectionGroupAttributes: remoteParams,
		},
		Status: repv1.DellCSIReplicationGroupStatus{
			State:       "",
			RemoteState: "",
			ReplicationLinkState: repv1.ReplicationLinkState{
				State:                "",
				LastSuccessfulUpdate: &metav1.Time{},
				ErrorMessage:         "",
			},
			LastAction: repv1.LastAction{
				Condition:    "",
				Time:         &metav1.Time{},
				ErrorMessage: "",
			},
		},
	}
	return &replicationGroup
}

// WaitForAllToBeBound waits for every PVC, that belongs to PVCClient, to be Bound
func WaitForAllToBeBound(ctx context.Context, k8sClient client.Client, t *testing.T) error {
	// PVCPoll is a poll interval for PVC tests
	const Poll = 2 * time.Second
	// PVCTimeout is a timeout interval for PVC operations
	const Timeout = 1800 * time.Second
	startTime := time.Now()

	timeout := Timeout

	pollErr := wait.PollImmediate(Poll, timeout,
		func() (bool, error) {
			select {
			case <-ctx.Done():
				t.Log("Stopping pvc wait polling")
				return true, fmt.Errorf("stopped waiting to be bound")
			default:
				break
			}
			pvcList := &v1.PersistentVolumeClaimList{}
			err := k8sClient.List(context.Background(), pvcList)
			if err != nil {
				return false, err
			}
			for _, pvc := range pvcList.Items {
				if pvc.Status.Phase != v1.ClaimBound {
					t.Logf("PVC %s is still not bound", pvc.Name)
					return false, nil
				}
			}
			return true, nil
		})
	if pollErr != nil {
		return pollErr
	}
	yellow := color.New(color.FgHiYellow)
	t.Logf("All PVCs in %s are bound", yellow.Sprint(time.Since(startTime)))
	return nil
}

// StopMockServer terminates mock grpc server
func StopMockServer() {
	// terminate gracefully
	MockServer.GracefulStop()
	log.Info("Server stopped gracefully")
}

// MockUtils contains utils for mocking calls
type MockUtils struct {
	FakeClient           *fakeclient.Client
	Specs                Common
	FakeControllerClient client.Client
}

// GetLogger returns currently used logger
func GetLogger() logr.Logger {
	return log
}

func init() {
	log = zap.New(zap.UseDevMode(false))
}
