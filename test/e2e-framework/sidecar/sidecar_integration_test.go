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

package sidecar_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/test/e2e-framework/utils"

	"k8s.io/apimachinery/pkg/api/errors"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/client-go/util/workqueue"

	"golang.org/x/sync/singleflight"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/dell/dell-csi-extensions/replication"

	"google.golang.org/grpc"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/dell/csm-replication/controllers"
	controller "github.com/dell/csm-replication/controllers/csi-replicator"
	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/connection"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"
	"github.com/stretchr/testify/suite"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	storageV1 "k8s.io/api/storage/v1"
)

const (
	defaultNamespace          = "test"
	defaultTestTimeout        = 15 * time.Minute
	defaultTimeout            = 180 * time.Second
	defaultProbeTimeout       = 10 * time.Second
	defaultDomain             = "replication.storage.dell.com"
	defaultWorkers            = 10
	defaultRetryStartInterval = time.Second
	defaultRetryMaxInterval   = 5 * time.Minute
)

var (
	scheme               = runtime.NewScheme()
	log                  = ctrl.Log.WithName("sidecar-integration-test")
	requiredCapabilities = []replication.ReplicationCapability_RPC_Type{
		replication.ReplicationCapability_RPC_CREATE_REMOTE_VOLUME,
		replication.ReplicationCapability_RPC_CREATE_PROTECTION_GROUP,
	}
)

type SidecarTestSuite struct {
	suite.Suite
	client                  client.Client
	ctx                     context.Context
	ctxCancel               context.CancelFunc
	csiConn                 *grpc.ClientConn
	driverName              string
	replicationCapabilities csiidentity.ReplicationCapabilitySet
	supportedActions        []*replication.SupportedActions
	manager                 manager.Manager
	rateLimiter             workqueue.TypedRateLimiter[reconcile.Request]
	workers                 int
	timeout                 time.Duration
	testTimeout             time.Duration
	domain                  string
	contextPrefix           string
	storageClass            string
	claim                   *v1.PersistentVolumeClaim
	namespace               string
}

func (ss *SidecarTestSuite) randomSuffix() (string, error) {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b[0:]), nil
}

func (ss *SidecarTestSuite) readSidecarParams() error {
	ss.timeout = defaultTimeout
	if timeoutStr, ok := os.LookupEnv("TIMEOUT"); ok {
		timeout, err := time.ParseDuration(timeoutStr)
		if err == nil {
			ss.timeout = timeout
		}
	}

	ss.testTimeout = defaultTestTimeout
	if testTimeoutStr, ok := os.LookupEnv("TEST_TIMEOUT"); ok {
		timeout, err := time.ParseDuration(testTimeoutStr)
		if err == nil {
			ss.testTimeout = timeout
		}
	}

	ss.domain = defaultDomain
	if domain, ok := os.LookupEnv("DOMAIN"); ok {
		ss.domain = domain
	}

	controllers.InitLabelsAndAnnotations(ss.domain)

	if contextPrefix, ok := os.LookupEnv("CONTEXT_PREFIX"); ok {
		ss.contextPrefix = contextPrefix
	}

	ss.workers = defaultWorkers
	if workers, ok := os.LookupEnv("WORKERS"); ok {
		w, err := strconv.ParseInt(workers, 10, 0)
		if err == nil {
			ss.workers = int(w)
		}
	}

	ss.namespace = defaultNamespace
	if ns, ok := os.LookupEnv("NAMESPACE"); ok {
		ss.namespace = ns
	}

	if storageClass, ok := os.LookupEnv("STORAGE_CLASS"); ok {
		ss.storageClass = storageClass
	} else {
		return fmt.Errorf("storage-class not found")
	}
	return nil
}

func (ss *SidecarTestSuite) registerSchemes() error {
	err := clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return err
	}
	return repv1.AddToScheme(scheme)
}

func (ss *SidecarTestSuite) createCSIConnection() error {
	csiAddress, ok := os.LookupEnv("CSI_ADDRESS")
	if !ok {
		return fmt.Errorf("csi_address not found")
	}
	var err error
	ss.csiConn, err = connection.Connect(csiAddress, utils.GetLogger())
	return err
}

func (ss *SidecarTestSuite) identifyDriverAndCapabilities() error {
	probeTimeout := defaultProbeTimeout
	if timeoutStr, ok := os.LookupEnv("PROBE_TIMEOUT"); ok {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			probeTimeout = timeout
		}
	}
	client := csiidentity.New(ss.csiConn, log.WithName("identity-client"), ss.timeout, probeTimeout)
	var err error
	ss.driverName, err = client.ProbeForever(context.Background())
	if err != nil {
		return err
	}
	ss.replicationCapabilities, ss.supportedActions, err = client.GetReplicationCapabilities(context.Background())
	if err != nil {
		return err
	}
	for _, capability := range requiredCapabilities {
		if _, ok := ss.replicationCapabilities[capability]; !ok {
			return fmt.Errorf("missing %s capability, which is required", capability)
		}
	}
	return nil
}

func (ss *SidecarTestSuite) getConfigOrDie() *rest.Config {
	kubeConfigPath, ok := os.LookupEnv("KUBECONFIG_PATH")
	if !ok {
		panic("no config path set")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		panic(err)
	}
	return config
}

func (ss *SidecarTestSuite) createManager() error {
	var err error
	ss.manager, err = ctrl.NewManager(ss.getConfigOrDie(), ctrl.Options{Scheme: scheme})
	return err
}

func (ss *SidecarTestSuite) createRateLimiter() {
	startInterval, maxInterval := defaultRetryStartInterval, defaultRetryMaxInterval
	if startStr, ok := os.LookupEnv("RETRY_START_INTERVAL"); ok {
		start, err := time.ParseDuration(startStr)
		if err == nil {
			startInterval = start
		}
	}
	if maxStr, ok := os.LookupEnv("RETRY_MAX_INTERVAL"); ok {
		max, err := time.ParseDuration(maxStr)
		if err == nil {
			maxInterval = max
		}
	}
	ss.rateLimiter = workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](startInterval, maxInterval)
}

func (ss *SidecarTestSuite) createClaimController() error {
	return (&controller.PersistentVolumeClaimReconciler{
		Client:            ss.manager.GetClient(),
		Log:               log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:            ss.manager.GetScheme(),
		EventRecorder:     ss.manager.GetEventRecorderFor(common.DellCSIReplicator),
		SingleFlightGroup: singleflight.Group{},
		ReplicationClient: csireplication.New(ss.csiConn, log.WithName("replication-client"), ss.timeout),
		DriverName:        ss.driverName,
		ContextPrefix:     ss.contextPrefix,
		Domain:            ss.domain,
	}).SetupWithManager(ss.manager, ss.rateLimiter, ss.workers)
}

func (ss *SidecarTestSuite) createVolumeController() error {
	return (&controller.PersistentVolumeReconciler{
		Client:            ss.manager.GetClient(),
		Log:               log.WithName("controllers").WithName("PersistentVolume"),
		Scheme:            ss.manager.GetScheme(),
		SingleFlightGroup: singleflight.Group{},
		EventRecorder:     ss.manager.GetEventRecorderFor(common.DellCSIReplicator),
		ReplicationClient: csireplication.New(ss.csiConn, log.WithName("replication-client"), ss.timeout),
		Domain:            ss.domain,
		DriverName:        ss.driverName,
		ContextPrefix:     ss.contextPrefix,
	}).SetupWithManager(ss.ctx, ss.manager, ss.rateLimiter, ss.workers)
}

func (ss *SidecarTestSuite) createReplicationGroupController() error {
	return (&controller.ReplicationGroupReconciler{
		Client:                     ss.manager.GetClient(),
		Scheme:                     ss.manager.GetScheme(),
		EventRecorder:              ss.manager.GetEventRecorderFor(common.DellCSIReplicator),
		Log:                        log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		SupportedActions:           ss.supportedActions,
		DriverName:                 ss.driverName,
		ReplicationClient:          csireplication.New(ss.csiConn, log.WithName("replication-client"), ss.timeout),
		MaxRetryDurationForActions: ss.timeout, // TODO: Need to check if set to correct value
	}).SetupWithManager(ss.manager, ss.rateLimiter, ss.workers)
}

func (ss *SidecarTestSuite) runManager() {
	ss.ctx, ss.ctxCancel = signal.NotifyContext(context.Background(), []os.Signal{os.Interrupt, os.Kill}...)
	go func() {
		err := ss.manager.Start(ss.ctx)
		if err != nil {
			log.Error(err, "Manager stopped due to an error")
			ss.ctxCancel()
		} else {
			log.Info("Manager stop due to context cancellation")
		}
	}()
}

func (ss *SidecarTestSuite) verifyStorageClass() error {
	sc := new(storageV1.StorageClass)
	if err := ss.client.Get(ss.ctx, client.ObjectKey{Name: ss.storageClass}, sc); err != nil {
		return err
	}
	if val, ok := sc.Parameters[ss.domain+"/isReplicationEnabled"]; !ok || val != "true" {
		return fmt.Errorf("speficified storage class, %s, is not replication enabled", ss.storageClass)
	}
	return nil
}

func (ss *SidecarTestSuite) createClient() error {
	var err error
	ss.client, err = client.New(ss.getConfigOrDie(), client.Options{
		Scheme: ss.manager.GetScheme(),
		Mapper: ss.manager.GetRESTMapper(),
	})
	return err
}

func (ss *SidecarTestSuite) createTestNamespaceIfNotExist() error {
	claimNamespaceSuffix, err := ss.randomSuffix()
	if err != nil {
		return err
	}
	claimNamespace := fmt.Sprintf("test-%s", claimNamespaceSuffix)
	// Check if the namespace exists; create if it doesn't
	ns := new(v1.Namespace)
	err = ss.client.Get(ss.ctx, client.ObjectKey{Name: claimNamespace}, ns)
	if err != nil {
		if errors.IsNotFound(err) {
			ns := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: claimNamespace,
				},
			}
			return ss.client.Create(ss.ctx, ns)
		}
		return err
	}
	ss.namespace = ns.Name
	return nil
}

func (ss *SidecarTestSuite) SetupSuite() {
	// register schemes, and create managers and kubeclient
	utilruntime.Must(ss.readSidecarParams())
	utilruntime.Must(ss.registerSchemes())
	utilruntime.Must(ss.createCSIConnection())
	utilruntime.Must(ss.identifyDriverAndCapabilities())
	utilruntime.Must(ss.createManager())
	ss.createRateLimiter()
	utilruntime.Must(ss.createClaimController())
	utilruntime.Must(ss.createVolumeController())
	utilruntime.Must(ss.createReplicationGroupController())
	ss.runManager()
	utilruntime.Must(ss.createClient())
	utilruntime.Must(ss.verifyStorageClass())
	// utilruntime.Must(ss.createTestNamespaceIfNotExist())
}

func (ss *SidecarTestSuite) waitForPVCProtectionComplete(ctx context.Context, _ *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {
	errorCh := make(chan error)
	pvc := new(v1.PersistentVolumeClaim)
	go func() {
		for {
			err := ss.client.Get(ctx, client.ObjectKey{
				Name:      ss.claim.Name,
				Namespace: ss.namespace,
			}, pvc)
			if err != nil {
				if errors.IsNotFound(err) {
					errorCh <- err
					break
				}
			} else {
				if _, ok := pvc.Annotations[controllers.PVCProtectionComplete]; ok {
					close(errorCh)
					break
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()

	select {
	case <-ctx.Done():
		return pvc, fmt.Errorf("timeouted out waiting for the PVC to go to protection complete state")
	case err := <-errorCh:
		return pvc, err
	}
}

func (ss *SidecarTestSuite) TearDownSuite() {
	ss.client.Delete(ss.ctx, ss.claim)
	ss.ctxCancel()
}

func (ss *SidecarTestSuite) TestCreatePVC() {
	innerctx, cancel := context.WithTimeout(ss.ctx, ss.testTimeout)
	defer cancel()
	pvcNameSuffix, err := ss.randomSuffix()
	if err != nil {
		ss.FailNowf("PVC SUFFIX GENERATION FAILED", err.Error())
	}
	quantity, _ := resource.ParseQuantity("1Gi")
	filesystem := v1.PersistentVolumeFilesystem
	ss.claim = &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("pvc-%s", pvcNameSuffix),
			Namespace: ss.namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					"storage": quantity,
				},
			},
			VolumeMode:       &filesystem,
			StorageClassName: &ss.storageClass,
		},
	}

	err = ss.client.Create(ss.ctx, ss.claim)
	if err != nil {
		ss.FailNowf("PVC CREATION FAILED", err.Error())
	}

	ss.claim, err = ss.waitForPVCProtectionComplete(innerctx, ss.claim)
	if err != nil {
		ss.FailNowf("PROTECTION INCOMPLETE", err.Error())
	}
}

func TestSidecar(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping as requested by short flag")
	}
	suite.Run(t, new(SidecarTestSuite))
}
