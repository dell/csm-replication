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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bombsimon/logrusr/v4"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"

	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"

	"golang.org/x/sync/singleflight"

	"github.com/dell/dell-csi-extensions/replication"

	controller "github.com/dell/csm-replication/controllers/csi-replicator"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/core"
	"github.com/dell/csm-replication/pkg/connection"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	scheme                       = runtime.NewScheme()
	setupLog                     = ctrl.Log.WithName("setup")
	currentSupportedCapabilities = []replication.ReplicationCapability_RPC_Type{
		replication.ReplicationCapability_RPC_CREATE_REMOTE_VOLUME,
		replication.ReplicationCapability_RPC_CREATE_PROTECTION_GROUP,
	}
	monitoringCapability = replication.ReplicationCapability_RPC_MONITOR_PROTECTION_GROUP
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(repv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// ReplicatorManager - Represents the controller manager and its configuration
type ReplicatorManager struct {
	Opts    config.ControllerManagerOpts
	Manager ctrl.Manager
	config  *config.Config
}

var getUpdateConfigMapFunc = func(mgr *ReplicatorManager, ctx context.Context) error {
	return mgr.config.UpdateConfigMap(ctx, nil, mgr.Opts, nil, mgr.Manager.GetLogger())
}

func (mgr *ReplicatorManager) processConfigMapChanges(loggerConfig *logrus.Logger) {
	log.Println("Received a config change event")
	err := getUpdateConfigMapFunc(mgr, context.Background())
	if err != nil {
		log.Printf("Error parsing the config: %v\n", err)
		return
	}
	mgr.config.Lock.Lock()
	defer mgr.config.Lock.Unlock()
	level, err := common.ParseLevel(mgr.config.LogLevel)
	if err != nil {
		log.Println("Unable to parse ", err)
	}
	log.Println("set level to", level)
	loggerConfig.SetLevel(level)
}

var (
	watchConfig    = viper.WatchConfig
	onConfigChange = viper.OnConfigChange
)

func (mgr *ReplicatorManager) setupConfigMapWatcher(loggerConfig *logrus.Logger) {
	log.Println("Started ConfigMap Watcher")
	watchConfig()
	onConfigChange(func(_ fsnotify.Event) {
		mgr.processConfigMapChanges(loggerConfig)
	})
}

var (
	getControllerManagerOpts = config.GetControllerManagerOpts
	getConfig                = config.GetConfig
)

func createReplicatorManager(ctx context.Context, mgr ctrl.Manager) (*ReplicatorManager, error) {
	opts := getControllerManagerOpts()
	opts.Mode = "sidecar"
	mgrLogger := mgr.GetLogger()
	repConfig, err := getConfig(ctx, nil, opts, nil, mgrLogger)
	if err != nil {
		return nil, err
	}

	controllerManager := ReplicatorManager{
		Opts:    opts,
		Manager: mgr,
		config:  repConfig,
	}
	return &controllerManager, nil
}

var getConnectToCsiFunc = func(csiAddress string, setupLog logr.Logger) (*grpc.ClientConn, error) {
	return connection.Connect(csiAddress, setupLog)
}

var getProbeForeverFunc = func(ctx context.Context, identityClient csiidentity.Identity) (string, error) {
	return identityClient.ProbeForever(ctx)
}

var getReplicationCapabilitiesFunc = func(ctx context.Context, identityClient csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
	return identityClient.GetReplicationCapabilities(ctx)
}

var getcreateReplicatorManagerFunc = func(ctx context.Context, mgr ctrl.Manager) (*ReplicatorManager, error) {
	return createReplicatorManager(ctx, mgr)
}

var getParseLevelFunc = func(level string) (logrus.Level, error) {
	return common.ParseLevel(level)
}

var getManagerStart = func(mgr manager.Manager) error {
	return mgr.Start(ctrl.SetupSignalHandler())
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;watch;list;delete;update;create

func main() {
	var (
		metricsAddr                string
		enableLeaderElection       bool
		csiAddress                 string
		workerThreads              int
		retryIntervalStart         time.Duration
		retryIntervalMax           time.Duration
		operationTimeout           time.Duration
		pgContextKeyPrefix         string
		domain                     string
		monitoringInterval         time.Duration
		probeFrequency             time.Duration
		maxRetryDurationForActions time.Duration
	)
	flag.StringVar(&metricsAddr, "metrics-addr", ":8000", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&csiAddress, "csi-address", "/var/run/csi.sock", "Address for the csi driver socket")
	flag.StringVar(&domain, "prefix", common.DefaultDomain, "Prefix used for creating labels/annotations")
	flag.IntVar(&workerThreads, "worker-threads", 2, "Number of concurrent reconcilers for each of the controllers")
	flag.DurationVar(&retryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed reconcile request. It doubles with each failure, upto retry-interval-max")
	flag.DurationVar(&retryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed reconcile request")
	flag.DurationVar(&operationTimeout, "timeout", 10*time.Second, "Timeout of waiting for response for CSI Driver")
	flag.StringVar(&pgContextKeyPrefix, "context-prefix", "", "All the protection-group-attribute-keys with this prefix are added as annotation to the DellCSIReplicationGroup")
	flag.DurationVar(&monitoringInterval, "monitoring-interval", 60*time.Second, "Time after which monitoring cycle runs")
	flag.DurationVar(&probeFrequency, "probe-frequency", 5*time.Second, "Time between identity ProbeController calls")
	flag.DurationVar(&maxRetryDurationForActions, "max-retry-action-duration", controller.MaxRetryDurationForActions,
		"Max duration after (since the first error encountered) which action won't be retried")
	flag.Parse()
	controllers.InitLabelsAndAnnotations(domain)
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})

	logger := logrusr.New(logrusLog)
	ctrl.SetLogger(logger)

	setupLog.V(1).Info("Prefix", "Domain", domain)
	setupLog.V(1).Info(common.DellCSIReplicator, "Version", core.SemVer, "Commit ID", core.CommitSha32, "Commit SHA", core.CommitTime.Format(time.RFC1123))

	ctx := context.Background()

	// Connect to csi
	csiConn, err := getConnectToCsiFunc(csiAddress, setupLog)
	if err != nil {
		setupLog.Error(err, "failed to connect to CSI driver")
		os.Exit(1)
	}

	identityClient := csiidentity.New(csiConn, ctrl.Log.WithName("identity-client"), operationTimeout, probeFrequency)

	driverName, err := getProbeForeverFunc(ctx, identityClient)
	if err != nil {
		setupLog.Error(err, "error waiting for the CSI driver to be ready")
		os.Exit(1)
	}
	setupLog.V(1).Info("CSI driver name", "driverName", driverName)

	capabilitySet, supportedActions, err := getReplicationCapabilitiesFunc(ctx, identityClient)
	if err != nil {
		setupLog.Error(err, "error fetching replication capabilities")
		os.Exit(1)
	}
	if len(capabilitySet) == 0 {
		setupLog.Error(fmt.Errorf("driver doesn't support replication"), "replication not supported")
		os.Exit(1)
	}
	for _, capability := range currentSupportedCapabilities {
		if _, ok := capabilitySet[capability]; !ok {
			setupLog.Error(fmt.Errorf("driver doesn't support %s capability, which is required", capability),
				"one of the capabilities not supported")
			os.Exit(1)
		}
	}

	leaderElectionID := common.DellCSIReplicator + strings.ReplaceAll(driverName, ".", "-")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsServer.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer:              webhook.NewServer(webhook.Options{Port: 9443}),
		LeaderElection:             enableLeaderElection,
		LeaderElectionResourceLock: "leases",
		LeaderElectionID:           leaderElectionID,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	controllerMgr, err := getcreateReplicatorManagerFunc(ctx, mgr)
	if err != nil {
		setupLog.Error(err, "failed to configure the controller manager")
		os.Exit(1)
	}
	// Start the watch on configmap
	controllerMgr.setupConfigMapWatcher(logrusLog)

	// Process the config. Get initial log level
	level, err := getParseLevelFunc(controllerMgr.config.LogLevel)
	if err != nil {
		log.Println("Unable to parse ", err)
	}
	log.Println("set level to", level)
	logrusLog.SetLevel(level)

	// Get the kube-system content
	var clusterUID string
	ns, err := getClusterUID(ctx)
	if err != nil {
		log.Println("getClusterUuid error: ", err.Error())
	} else {
		log.Println("getClusterUuid got uuid: ", ns.GetUID())
		clusterUID = string(ns.GetUID())
	}

	expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](retryIntervalStart, retryIntervalMax)
	if err = (&controller.PersistentVolumeClaimReconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(common.DellCSIReplicator),
		DriverName:        driverName,
		ReplicationClient: csireplication.New(csiConn, ctrl.Log.WithName("replication-client"), operationTimeout),
		ContextPrefix:     pgContextKeyPrefix,
		SingleFlightGroup: singleflight.Group{},
		Domain:            domain,
	}).SetupWithManager(mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PersistentVolumeClaim")
		os.Exit(1)
	}

	if err = (&controller.PersistentVolumeReconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolume"),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(common.DellCSIReplicator),
		DriverName:        driverName,
		ReplicationClient: csireplication.New(csiConn, ctrl.Log.WithName("replication-client"), operationTimeout),
		ContextPrefix:     pgContextKeyPrefix,
		SingleFlightGroup: singleflight.Group{},
		Domain:            domain,
		ClusterUID:        clusterUID,
	}).SetupWithManager(ctx, mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PersistentVolume")
		os.Exit(1)
	}

	if err = (&controller.ReplicationGroupReconciler{
		Client:                     mgr.GetClient(),
		Log:                        ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:                     mgr.GetScheme(),
		EventRecorder:              mgr.GetEventRecorderFor(common.DellCSIReplicator),
		DriverName:                 driverName,
		ReplicationClient:          csireplication.New(csiConn, ctrl.Log.WithName("replication-client"), operationTimeout),
		SupportedActions:           supportedActions,
		MaxRetryDurationForActions: maxRetryDurationForActions,
	}).SetupWithManager(mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DellCSIReplicationGroup")
		os.Exit(1)
	}

	if _, ok := capabilitySet[monitoringCapability]; ok {
		setupLog.Info("driver supports monitoring capability. we will monitor the RGs")
		rgMonitor := controller.ReplicationGroupMonitoring{
			Client:             mgr.GetClient(),
			Log:                ctrl.Log.WithName("controllers").WithName(common.Monitoring),
			EventRecorder:      mgr.GetEventRecorderFor(common.Monitoring),
			DriverName:         driverName,
			ReplicationClient:  csireplication.New(csiConn, ctrl.Log.WithName("replication-client"), operationTimeout),
			MonitoringInterval: monitoringInterval,
		}

		err = rgMonitor.Monitor(ctx)
		if err != nil {
			os.Exit(1)
		}
	} else {
		setupLog.Info("driver does not support monitoring capability")
	}

	setupLog.Info("starting manager")
	if err := getManagerStart(mgr); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

var (
	getControllerClient = connection.GetControllerClient
	kubeSystemNamespace = controllers.KubeSystemNamespace
)

func getClusterUID(ctx context.Context) (*v1.Namespace, error) {
	client, err := getControllerClient(nil, scheme)
	if err != nil {
		return nil, err
	}

	ns := new(v1.Namespace)
	err = client.Get(ctx, types.NamespacedName{Name: kubeSystemNamespace}, ns)
	if err != nil {
		return nil, err
	}

	return ns, nil
}
