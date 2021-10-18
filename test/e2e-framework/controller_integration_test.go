package e2e_framework_test

import (
	"context"
	"github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

const LocalRg = "test-rg"

// ReplicationControllerTestSuite - Test suite for Replication controller
type ReplicationControllerTestSuite struct {
	suite.Suite
	driver    utils.Driver
	realUtils *RealControllerUtils
}

// RealControllerUtils - Real utilities for Controller tests
type RealControllerUtils struct {
	KubernetesClient       client.Client
	specs                  utils.Common
	RemoteKubernetesClient client.Client
}

func (suite *ReplicationControllerTestSuite) Init() {
	utils.InitializeSchemes()
	//Setting driver configurations, update here to run against any driver
	suite.driver = utils.Driver{
		DriverName:   "hostpath.csi.k8s.io",
		StorageClass: "hostpath-del",
	}
	suite.T().Log("Connecting to remote cluster")
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	suite.connectToRemoteCluster()
	// create Local resources
	err := suite.createLocalPVC()
	assert.NoError(suite.T(), err, "Local PVC created successfully")

	pvc, err := suite.getLocalPVC()
	assert.NoError(suite.T(), err)
	err = suite.updateLocalPV(pvc.Spec.VolumeName)

	err = suite.createLocalRG()
	assert.NoError(suite.T(), err, "Local RG created successfully")

	// wait for few seconds for the replicated volume and RG to be created at the target
	time.Sleep(15 * time.Second)
}

func TestReplicationControllerTestSuite(t *testing.T) {
	if testing.Short(){
		t.Skip("Skipping as requested by short flag")
	}
	testSuite := new(ReplicationControllerTestSuite)
	suite.Run(t, testSuite)
	testSuite.TearDownTestSuite()
}

func (suite *ReplicationControllerTestSuite) SetupSuite() {
	suite.Init()
	suite.runReplicationManager()
}

func (suite *ReplicationControllerTestSuite) runReplicationManager() {
	// TODO - Add replication controller running from here
}

func (suite *ReplicationControllerTestSuite) TestReplicatedVolume() {
	pvc, err := suite.getLocalPVC()
	assert.NoError(suite.T(), err, "Fetched local PVC Successfully")

	// Validate Remote PV by fetching it from the remote cluster
	remotePV := &v1.PersistentVolume{}
	err = suite.realUtils.RemoteKubernetesClient.Get(context.Background(), types.NamespacedName{
		Name: pvc.Spec.VolumeName,
	}, remotePV)
	assert.NoError(suite.T(), err, "Fetched remote PV Successfully")
	suite.T().Logf("Annotations: %v", remotePV.Annotations)
	utils.ValidateRemotePVAnnotations(remotePV.Annotations, suite.T())
}

func (suite *ReplicationControllerTestSuite) TestReplicatedRG() {
	pvc, err := suite.getLocalPVC()
	assert.NoError(suite.T(), err, "Fetched local PVC Successfully")

	// Validate Remote RG by fetching it from the remote cluster
	remoteRg := &v1alpha1.DellCSIReplicationGroup{}
	err = suite.realUtils.RemoteKubernetesClient.Get(context.Background(), types.NamespacedName{
		Name: pvc.Annotations[controllers.ReplicationGroup],
	}, remoteRg)
	assert.NoError(suite.T(), err, "Fetched remote RG successfully")
	utils.ValidateRemoteRGAnnotations(remoteRg.Annotations, suite.T())
}

func (suite *ReplicationControllerTestSuite) getLocalPVC() (*v1.PersistentVolumeClaim, error) {
	// Get Local PVC to validate at the remote site
	pvc := &v1.PersistentVolumeClaim{}
	err := suite.realUtils.KubernetesClient.Get(context.Background(),
		types.NamespacedName{
			Name:      utils.PVCName,
			Namespace: "default",
		}, pvc)
	return pvc, err
}

// createLocalPVC - creates a new local PVC
func (suite *ReplicationControllerTestSuite) createLocalPVC() error {
	ctx := context.Background()
	pvcObj := &v1.PersistentVolumeClaim{}
	err := suite.realUtils.KubernetesClient.Get(ctx, types.NamespacedName{
		Name:      utils.PVCName,
		Namespace: suite.realUtils.specs.Namespace,
	}, pvcObj)

	if err != nil && errors.IsNotFound(err) {
		pvcObj = utils.GetPVCObj(utils.PVCName, suite.realUtils.specs.Namespace, suite.driver.StorageClass)
		// Adding annotations
		annotations := make(map[string]string)
		annotations[controllers.PVCProtectionComplete] = "yes"
		annotations[controllers.RemoteClusterID] = "lglap066"
		annotations[controllers.RemoteStorageClassAnnotation] = suite.driver.StorageClass
		annotations[controllers.RemoteVolumeAnnotation] = `{
			"capacity_bytes": 30023,
				"volume_id": "pvc-d559bbfa-6612-4b57-a542-5ca64c9625fe",
				"volume_context": {
				"RdfGroup": "2",
					"RdfMode": "ASYNC",
					"RemoteRDFGroup": "2",
					"RemoteSYMID": "000000000002",
					"RemoteServiceLevel": "Bronze",
					"SRP": "SRP_1",
					"SYMID": "000000000001",
					"ServiceLevel": "Bronze",
					"replication.storage.dell.com/remotePVRetentionPolicy": "delete",
					"replication.storage.dell.com/isReplicationEnabled": "true",
					"replication.storage.dell.com/remoteClusterID": "target",
					"replication.storage.dell.com/remoteStorageClassName": "hostpath-del"
			}
		}`
		annotations[controllers.ReplicationGroup] = LocalRg
		pvcObj.SetAnnotations(annotations)

		//Adding labels
		labels := make(map[string]string)
		labels[controllers.RemoteClusterID] = "lglap066"
		labels[controllers.DriverName] = suite.driver.DriverName
		labels[controllers.ReplicationGroup] = LocalRg
		pvcObj.SetLabels(labels)

		err = suite.realUtils.KubernetesClient.Create(ctx, pvcObj)
		err = utils.WaitForAllToBeBound(ctx, suite.realUtils.KubernetesClient, suite.T())
	}
	return err
}

// createLocalRG - creates a new local RG
func (suite *ReplicationControllerTestSuite) createLocalRG() error {
	ctx := context.Background()
	annotations := make(map[string]string)
	annotations[controllers.RemoteClusterID] = "lglap066"
	attributes := map[string]string{
		"RdfGroup":                               "2",
		"RdfMode":                                "ASYNC",
		"RemoteRDFGroup":                         "2",
		"RemoteSYMID":                            "000000000002",
		"RemoteServiceLevel":                     "Bronze",
		"SRP":                                    "SRP_1",
		"SYMID":                                  "000000000001",
		"ServiceLevel":                           "Bronze",
		controllers.StorageClassReplicationParam: controllers.StorageClassReplicationParamEnabledValue,
		controllers.RemoteClusterID:              "lglap066",
		controllers.RemotePVRetentionPolicy:      controllers.RemoteRetentionValueDelete,
		controllers.RemoteStorageClassAnnotation: suite.driver.StorageClass,
	}

	newRG := v1alpha1.DellCSIReplicationGroup{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: LocalRg,
			Labels: map[string]string{
				controllers.DriverName:      suite.driver.DriverName,
				controllers.RemoteClusterID: "lglap066",
			},
			Annotations: annotations,
		},
		Spec: v1alpha1.DellCSIReplicationGroupSpec{
			DriverName:                      suite.driver.DriverName,
			Action:                          "",
			RemoteClusterID:                 "lglap066",
			ProtectionGroupID:               "l-group-id-1",
			ProtectionGroupAttributes:       attributes,
			RemoteProtectionGroupID:         "r-group-id-1",
			RemoteProtectionGroupAttributes: attributes,
		},
	}

	err := suite.realUtils.KubernetesClient.Get(ctx, types.NamespacedName{
		Name: LocalRg,
	}, &newRG)

	if err != nil {
		err := suite.realUtils.KubernetesClient.Create(ctx, &newRG)
		if err != nil {
			return err
		}
	}

	err = suite.realUtils.KubernetesClient.Get(ctx, types.NamespacedName{
		Name: LocalRg,
	}, &newRG)

	newRG.Status = v1alpha1.DellCSIReplicationGroupStatus{
		State:                "Created",
		RemoteState:          "",
		ReplicationLinkState: v1alpha1.ReplicationLinkState{},
		LastAction:           v1alpha1.LastAction{},
		Conditions:           nil,
	}
	err = suite.realUtils.KubernetesClient.Status().Update(ctx, &newRG)
	return nil
}

func (suite *ReplicationControllerTestSuite) updateLocalPV(name string) error {
	ctx := context.Background()
	pv := v1.PersistentVolume{}
	err := suite.realUtils.KubernetesClient.Get(ctx, types.NamespacedName{
		Name: name,
	}, &pv)

	// Adding annotations
	annotations := make(map[string]string)
	annotations[controllers.PVProtectionComplete] = "yes"
	annotations[controllers.RemotePVRetentionPolicy] = controllers.RemoteRetentionValueDelete
	annotations[controllers.RemoteClusterID] = "xyz066"
	annotations[controllers.RemoteStorageClassAnnotation] = suite.driver.StorageClass
	annotations[controllers.RemoteVolumeAnnotation] = `{
	"capacity_bytes": 30023,
	"volume_id": "pvc-d559bbfa-6612-4b57-a542-5ca64c9625fe",
	"volume_context": {
		"RdfGroup": "2",
		"RdfMode": "ASYNC",
		"RemoteRDFGroup": "2",
		"RemoteSYMID": "000000000002",
		"RemoteServiceLevel": "Bronze",
		"SRP": "SRP_1",
		"SYMID": "000000000001",
		"ServiceLevel": "Bronze",
		"replication.storage.dell.com/remotePVRetentionPolicy": "delete",
		"replication.storage.dell.com/isReplicationEnabled": "true",
		"replication.storage.dell.com/remoteClusterID": "target",
		"replication.storage.dell.com/remoteStorageClassName": "hostpath-del"
	}
}`
	annotations[controllers.ReplicationGroup] = LocalRg
	pv.SetAnnotations(annotations)

	//Adding labels
	labels := make(map[string]string)
	labels[controllers.RemoteClusterID] = "lglap066"
	labels[controllers.DriverName] = suite.driver.DriverName
	labels[controllers.ReplicationGroup] = LocalRg
	pv.SetLabels(labels)
	err = suite.realUtils.KubernetesClient.Update(ctx, &pv)
	return err
}

func (suite *ReplicationControllerTestSuite) connectToRemoteCluster() {
	// Loading config
	config, err := utils.GetConfigFromFile("")
	if err != nil {
		suite.T().Error(err)
	}

	err = v1alpha1.AddToScheme(scheme.Scheme)

	// Connecting to host and creating new Kubernetes Client
	kubeClient, kubeErr := client.New(config, client.Options{Scheme: scheme.Scheme})
	if kubeErr != nil {
		suite.T().Errorf("Couldn't create new local kubernetes client. Error = %v", kubeErr)
	}

	// Loading remote config
	filePath := os.Getenv("REMOTE_KUBE_CONFIG_PATH")
	remoteConfig, err := utils.GetConfigFromFile(filePath)
	if err != nil {
		suite.T().Error(err)
	}

	// Connecting to remote host and creating new Kubernetes Client
	remoteKubeClient, kubeErr := client.New(remoteConfig, client.Options{Scheme: scheme.Scheme})
	if kubeErr != nil {
		suite.T().Errorf("Couldn't create new kubernetes client. Error = %v", kubeErr)
	}

	suite.realUtils = &RealControllerUtils{
		KubernetesClient:       kubeClient,
		specs:                  utils.Common{Namespace: "default"},
		RemoteKubernetesClient: remoteKubeClient,
	}
}

func (suite *ReplicationControllerTestSuite) TearDownTestSuite() {
	suite.T().Log("Cleaning up resources...")
	if suite.realUtils != nil {
		// Clean up local PVC
		pvc := new(v1.PersistentVolumeClaim)
		pvc.Name = utils.PVCName

		err := suite.realUtils.KubernetesClient.Get(context.Background(), types.NamespacedName{Name: pvc.Name, Namespace: "default"}, pvc)
		pv := new(v1.PersistentVolume)
		pv.Name = pvc.Spec.VolumeName

		err = suite.realUtils.KubernetesClient.Delete(context.Background(), pvc)
		if err != nil {
			suite.T().Log("Failed to clean up local PVC")
		}

		// Clean up local RG
		rg := new(v1alpha1.DellCSIReplicationGroup)
		rg.Name = pvc.Annotations[controllers.ReplicationGroup]
		err = suite.realUtils.KubernetesClient.Delete(context.Background(), rg)
		if err != nil {
			suite.T().Log("Failed to clean clean up local RG")
		}

		// Clean up remote PV
		err = suite.realUtils.RemoteKubernetesClient.Delete(context.Background(), pv)
		if err != nil {
			suite.T().Log("Failed to clean clean up remote PV")
		}

		// Clean up remote RG
		remoteRG := new(v1alpha1.DellCSIReplicationGroup)
		remoteRG.Name = pvc.Annotations[controllers.ReplicationGroup]
		err = suite.realUtils.RemoteKubernetesClient.Delete(context.Background(), remoteRG)
		if err != nil {
			suite.T().Log("Failed to clean clean up remote RG")
		}
	}
}
