/*
 Copyright © 2021-2026 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"os"
	"strings"
	"time"

	metricscommon "github.com/dell/csm-metrics-common/pkg/server"
	"github.com/dell/csm-replication/internal/metrics"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csmlog"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"

	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common/constants"

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
	currentSupportedCapabilities = []replication.ReplicationCapability_RPC_Type{
		replication.ReplicationCapability_RPC_CREATE_REMOTE_VOLUME,
		replication.ReplicationCapability_RPC_CREATE_PROTECTION_GROUP,
	}
	monitoringCapability = replication.ReplicationCapability_RPC_MONITOR_PROTECTION_GROUP
	ManifestSemver       string
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

type flags struct {
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
}

var (
	watchConfig              = viper.WatchConfig
	onConfigChange           = viper.OnConfigChange
	getConfig                = config.GetConfig
	getControllerManagerOpts = config.GetControllerManagerOpts
	getControllerClient      = connection.GetControllerClient
	kubeSystemNamespace      = controllers.KubeSystemNamespace

	getUpdateConfigMapFunc = func(mgr *ReplicatorManager, ctx context.Context) error {
		return mgr.config.UpdateConfigMap(ctx, nil, mgr.Opts, nil)
	}

	getConnectToCsiFunc = func(csiAddress string) (*grpc.ClientConn, error) {
		return connection.Connect(csiAddress)
	}

	getProbeForeverFunc = func(ctx context.Context, identityClient csiidentity.Identity) (string, error) {
		return identityClient.ProbeForever(ctx)
	}

	getReplicationCapabilitiesFunc = func(ctx context.Context, identityClient csiidentity.Identity) (csiidentity.ReplicationCapabilitySet, []*replication.SupportedActions, error) {
		return identityClient.GetReplicationCapabilities(ctx)
	}

	getCtrlNewManager = func(options manager.Options) (manager.Manager, error) {
		return ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	}

	getcreateReplicatorManagerFunc = func(ctx context.Context, mgr ctrl.Manager) (*ReplicatorManager, error) {
		return createReplicatorManager(ctx, mgr)
	}

	getWorkqueueReconcileRequest = func(retryIntervalStart time.Duration, retryIntervalMax time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
		return workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](retryIntervalStart, retryIntervalMax)
	}
	getReplicationMetricsCollectionInterval = func(defaultInterval time.Duration) time.Duration {
		if intervalStr := strings.TrimSpace(os.Getenv(constants.EnvReplicationMetricsCollectionInterval)); intervalStr != "" {
			if interval, err := time.ParseDuration(intervalStr); err == nil && interval > 0 {
				return interval
			}
			csmlog.WithFields(csmlog.Fields{"env": constants.EnvReplicationMetricsCollectionInterval}).Warnf("Invalid %s value: %q, using default %s", constants.EnvReplicationMetricsCollectionInterval, intervalStr, defaultInterval)
		}
		return defaultInterval
	}

	getPersistentVolumeClaimReconcilerSetupWithManager = func(r *controller.PersistentVolumeClaimReconciler, mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(mgr, limiter, maxReconcilers)
	}

	getPersistentVolumeReconcilerSetupWithManager = func(r *controller.PersistentVolumeReconciler, ctx context.Context, mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(ctx, mgr, limiter, maxReconcilers)
	}

	getReplicationGroupReconcilerSetupWithManager = func(r *controller.ReplicationGroupReconciler, mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(mgr, limiter, maxReconcilers)
	}

	getManagerStart = func(mgr manager.Manager) error {
		return mgr.Start(ctrl.SetupSignalHandler())
	}
	osExit = os.Exit

	newMetricsServerFunc   = metricscommon.NewMetricsServer
	startMetricsServerFunc = func(s *metricscommon.MetricsServer) error {
		return s.Start()
	}

	setupFlags = func() flags {
		flags := flags{}
		flag.StringVar(&flags.metricsAddr, "metrics-addr", ":8000", "The address the metric endpoint binds to.")
		flag.BoolVar(&flags.enableLeaderElection, "leader-election", false,
			"Enable leader election for controller manager. "+
				"Enabling this will ensure there is only one active controller manager.")
		flag.StringVar(&flags.csiAddress, "csi-address", "/var/run/csi.sock", "Address for the csi driver socket")
		flag.StringVar(&flags.domain, "prefix", constants.DefaultDomain, "Prefix used for creating labels/annotations")
		flag.IntVar(&flags.workerThreads, "worker-threads", 2, "Number of concurrent reconcilers for each of the controllers")
		flag.DurationVar(&flags.retryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed reconcile request. It doubles with each failure, upto retry-interval-max")
		flag.DurationVar(&flags.retryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed reconcile request")
		flag.DurationVar(&flags.operationTimeout, "timeout", 10*time.Second, "Timeout of waiting for response for CSI Driver")
		flag.StringVar(&flags.pgContextKeyPrefix, "context-prefix", "", "All the protection-group-attribute-keys with this prefix are added as annotation to the DellCSIReplicationGroup")
		flag.DurationVar(&flags.monitoringInterval, "monitoring-interval", 60*time.Second, "Time after which monitoring cycle runs")
		flag.DurationVar(&flags.probeFrequency, "probe-frequency", 5*time.Second, "Time between identity ProbeController calls")
		flag.DurationVar(&flags.maxRetryDurationForActions, "max-retry-action-duration", controller.MaxRetryDurationForActions,
			"Max duration after (since the first error encountered) which action won't be retried")
		flag.Parse()
		controllers.InitLabelsAndAnnotations(flags.domain)
		return flags
	}
)

func (mgr *ReplicatorManager) processConfigMapChanges() {
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

func (mgr *ReplicatorManager) setupConfigMapWatcher() {
	csmlog.Info("Started ConfigMap Watcher")
	watchConfig()
	onConfigChange(func(_ fsnotify.Event) {
		mgr.processConfigMapChanges()
	})
}

func createReplicatorManager(ctx context.Context, mgr ctrl.Manager) (*ReplicatorManager, error) {
	opts := getControllerManagerOpts()
	opts.Mode = "sidecar"
	repConfig, err := getConfig(ctx, nil, opts, nil)
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

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;watch;list;delete;update;create

func main() {
	flags := setupFlags()

	// Set controller-runtime logger to discard to prevent goroutine error
	// controller-runtime requires a global logger to be set; we discard its internal logs
	// and use csmlog for all application-specific logging instead
	ctrl.SetLogger(logr.Discard())

	csmlog.Infof("Prefix Domain: %s", flags.domain)
	csmlog.Infof("%s Version: %s, Creation Time: %s", constants.DellCSIReplicator, ManifestSemver, core.CommitTime.Format(time.RFC1123))

	ctx := context.Background()

	// Connect to csi
	csiConn, err := getConnectToCsiFunc(flags.csiAddress)
	if err != nil {
		csmlog.Errorf("failed to connect to CSI driver: %v", err)
		osExit(1)
	}

	identityClient := csiidentity.New(csiConn, flags.operationTimeout, flags.probeFrequency)

	driverName, err := getProbeForeverFunc(ctx, identityClient)
	if err != nil {
		csmlog.Errorf("error waiting for the CSI driver to be ready: %v", err)
		osExit(1)
	}
	csmlog.WithFields(csmlog.Fields{"driverName": driverName}).Info("CSI driver name")

	capabilitySet, supportedActions, err := getReplicationCapabilitiesFunc(ctx, identityClient)
	if err != nil {
		csmlog.Errorf("error fetching replication capabilities: %v", err)
		osExit(1)
	}
	if len(capabilitySet) == 0 {
		csmlog.Errorf("replication not supported: %v", fmt.Errorf("driver doesn't support replication"))
		osExit(1)
	}
	for _, capability := range currentSupportedCapabilities {
		if _, ok := capabilitySet[capability]; !ok {
			csmlog.Error(
				"one of the capabilities not supported")
			osExit(1)
		}
	}

	leaderElectionID := constants.DellCSIReplicator + strings.ReplaceAll(driverName, ".", "-")
	mgr, err := getCtrlNewManager(ctrl.Options{
		Scheme: scheme,
		Metrics: metricsServer.Options{
			BindAddress: flags.metricsAddr,
		},
		WebhookServer:              webhook.NewServer(webhook.Options{Port: 9443}),
		LeaderElection:             flags.enableLeaderElection,
		LeaderElectionResourceLock: "leases",
		LeaderElectionID:           leaderElectionID,
	})
	if err != nil {
		csmlog.Errorf("unable to start manager: %v", err)
		osExit(1)
	}

	controllerMgr, err := getcreateReplicatorManagerFunc(ctx, mgr)
	if err != nil {
		csmlog.Errorf("failed to configure the controller manager: %v", err)
		osExit(1)
	}
	// Start the watch on configmap
	controllerMgr.setupConfigMapWatcher()

	// Process the config. Get initial log level and format
	normalizedLogLevel := strings.ToLower(strings.TrimSpace(controllerMgr.config.LogLevel))
	if normalizedLogLevel != "" {
		level, err := csmlog.ParseLevel(normalizedLogLevel)
		if err != nil {
			csmlog.Errorf("Unable to parse log level: %v", err)
		} else {
			csmlog.Infof("set level to %v", level)
			csmlog.SetLevel(level)
		}
	}
	if controllerMgr.config.LogFormat != "" {
		switch strings.ToLower(controllerMgr.config.LogFormat) {
		case "json", "text":
			csmlog.Infof("set format to %v", strings.ToLower(controllerMgr.config.LogFormat))
			csmlog.SetFormat(strings.ToLower(controllerMgr.config.LogFormat))
		default:
			csmlog.Errorf("invalid log format %q, falling back to json", controllerMgr.config.LogFormat)
			csmlog.SetFormat("json")
		}
	}

	csmlog.Info("Starting manager")
	csmlog.Info("Starting controller-runtime")

	// Get the kube-system content
	var clusterUID string
	ns, err := getClusterUID(ctx)
	if err != nil {
		csmlog.Errorf("getClusterUuid error: %v", err)
	} else {
		csmlog.Infof("getClusterUuid got uuid: %s", ns.GetUID())
		clusterUID = string(ns.GetUID())
	}

	initReplicationMetrics(driverName)

	expRateLimiter := getWorkqueueReconcileRequest(flags.retryIntervalStart, flags.retryIntervalMax)
	csmlog.Info("Starting PersistentVolumeClaim controller")
	if err = getPersistentVolumeClaimReconcilerSetupWithManager(&controller.PersistentVolumeClaimReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(constants.DellCSIReplicator),
		DriverName:        driverName,
		ReplicationClient: csireplication.New(csiConn, flags.operationTimeout),
		ContextPrefix:     flags.pgContextKeyPrefix,
		SingleFlightGroup: singleflight.Group{},
		Domain:            flags.domain,
	}, mgr, expRateLimiter, flags.workerThreads); err != nil {
		csmlog.Errorf("unable to create controller: %v", err)
		osExit(1)
	}

	csmlog.Info("Starting PersistentVolume controller")
	if err = getPersistentVolumeReconcilerSetupWithManager(&controller.PersistentVolumeReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(constants.DellCSIReplicator),
		DriverName:        driverName,
		ReplicationClient: csireplication.New(csiConn, flags.operationTimeout),
		ContextPrefix:     flags.pgContextKeyPrefix,
		SingleFlightGroup: singleflight.Group{},
		Domain:            flags.domain,
		ClusterUID:        clusterUID,
	}, ctx, mgr, expRateLimiter, flags.workerThreads); err != nil {
		csmlog.Errorf("unable to create controller: %v", err)
		osExit(1)
	}

	csmlog.Info("Starting ReplicationGroup controller")
	if err = getReplicationGroupReconcilerSetupWithManager(&controller.ReplicationGroupReconciler{
		Client:                     mgr.GetClient(),
		Scheme:                     mgr.GetScheme(),
		EventRecorder:              mgr.GetEventRecorderFor(constants.DellCSIReplicator),
		DriverName:                 driverName,
		ReplicationClient:          csireplication.New(csiConn, flags.operationTimeout),
		SupportedActions:           supportedActions,
		MaxRetryDurationForActions: flags.maxRetryDurationForActions,
	}, mgr, expRateLimiter, flags.workerThreads); err != nil {
		csmlog.Errorf("unable to create controller: %v", err)
		osExit(1)
	}

	monitoringInterval := getReplicationMetricsCollectionInterval(flags.monitoringInterval)

	if _, ok := capabilitySet[monitoringCapability]; ok {
		csmlog.Info("driver supports monitoring capability. we will monitor the RGs")
		rgMonitor := controller.ReplicationGroupMonitoring{
			Client:             mgr.GetClient(),
			EventRecorder:      mgr.GetEventRecorderFor(constants.Monitoring),
			DriverName:         driverName,
			ReplicationClient:  csireplication.New(csiConn, flags.operationTimeout),
			MonitoringInterval: monitoringInterval,
		}

		err = rgMonitor.Monitor(ctx)
		if err != nil {
			osExit(1)
		}
	} else {
		csmlog.Info("driver does not support monitoring capability")
	}

	csmlog.Infof("Starting workers with %d threads", flags.workerThreads)

	csmlog.Info("starting manager")
	if err := getManagerStart(mgr); err != nil {
		csmlog.Errorf("problem running manager: %v", err)
		osExit(1)
	}
}

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

func initReplicationMetrics(driverName string) {
	if os.Getenv(constants.EnvReplicationMetricsEnabled) != "true" {
		csmlog.Info("Replication metrics are disabled")
		return
	}

	registry := prometheus.NewRegistry()
	replMetrics := metrics.NewReplicationMetrics(registry)
	metrics.SetGlobalReplicationMetrics(replMetrics)
	replMetrics.SetControllerHealth(driverName, true)

	srdfMetrics := metrics.NewSRDFMetrics(registry)
	metrics.SetGlobalSRDFMetrics(srdfMetrics)

	metricsPort := os.Getenv(constants.EnvReplicationMetricsPort)
	if metricsPort == "" {
		metricsPort = constants.DefaultMetricsPort
	}

	tlsCert := os.Getenv(constants.EnvReplicationMetricsTLSCertFile)
	tlsKey := os.Getenv(constants.EnvReplicationMetricsTLSKeyFile)

	newMetricsServer := newMetricsServerFunc
	startMetricsServer := startMetricsServerFunc
	go func() {
		// StaleMetricName is intentionally omitted: dell_csm_repl_metrics_stale is
		// owned by ReplicationMetrics and set via SetMetricsStale in the monitoring
		// loop to avoid a double-registration panic.
		cfg := metricscommon.Config{
			Port:     ":" + metricsPort,
			CertFile: tlsCert,
			KeyFile:  tlsKey,
			Registry: registry,
		}
		metricsSrv := newMetricsServer(cfg)

		if tlsCert != "" && tlsKey != "" {
			csmlog.Infof("Starting replication metrics server with TLS on port %s for driver %s", metricsPort, driverName)
		} else {
			csmlog.Infof("Starting replication metrics server on port %s for driver %s", metricsPort, driverName)
		}

		if err := startMetricsServer(metricsSrv); err != nil {
			csmlog.Errorf("Replication metrics server failed: %v", err)
		}
	}()
}
