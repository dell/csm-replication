/*
Copyright © 2023-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"strconv"
	"strings"
	"time"

	"github.com/bombsimon/logrusr/v4"
	storagev1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	controller "github.com/dell/csm-replication/controllers/csi-node-rescanner"
	"github.com/dell/csm-replication/core"
	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/config"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	"github.com/dell/dell-csi-extensions/migration"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/dell/csm-replication/pkg/connection"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var (
	scheme                       = runtime.NewScheme()
	setupLog                     = ctrl.Log.WithName("setup")
	currentSupportedCapabilities = map[migration.MigrateTypes]bool{
		migration.MigrateTypes_NON_REPL_TO_REPL: true,
		migration.MigrateTypes_REPL_TO_NON_REPL: true,
		migration.MigrateTypes_VERSION_UPGRADE:  true,
	}
	// Added to improve UT coverage
	osExit = os.Exit

	// Create a new wrapper function
	createNodeReScannerManagerWrapper = func(ctx context.Context, mgr ctrl.Manager) *NodeRescanner {
		// Call the original createNodeReScannerManager function
		return createNodeReScannerManager(ctx, mgr)
	}

	getCtrlNewManager = func(options manager.Options) (manager.Manager, error) {
		return ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	}

	getWorkqueueReconcileRequest = func(retryIntervalStart time.Duration, retryIntervalMax time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
		return workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](retryIntervalStart, retryIntervalMax)
	}

	getNodeRescanReconcilerManager = func(r *controller.NodeRescanReconciler, mgr manager.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(mgr, limiter, maxReconcilers)
	}

	getManagerStart = func(mgr manager.Manager) error {
		return mgr.Start(ctrl.SetupSignalHandler())
	}

	getConnection = func(csiAddress string, setupLog logr.Logger) (*grpc.ClientConn, error) {
		return connection.Connect(csiAddress, setupLog)
	}
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(storagev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// NodeRescanner - Represents the controller manager and its configuration
type NodeRescanner struct {
	Opts     config.ControllerManagerOpts
	Manager  ctrl.Manager
	config   *config.Config
	NodeName string
}

func (mgr *NodeRescanner) processConfigMapChanges(loggerConfig *logrus.Logger) {
	loggerConfig.Info("Received a config change event")
	err := mgr.config.UpdateConfigMap(context.Background(), nil, mgr.Opts, nil, mgr.Manager.GetLogger())
	if err != nil {
		loggerConfig.Error("Error parsing the config: ", err)
		return
	}
	mgr.config.Lock.Lock()
	defer mgr.config.Lock.Unlock()
	level, err := common.ParseLevel(mgr.config.LogLevel)
	if err != nil {
		loggerConfig.Error("Unable to parse ", err)
	}
	loggerConfig.Info("set level to", level)
	loggerConfig.SetLevel(level)
}

func (mgr *NodeRescanner) setupConfigMapWatcher(loggerConfig *logrus.Logger) {
	log.Println("Started ConfigMap Watcher")
	viper.WatchConfig()
	viper.OnConfigChange(func(_ fsnotify.Event) {
		mgr.processConfigMapChanges(loggerConfig)
	})
}

func createNodeReScannerManager(_ context.Context, mgr ctrl.Manager) *NodeRescanner {
	opts := config.GetControllerManagerOpts()
	opts.Mode = "sidecar"
	//mgrLogger := mgr.GetLogger()
	//repConfig, err := config.GetConfig(ctx, nil, opts, nil, mgrLogger)
	//if err != nil {
	//	return nil, err
	//}
	nodeName, found := os.LookupEnv(common.EnvNodeName)
	if !found {
		logrus.Warning("Node name not found")
		nodeName = ""
	}
	controllerManager := NodeRescanner{
		Opts:     opts,
		Manager:  mgr,
		NodeName: nodeName,
	}
	return &controllerManager
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;watch;list;delete;update;create

func main() {
	flagMap, setupLog, ctx := setupFlags()

	// Connect to csi
	csiConn := getCSIConn(flagMap["csi-address"], setupLog)

	// Create an instance of the identity client
	identityClient := csiidentity.New(csiConn, ctrl.Log.WithName("identity-client"), stringToTimeDuration(flagMap["timeout"]), stringToTimeDuration(flagMap["probe-frequency"]))

	// Probe the CSI driverand create the metrics server
	probeAndCreateMetricsServer(ctx, csiConn, setupLog, identityClient, flagMap)
}

func probeAndCreateMetricsServer(ctx context.Context, csiConn *grpc.ClientConn, setupLog logr.Logger, identityClient csiidentity.Identity, flagMap map[string]string) {
	driverName := probeCSIDriver(ctx, csiConn, setupLog, identityClient)

	// Create the metrics server
	createMetricsServer(ctx, driverName, flagMap["metrics-addr"], stringToBoolean(flagMap["leader-election"]), setupLog, stringToTimeDuration(flagMap["retry-interval-start"]), stringToTimeDuration(flagMap["retry-interval-max"]), stringToTimeDuration(flagMap["max-retry-duration-for-actions"]), stringToInt(flagMap["worker-threads"]))
}

func setupFlags() (map[string]string, logr.Logger, context.Context) {
	var (
		metricsAddr                string
		enableLeaderElection       bool
		csiAddress                 string
		workerThreads              int
		retryIntervalStart         time.Duration
		retryIntervalMax           time.Duration
		operationTimeout           time.Duration
		domain                     string
		replicationDomain          string
		probeFrequency             time.Duration
		maxRetryDurationForActions time.Duration
	)
	flag.StringVar(&metricsAddr, "metrics-addr", ":8001", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&csiAddress, "csi-address", "/var/run/csi.sock", "Address for the csi driver socket")
	flag.StringVar(&domain, "prefix", common.DefaultMigrationDomain, "Prefix used for creating labels/annotations")
	flag.StringVar(&replicationDomain, "repl-prefix", common.DefaultDomain, "Replication prefix used for creating labels/annotations")
	flag.IntVar(&workerThreads, "worker-threads", 2, "Number of concurrent reconcilers for each of the controllers")
	flag.DurationVar(&retryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed reconcile request. It doubles with each failure, upto retry-interval-max")
	flag.DurationVar(&retryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed reconcile request")
	flag.DurationVar(&operationTimeout, "timeout", 300*time.Second, "Timeout of waiting for response for CSI Driver")
	flag.DurationVar(&probeFrequency, "probe-frequency", 5*time.Second, "Time between identity ProbeController calls")
	flag.DurationVar(&maxRetryDurationForActions, "max-retry-action-duration", controller.MaxRetryDurationForActions,
		"Max duration after (since the first error encountered) which action won't be retried")
	flag.Parse()
	controllers.InitLabelsAndAnnotations(domain)

	setupLog.V(1).Info("Prefix", "Domain", domain)
	setupLog.V(1).Info(common.DellCSINodeReScanner, "Version", core.SemVer, "Commit ID", core.CommitSha32, "Commit SHA", core.CommitTime.Format(time.RFC1123))

	flags := make(map[string]string)
	flags["metrics-addr"] = metricsAddr
	flags["leader-election"] = strconv.FormatBool(enableLeaderElection)
	flags["csi-address"] = csiAddress
	flags["prefix"] = domain
	flags["repl-prefix"] = replicationDomain
	flags["worker-threads"] = strconv.Itoa(workerThreads)
	flags["retry-interval-start"] = retryIntervalStart.String()
	flags["retry-interval-max"] = retryIntervalMax.String()
	flags["timeout"] = operationTimeout.String()
	flags["probe-frequency"] = probeFrequency.String()
	flags["max-retry-action-duration"] = maxRetryDurationForActions.String()
	return flags, setupLog, context.Background()
}

func getCSIConn(csiAddress string, setupLog logr.Logger) *grpc.ClientConn {
	csiConn, err := getConnection(csiAddress, setupLog)
	if err != nil {
		setupLog.Error(err, "failed to connect to CSI driver")
		osExit(1)
	}
	return csiConn
}

func probeCSIDriver(ctx context.Context, _ *grpc.ClientConn, setupLog logr.Logger, identityClient csiidentity.Identity) string {
	driverName, err := identityClient.ProbeForever(ctx)
	if err != nil {
		setupLog.Error(err, "error waiting for the CSI driver to be ready")
		osExit(1)
	}
	setupLog.V(1).Info("CSI driver name", "driverName", driverName)

	capabilitySet, err := identityClient.GetMigrationCapabilities(ctx)
	if err != nil {
		setupLog.Error(err, "error fetching migration capabilities")
		osExit(1)
	}
	if len(capabilitySet) == 0 {
		setupLog.Error(fmt.Errorf("driver doesn't support migration"), "migration not supported")
		osExit(1)
	}
	for types := range capabilitySet {
		if _, ok := currentSupportedCapabilities[types]; !ok {
			setupLog.Error(err, "unknown capability advertised")
			osExit(1)
		}
	}

	return driverName
}

func createMetricsServer(ctx context.Context, driverName string, metricsAddr string, enableLeaderElection bool, setupLog logr.Logger, retryIntervalStart, retryIntervalMax, maxRetryDurationForActions time.Duration, workerThreads int) {
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})

	logger := logrusr.New(logrusLog)
	ctrl.SetLogger(logger)

	leaderElectionID := common.DellCSINodeReScanner + strings.ReplaceAll(driverName, ".", "-")

	mgr, err := getCtrlNewManager(ctrl.Options{
		Scheme: scheme,
		Metrics: metricsServer.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer:              webhook.NewServer(webhook.Options{Port: 8443}),
		LeaderElection:             enableLeaderElection,
		LeaderElectionResourceLock: "leases",
		LeaderElectionID:           leaderElectionID,
	})
	if err != nil {
		logrusLog.Error("Unable to start manager")
		setupLog.Error(err, "unable to start manager")
		osExit(1)
	}

	// Create the node rescan manager
	createRescanManager(ctx, mgr, driverName, retryIntervalStart, retryIntervalMax, maxRetryDurationForActions, workerThreads, logrusLog)
}

func createRescanManager(ctx context.Context, mgr manager.Manager, driverName string, retryIntervalStart time.Duration, retryIntervalMax time.Duration, maxRetryDurationForActions time.Duration, workerThreads int, logrusLog *logrus.Logger) {
	rescanMgr := createNodeReScannerManagerWrapper(ctx, mgr)
	log.Printf("Rescan manager configured: (+%v)", rescanMgr)
	// Start the watch on configmap
	// rescanMgr.setupConfigMapWatcher(logrusLog)

	// Process the config. Get initial log level
	level, _ := common.ParseLevel("debug")
	logrusLog.Info("set level to", level)
	logrusLog.SetLevel(level)

	expRateLimiter := getWorkqueueReconcileRequest(retryIntervalStart, retryIntervalMax)
	logrusLog.Info("expRateLimiter", expRateLimiter)

	if err := getNodeRescanReconcilerManager(&controller.NodeRescanReconciler{
		Client:                     mgr.GetClient(),
		Log:                        ctrl.Log.WithName("controllers").WithName("DellCSINodeReScanner"),
		Scheme:                     mgr.GetScheme(),
		EventRecorder:              mgr.GetEventRecorderFor(common.DellCSINodeReScanner),
		DriverName:                 driverName,
		NodeName:                   rescanMgr.NodeName,
		MaxRetryDurationForActions: maxRetryDurationForActions,
	}, mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DellCSINodeReScanner")
		osExit(1)
	}
	logrusLog.Info("strating manager")
	setupLog.Info("starting manager")
	if err := getManagerStart(mgr); err != nil {
		logrusLog.Error("problem running manager")
		setupLog.Error(err, "problem running manager")
		osExit(1)
	}
	logrusLog.Info("manager started successfully")
}

func stringToTimeDuration(timeString string) time.Duration {
	duration, err := time.ParseDuration(timeString)
	if err != nil {
		return 0
	}
	return duration
}

func stringToBoolean(boolString string) bool {
	boolean, err := strconv.ParseBool(boolString)
	if err != nil {
		return false
	}
	return boolean
}

func stringToInt(intString string) int {
	integer, err := strconv.Atoi(intString)
	if err != nil {
		return 0
	}
	return integer
}
