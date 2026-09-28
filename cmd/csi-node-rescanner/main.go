/*
Copyright © 2023-2026 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"os"
	"strconv"
	"strings"
	"time"

	storagev1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	controller "github.com/dell/csm-replication/controllers/csi-node-rescanner"
	"github.com/dell/csm-replication/core"
	"github.com/dell/csm-replication/pkg/common/constants"
	"github.com/dell/csm-replication/pkg/config"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	"github.com/dell/csmlog"
	"github.com/dell/dell-csi-extensions/migration"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
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

	getConnection = func(csiAddress string) (*grpc.ClientConn, error) {
		return connection.Connect(csiAddress)
	}

	getUpdateConfigMapFunc = func(mgr *NodeRescanner, ctx context.Context) error {
		return mgr.config.UpdateConfigMap(ctx, nil, mgr.Opts, nil)
	}

	ManifestSemver string
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

func (mgr *NodeRescanner) processConfigMapChanges() {
	csmlog.Info("Received a config change event")
	err := getUpdateConfigMapFunc(mgr, context.Background())
	if err != nil {
		csmlog.Errorf("Error parsing the config: %v", err)
		return
	}
	mgr.config.Lock.Lock()
	defer mgr.config.Lock.Unlock()
	normalizedLogLevel := strings.ToLower(strings.TrimSpace(mgr.config.LogLevel))
	if normalizedLogLevel != "" {
		level, err := csmlog.ParseLevel(normalizedLogLevel)
		if err != nil {
			csmlog.Errorf("Unable to parse log level: %v", err)
		} else {
			csmlog.Infof("set level to %v", level)
			csmlog.SetLevel(level)
		}
	}
	format := mgr.config.LogFormat
	if format != "" {
		switch strings.ToLower(format) {
		case "json", "text":
			csmlog.Infof("set format to %v", strings.ToLower(format))
			csmlog.SetFormat(strings.ToLower(format))
		default:
			csmlog.Errorf("invalid log format %q, falling back to json", format)
			csmlog.SetFormat("json")
		}
	}
}

func (mgr *NodeRescanner) setupConfigMapWatcher() {
	csmlog.Info("Started ConfigMap Watcher")
	viper.WatchConfig()
	viper.OnConfigChange(func(_ fsnotify.Event) {
		mgr.processConfigMapChanges()
	})
}

func createNodeReScannerManager(_ context.Context, mgr ctrl.Manager) *NodeRescanner {
	opts := config.GetControllerManagerOpts()
	opts.Mode = "sidecar"
	nodeName, found := os.LookupEnv(constants.EnvNodeName)
	if !found {
		csmlog.Warn("Node name not found")
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
	flagMap, ctx := setupFlags()

	// Connect to csi
	csiConn := getCSIConn(flagMap["csi-address"])

	// Create an instance of the identity client
	identityClient := csiidentity.New(csiConn, stringToTimeDuration(flagMap["timeout"]), stringToTimeDuration(flagMap["probe-frequency"]))

	// Probe the CSI driverand create the metrics server
	probeAndCreateMetricsServer(ctx, csiConn, identityClient, flagMap)
}

func probeAndCreateMetricsServer(ctx context.Context, csiConn *grpc.ClientConn, identityClient csiidentity.Identity, flagMap map[string]string) {
	driverName := probeCSIDriver(ctx, csiConn, identityClient)

	// Create the metrics server
	createMetricsServer(ctx, driverName, flagMap["metrics-addr"], stringToBoolean(flagMap["leader-election"]), stringToTimeDuration(flagMap["retry-interval-start"]), stringToTimeDuration(flagMap["retry-interval-max"]), stringToTimeDuration(flagMap["max-retry-duration-for-actions"]), stringToInt(flagMap["worker-threads"]))
}

func setupFlags() (map[string]string, context.Context) {
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
	flag.StringVar(&domain, "prefix", constants.DefaultMigrationDomain, "Prefix used for creating labels/annotations")
	flag.StringVar(&replicationDomain, "repl-prefix", constants.DefaultDomain, "Replication prefix used for creating labels/annotations")
	flag.IntVar(&workerThreads, "worker-threads", 2, "Number of concurrent reconcilers for each of the controllers")
	flag.DurationVar(&retryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed reconcile request. It doubles with each failure, upto retry-interval-max")
	flag.DurationVar(&retryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed reconcile request")
	flag.DurationVar(&operationTimeout, "timeout", 300*time.Second, "Timeout of waiting for response for CSI Driver")
	flag.DurationVar(&probeFrequency, "probe-frequency", 5*time.Second, "Time between identity ProbeController calls")
	flag.DurationVar(&maxRetryDurationForActions, "max-retry-action-duration", controller.MaxRetryDurationForActions,
		"Max duration after (since the first error encountered) which action won't be retried")
	flag.Parse()
	controllers.InitLabelsAndAnnotations(domain)

	// Set controller-runtime logger to discard to prevent goroutine error
	// controller-runtime requires a global logger to be set; we discard its internal logs
	// and use csmlog for all application-specific logging instead
	ctrl.SetLogger(logr.Discard())

	csmlog.Infof("Prefix: %v", domain)
	csmlog.Infof("%s Version: %s, Creation Time: %s", constants.DellCSINodeReScanner, ManifestSemver, core.CommitTime.Format(time.RFC1123))

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
	return flags, context.Background()
}

func getCSIConn(csiAddress string) *grpc.ClientConn {
	csiConn, err := getConnection(csiAddress)
	if err != nil {
		csmlog.Errorf("failed to connect to CSI driver: %v", err)
		osExit(1)
	}
	return csiConn
}

func probeCSIDriver(ctx context.Context, _ *grpc.ClientConn, identityClient csiidentity.Identity) string {
	driverName, err := identityClient.ProbeForever(ctx)
	if err != nil {
		csmlog.Errorf("error waiting for the CSI driver to be ready: %v", err)
		osExit(1)
	}
	csmlog.WithFields(csmlog.Fields{"driverName": driverName}).Info("CSI driver name")

	capabilitySet, err := identityClient.GetMigrationCapabilities(ctx)
	if err != nil {
		csmlog.Errorf("error fetching migration capabilities: %v", err)
		osExit(1)
	}
	if len(capabilitySet) == 0 {
		csmlog.Error("driver doesn't support migration")
		osExit(1)
	}
	for types := range capabilitySet {
		if _, ok := currentSupportedCapabilities[types]; !ok {
			csmlog.Error("unknown capability advertised")
			osExit(1)
		}
	}

	return driverName
}

func createMetricsServer(ctx context.Context, driverName string, metricsAddr string, enableLeaderElection bool, retryIntervalStart, retryIntervalMax, maxRetryDurationForActions time.Duration, workerThreads int) {
	leaderElectionID := constants.DellCSINodeReScanner + strings.ReplaceAll(driverName, ".", "-")

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
		csmlog.Error("Unable to start manager")
		csmlog.Errorf("unable to start manager")
		osExit(1)
	}

	// Create the node rescan manager
	createRescanManager(ctx, mgr, driverName, retryIntervalStart, retryIntervalMax, maxRetryDurationForActions, workerThreads)
}

func createRescanManager(ctx context.Context, mgr manager.Manager, driverName string, retryIntervalStart time.Duration, retryIntervalMax time.Duration, maxRetryDurationForActions time.Duration, workerThreads int) {
	rescanMgr := createNodeReScannerManagerWrapper(ctx, mgr)
	csmlog.Infof("Rescan manager configured: (+%v)", rescanMgr)
	// Start the watch on configmap
	// rescanMgr.setupConfigMapWatcher()

	// Process the config. Get initial log level
	level, _ := csmlog.ParseLevel("debug")
	csmlog.Infof("set level to %v", level)
	csmlog.SetLevel(level)

	csmlog.Info("Starting manager")
	csmlog.Info("Starting controller-runtime")

	expRateLimiter := getWorkqueueReconcileRequest(retryIntervalStart, retryIntervalMax)
	csmlog.Infof("expRateLimiter %v", expRateLimiter)

	csmlog.Info("Starting NodeRescan controller")
	if err := getNodeRescanReconcilerManager(&controller.NodeRescanReconciler{
		Client:                     mgr.GetClient(),
		Scheme:                     mgr.GetScheme(),
		EventRecorder:              mgr.GetEventRecorderFor(constants.DellCSINodeReScanner),
		DriverName:                 driverName,
		NodeName:                   rescanMgr.NodeName,
		MaxRetryDurationForActions: maxRetryDurationForActions,
	}, mgr, expRateLimiter, workerThreads); err != nil {
		csmlog.WithFields(csmlog.Fields{"controller": constants.DellCSINodeReScanner}).Errorf("unable to create controller: %v", err)
		osExit(1)
	}
	csmlog.Infof("Starting workers with %d threads", workerThreads)
	csmlog.Info("starting manager")
	if err := getManagerStart(mgr); err != nil {
		csmlog.Errorf("problem running manager: %v", err)
		osExit(1)
	}
	csmlog.Info("manager started successfully")
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
