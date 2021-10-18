package e2e_framework_test

import (
	"context"
	"sync"
	"time"

	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	controller "github.com/dell/csm-replication/controllers/csi-replicator"
	replicationController "github.com/dell/csm-replication/controllers/replication-controller"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csm-replication/pkg/connection"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/singleflight"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"

	"testing"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CSIReplicator struct {
	PVCReconciler *controller.PersistentVolumeClaimReconciler
	PVReconciler  *controller.PersistentVolumeReconciler
	RGReconciler  *controller.ReplicationGroupReconciler
}

type ReplicationController struct {
	PVCReconciler *replicationController.PersistentVolumeClaimReconciler
	PVReconciler  *replicationController.PersistentVolumeReconciler
	RGReconciler  *replicationController.ReplicationGroupReconciler
}

type FakeReplicationTestSuite struct {
	suite.Suite
	client                client.Client
	driver                utils.Driver
	multiClusterClient    connection.MultiClusterClient
	csiReplicator         *CSIReplicator
	replicationController *ReplicationController
}

func (suite *FakeReplicationTestSuite) SetupTest() {
	suite.Init()
}

func (suite *FakeReplicationTestSuite) Init() {
	suite.driver = utils.Driver{
		DriverName:      utils.FakeDriverName,
		StorageClass:    utils.FakeSCName,
		RemoteClusterID: utils.RemoteClusterID,
		RemoteSCName:    utils.FakeRemoteSCName,
		Namespace:       utils.FakeNamespaceName,
		SourceClusterID: utils.SourceClusterID,
	}
	suite.client = utils.GetFakeClient()
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	suite.runCSIReplicator()
}

func (suite *FakeReplicationTestSuite) runCSIReplicator() {
	fakeRecorder := record.NewFakeRecorder(100)
	mockReplicationClient := csireplication.NewFakeReplicationClient(utils.ContextPrefix)
	pvReconciler := &controller.PersistentVolumeReconciler{
		Client:            suite.client,
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:            utils.Scheme,
		EventRecorder:     fakeRecorder,
		DriverName:        suite.driver.DriverName,
		ReplicationClient: &mockReplicationClient,
		ContextPrefix:     utils.ContextPrefix,
		SingleFlightGroup: singleflight.Group{},
		Domain:            constants.DefaultDomain,
	}
	pvcReconciler := &controller.PersistentVolumeClaimReconciler{
		Client:            suite.client,
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:            utils.Scheme,
		EventRecorder:     fakeRecorder,
		DriverName:        suite.driver.DriverName,
		ReplicationClient: &mockReplicationClient,
		Domain:            constants.DefaultDomain,
	}
	rgReconciler := &controller.ReplicationGroupReconciler{
		Client:            suite.client,
		Log:               ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:            utils.Scheme,
		DriverName:        suite.driver.DriverName,
		EventRecorder:     fakeRecorder,
		ReplicationClient: &mockReplicationClient,
	}
	csiReplicator := CSIReplicator{
		PVCReconciler: pvcReconciler,
		PVReconciler:  pvReconciler,
		RGReconciler:  rgReconciler,
	}
	suite.csiReplicator = &csiReplicator
}

func (suite *FakeReplicationTestSuite) runReplicationController() {
	fakeRecorder := record.NewFakeRecorder(100)
	suite.multiClusterClient = config.NewFakeConfig(suite.driver.SourceClusterID, suite.driver.RemoteClusterID)
	pvReconciler := &replicationController.PersistentVolumeReconciler{
		Client:        suite.client,
		Log:           ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:        utils.Scheme,
		EventRecorder: fakeRecorder,
		Domain:        constants.DefaultDomain,
		Config:        suite.multiClusterClient,
	}
	pvcReconciler := &replicationController.PersistentVolumeClaimReconciler{
		Client:        suite.client,
		Log:           ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:        utils.Scheme,
		EventRecorder: fakeRecorder,
		Domain:        constants.DefaultDomain,
		Config:        suite.multiClusterClient,
	}
	rgReconciler := &replicationController.ReplicationGroupReconciler{
		Client:        suite.client,
		Log:           ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:        utils.Scheme,
		EventRecorder: fakeRecorder,
		Domain:        constants.DefaultDomain,
		Config:        suite.multiClusterClient,
	}
	repController := ReplicationController{
		PVCReconciler: pvcReconciler,
		PVReconciler:  pvReconciler,
		RGReconciler:  rgReconciler,
	}
	suite.replicationController = &repController
}

func TestFakeReplicationTestSuite(t *testing.T) {
	if testing.Short(){
		t.Skip("Skipping as requested by short flag")
	}
	testSuite := new(FakeReplicationTestSuite)
	suite.Run(t, testSuite)
	testSuite.TearDownTestSuite()
}

func (suite *FakeReplicationTestSuite) getTypicalSC(scName, remoteSCName, remoteClusterID string) *storagev1.StorageClass {
	parameters := map[string]string{
		"replication.storage.dell.com/isReplicationEnabled":   "true",
		"replication.storage.dell.com/remoteClusterID":        remoteClusterID,
		"replication.storage.dell.com/remoteStorageClassName": remoteSCName,
		"param": "value",
	}
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	storageClass := storagev1.StorageClass{
		ObjectMeta:    metav1.ObjectMeta{Name: scName},
		Provisioner:   suite.driver.DriverName,
		Parameters:    parameters,
		ReclaimPolicy: &reclaimPolicy,
	}
	return &storageClass
}

func (suite *FakeReplicationTestSuite) getTypicalPV() *corev1.PersistentVolume {
	persistentVolume := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.FakePVName,
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           suite.driver.DriverName,
					VolumeHandle:     "csivol-",
					FSType:           "ext4",
					VolumeAttributes: suite.getTypicalVolAttributes(),
				},
			},
			StorageClassName: suite.driver.StorageClass,
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	return &persistentVolume
}

func (suite *FakeReplicationTestSuite) getTypicalReconcileRequest(name, namespace string) reconcile.Request {
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}
	return req
}

func (suite *FakeReplicationTestSuite) getTypicalPVC() *corev1.PersistentVolumeClaim {
	pvc := utils.GetPVCObj(utils.FakePVCName, suite.driver.Namespace, suite.driver.StorageClass)
	pvc.Status.Phase = corev1.ClaimBound
	pvc.Spec.VolumeName = utils.FakePVName
	return pvc
}

func (suite *FakeReplicationTestSuite) getTypicalVolAttributes() map[string]string {
	//creating fake PV to use with our fake PVC
	volumeAttributes := map[string]string{
		"CapacityGB": "3.00",
		"param1":     "val1",
		"storage.kubernetes.io/csiProvisionerIdentity": "1611934095007-8081-csi-fake",
	}
	return volumeAttributes
}

func (suite *FakeReplicationTestSuite) initObjects() {
	pv := suite.getTypicalPV()
	err := suite.client.Create(context.Background(), pv)
	suite.NoError(err)
	sc := suite.getTypicalSC(suite.driver.StorageClass, suite.driver.RemoteSCName, suite.driver.RemoteClusterID)
	err = suite.client.Create(context.Background(), sc)
	suite.NoError(err)
	pvc := suite.getTypicalPVC()
	err = suite.client.Create(context.Background(), pvc)
	suite.NoError(err)
}

func (suite *FakeReplicationTestSuite) testCreatePVC() {
	suite.initObjects()
	// Get Storage Class details
	sc := storagev1.StorageClass{}
	scName := types.NamespacedName{
		Namespace: "",
		Name:      suite.driver.StorageClass,
	}
	err := suite.client.Get(context.Background(), scName, &sc)
	suite.NoError(err)
	pvcReq := suite.getTypicalReconcileRequest(utils.FakePVCName, suite.driver.Namespace)
	// Get the object
	pvc := corev1.PersistentVolumeClaim{}
	err = suite.client.Get(context.Background(), pvcReq.NamespacedName, &pvc)
	suite.NoError(err)
	pvReq := suite.getTypicalReconcileRequest(utils.FakePVName, "")
	_, err = suite.csiReplicator.PVReconciler.Reconcile(context.Background(), pvReq)
	suite.NoError(err)
	_, err = suite.csiReplicator.PVCReconciler.Reconcile(context.Background(), pvcReq)
	suite.NoError(err)
	err = suite.client.Get(context.Background(), pvcReq.NamespacedName, &pvc)
	// Check if annotations have been applied on PVC
	annotations := pvc.Annotations
	// PVC Protection
	suite.Equal("yes", annotations[controllers.PVCProtectionComplete], "PVC Protection annotation set")
	// Remote Cluster ID
	suite.Equal(annotations[controllers.RemoteClusterID],
		sc.Parameters[controllers.RemoteClusterID], "Remote cluster ID matches")
	// Remote SC Name
	suite.Equal(annotations[controllers.RemoteStorageClassAnnotation],
		sc.Parameters[controllers.RemoteStorageClassAnnotation], "Remote SC annotation matches")
	// Find out RG name
	rgReq := suite.getTypicalReconcileRequest(annotations[controllers.ReplicationGroup], "")
	// Fetch RG details
	rg := storagev1alpha1.DellCSIReplicationGroup{}
	err = suite.client.Get(context.Background(), rgReq.NamespacedName, &rg)
	suite.NoError(err)
	// Lets reconcile the RG to see if it goes to Ready state
	_, err = suite.csiReplicator.RGReconciler.Reconcile(context.Background(), rgReq)
	suite.NoError(err)
	// If no error, the RG should be in Ready state
	err = suite.client.Get(context.Background(), rgReq.NamespacedName, &rg)
	suite.NoError(err)
	suite.Equal(controller.ReadyState, rg.Status.State)
}

func (suite *FakeReplicationTestSuite) runPVReconcile(pvName string, count int, wg *sync.WaitGroup, shouldFail bool) {
	pvReq := suite.getTypicalReconcileRequest(pvName, "")
	finalFailure := false
	for i := 0; i < count; i++ {
		_, err := suite.replicationController.PVReconciler.Reconcile(context.Background(), pvReq)
		if err != nil {
			finalFailure = true
		}
		sleepDuration := rand.IntnRange(100, 300)
		time.Sleep(time.Duration(sleepDuration) * time.Millisecond)
		wg.Done()
	}
	suite.Equal(shouldFail, finalFailure)
}

func (suite *FakeReplicationTestSuite) runRGReconcile(rgName string, count int, wg *sync.WaitGroup) {
	rgReq := suite.getTypicalReconcileRequest(rgName, "")
	for i := 0; i < count; i++ {
		_, err := suite.replicationController.RGReconciler.Reconcile(context.Background(), rgReq)
		suite.NoError(err)
		sleepDuration := rand.IntnRange(100, 300)
		time.Sleep(time.Duration(sleepDuration) * time.Millisecond)
		wg.Done()
	}
}

func (suite *FakeReplicationTestSuite) testSyncPV(wg *sync.WaitGroup, count int, shouldFail bool) {
	wg.Add(count)
	go suite.runPVReconcile(utils.FakePVName, count, wg, shouldFail)
}

func (suite *FakeReplicationTestSuite) testSyncAllObjects() {
	pvReq := suite.getTypicalReconcileRequest(utils.FakePVName, "")
	pv := corev1.PersistentVolume{}
	err := suite.client.Get(context.Background(), pvReq.NamespacedName, &pv)
	suite.NoError(err)
	annotations := pv.Annotations
	rgName := annotations[controllers.ReplicationGroup]
	var wg sync.WaitGroup
	n := 3
	wg.Add(n)
	go suite.runPVReconcile(utils.FakePVName, n, &wg, false)
	wg.Add(n)
	go suite.runRGReconcile(rgName, n, &wg)
	wg.Wait()
	// Check if RG object is updated with the remote RG name
	// Fetch RG details
	rg := storagev1alpha1.DellCSIReplicationGroup{}
	rgReq := suite.getTypicalReconcileRequest(rgName, "")
	err = suite.client.Get(context.Background(), rgReq.NamespacedName, &rg)
	suite.NoError(err)
	// Remote RG name should be set
	suite.Equal(rgName, rg.Annotations[controllers.RemoteReplicationGroup], "Remote RG name set")
	err = suite.client.Get(context.Background(), pvReq.NamespacedName, &pv)
	suite.NoError(err)
	// PV Sync complete annotation should be set
	suite.Equal("yes", pv.Annotations[controllers.PVSyncComplete], "PV Sync complete")
	// Get remote PV Name
	remotePVName := pv.Annotations[controllers.RemotePV]
	suite.NotEqual("", remotePVName)
	rClient, err := suite.multiClusterClient.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	remotePV, err := rClient.GetPersistentVolume(context.Background(), remotePVName)
	suite.NoError(err)
	suite.Equal(remotePV.Annotations[controllers.RemotePV], pv.Name)
	suite.Equal(remotePV.Labels[controllers.ReplicationGroup], rgName)
}

func (suite *FakeReplicationTestSuite) TestSideCarCreatePVC() {
	suite.testCreatePVC()
}

func (suite *FakeReplicationTestSuite) TestE2ECreatePVC() {
	suite.testCreatePVC()
	suite.runReplicationController()
	// First create the remote SC
	conn, err := suite.multiClusterClient.GetConnection(suite.driver.RemoteClusterID)
	suite.NoError(err)
	sc := suite.getTypicalSC(suite.driver.RemoteSCName, suite.driver.StorageClass, suite.driver.SourceClusterID)
	err = conn.CreateStorageClass(context.Background(), sc)
	assert.NoError(suite.T(), err)
	suite.testSyncAllObjects()
}

func (suite *FakeReplicationTestSuite) TestE2ESyncPVWithoutRemoteSC() {
	suite.testCreatePVC()
	suite.runReplicationController()
	var wg sync.WaitGroup
	count := 5 // Reconcile 5 times
	wg.Add(count)
	suite.testSyncPV(&wg, count, true)
}

func (suite *FakeReplicationTestSuite) TearDownTestSuite() {
	suite.T().Log("Cleaning up resources...")
}
