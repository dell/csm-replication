package e2e_framework_test

import (
	"context"
	"os"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"

	"github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	controller "github.com/dell/csm-replication/controllers/csi-replicator"
	"github.com/dell/csm-replication/pkg/connection"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	ctrl "sigs.k8s.io/controller-runtime"

	constants "github.com/dell/csm-replication/pkg/common"
)

type ReplicationTestSuite struct {
	suite.Suite
	driver    utils.Driver
	realUtils *RealUtils
}

type RealUtils struct {
	KubernetesClient client.Client
	specs            utils.Common
}

func (suite *ReplicationTestSuite) Init() {
	utils.InitializeSchemes()
	// Setting driver configurations, update here to run against any driver
	suite.driver = utils.Driver{
		DriverName:   "hostpath.csi.k8s.io",
		StorageClass: "hostpath-del",
	}
	suite.T().Log("Connecting to remote cluster")
	suite.connectToCluster()
}

func (suite *ReplicationTestSuite) SetupSuite() {
	suite.Init()
	utils.RunServer("")
	suite.runReplicationManager()
}

func TestReplicationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping as requested by short flag")
	}
	testSuite := new(ReplicationTestSuite)
	suite.Run(t, testSuite)
	testSuite.TearDownTestSuite()
}

func (suite *ReplicationTestSuite) runReplicationManager() {
	// Connect to CSI
	csiConn, err := connection.Connect("localhost:4772", utils.GetLogger())
	if err != nil {
		suite.T().Error(err, "failed to connect to CSI driver")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             utils.Scheme,
		MetricsBindAddress: ":8080",
		Port:               9443,
		LeaderElection:     false,
		LeaderElectionID:   "bec0e175.replication.storage.dell.com",
	})
	if err != nil {
		suite.T().Error(err, "unable to start manager")
		os.Exit(1)
	}
	controllers.InitLabelsAndAnnotations(constants.DefaultDomain)
	expRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(5*time.Minute, 10*time.Minute)
	if err = (&controller.PersistentVolumeClaimReconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(constants.DellCSIReplicator),
		DriverName:        suite.driver.DriverName,
		ReplicationClient: csireplication.New(csiConn, ctrl.Log.WithName("replication-client"), 10*time.Second),
		Domain:            constants.DefaultDomain,
	}).SetupWithManager(mgr, expRateLimiter, 2); err != nil {
		suite.T().Error(err, "unable to create controller", "controller", "PersistentVolumeClaim")
		os.Exit(1)
	}

	if err = (&controller.ReplicationGroupReconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(constants.DellCSIReplicator),
		DriverName:        suite.driver.DriverName,
		ReplicationClient: csireplication.New(csiConn, ctrl.Log.WithName("replication-client"), 10*time.Second),
	}).SetupWithManager(mgr, expRateLimiter, 2); err != nil {
		suite.T().Error(err, "unable to create controller", "controller", "DellCSIReplicationGroup")
		os.Exit(1)
	}
	suite.T().Log("starting manager")
	errChan := make(chan error)

	// run blocking call in a separate goroutine, report errors via channel
	go func() {
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			suite.T().Error(err, "problem running manager")
			errChan <- err
		}
	}()
}

func (suite *ReplicationTestSuite) TestCreatePVC() {
	pvc, err := suite.createPVC()
	suite.T().Log("waiting for the reconciliation to complete")
	time.Sleep(10 * time.Second) //wait for few seconds for the reconciliation to happen

	//validate created PVC
	assert.NoError(suite.T(), err, "PVC Created successfully")
	assert.NotNil(suite.T(), pvc)

	assert.Equal(suite.T(), pvc.Name, utils.PVCName, "They should be equal")
	assert.Equal(suite.T(), pvc.Namespace, suite.realUtils.specs.Namespace, "They should be equal")

	// we will get PVC from cluster after reconciliation to validate for annotations and finalizers
	localPVC, err := suite.getLocalPVCFromCluster()
	assert.NoError(suite.T(), err, "Fetched PVC successfully")
	utils.ValidateAnnotations(localPVC.ObjectMeta.Annotations, suite.T())

	suite.performAction(controllers.FailOver, controller.ReadyState)
	// TODO - Add additional actions type once the integration with the driver is completed
}

// createPVC - creates a new PVC
func (suite *ReplicationTestSuite) createPVC() (*corev1.PersistentVolumeClaim, error) {
	ctx := context.Background()
	pvcObj, err := suite.getLocalPVCFromCluster()
	if err != nil && errors.IsNotFound(err) {
		pvcObj = utils.GetPVCObj(utils.PVCName, suite.realUtils.specs.Namespace, suite.driver.StorageClass)
		err = suite.realUtils.KubernetesClient.Create(ctx, pvcObj)
		err = utils.WaitForAllToBeBound(ctx, suite.realUtils.KubernetesClient, suite.T())
	}
	return pvcObj, err
}

func (suite *ReplicationTestSuite) connectToCluster() {
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

	suite.realUtils = &RealUtils{
		KubernetesClient: kubeClient,
		specs:            utils.Common{Namespace: "default"},
	}
}

func (suite *ReplicationTestSuite) performAction(action string, expectedState string) {
	pvc, err := suite.getLocalPVCFromCluster()
	assert.NoError(suite.T(), err, "Fetched PVC successfully")

	rg := &v1alpha1.DellCSIReplicationGroup{}
	err = suite.realUtils.KubernetesClient.Get(context.Background(), types.NamespacedName{
		Name: pvc.Annotations[controllers.ReplicationGroup],
	}, rg)
	assert.NoError(suite.T(), err, "Fetched RG successfully")

	// update the action
	rg.Spec.Action = action
	err = suite.realUtils.KubernetesClient.Update(context.Background(), rg)
	assert.NoError(suite.T(), err, "Action updated successfully")

	time.Sleep(10 * time.Second)

	//validate the RG for the updated action
	rg = &v1alpha1.DellCSIReplicationGroup{}
	err = suite.realUtils.KubernetesClient.Get(context.Background(), types.NamespacedName{
		Name: pvc.Annotations[controllers.ReplicationGroup],
	}, rg)
	assert.NoError(suite.T(), err, "Fetched RG successfully")
	assert.Equal(suite.T(), expectedState, rg.Status.State, "Successfully FailedOver")
}

func (suite *ReplicationTestSuite) getLocalPVCFromCluster() (*v1.PersistentVolumeClaim, error) {
	pvc := &v1.PersistentVolumeClaim{}
	err := suite.realUtils.KubernetesClient.Get(context.Background(),
		types.NamespacedName{
			Name:      utils.PVCName,
			Namespace: suite.realUtils.specs.Namespace,
		}, pvc)
	return pvc, err
}

func (suite *ReplicationTestSuite) TearDownTestSuite() {
	suite.T().Log("Cleaning up resources...")
	if suite.realUtils != nil {
		ctx := context.Background()
		pvc, err := suite.getLocalPVCFromCluster()
		assert.NoError(suite.T(), err, "Fetched PVC successfully")

		//Delete PVC
		err = suite.realUtils.KubernetesClient.Delete(ctx, pvc)
		assert.NoError(suite.T(), err, "PVC Deleted successfully")

		//Delete RG
		rg := &v1alpha1.DellCSIReplicationGroup{}
		err = suite.realUtils.KubernetesClient.Get(context.Background(), types.NamespacedName{
			Name: pvc.Annotations[controllers.ReplicationGroup],
		}, rg)

		// we need to remove finalizers to delete the RG
		rg.SetFinalizers([]string{})
		err = suite.realUtils.KubernetesClient.Update(ctx, rg)

		err = suite.realUtils.KubernetesClient.Delete(ctx, rg)
		assert.NoError(suite.T(), err, "RG Deleted successfully")
	}
	utils.StopMockServer()
}
