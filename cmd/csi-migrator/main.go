/*
 Copyright © 2022-2026 Dell Inc. or its subsidiaries. All Rights Reserved.

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

	"github.com/dell/dell-csi-extensions/migration"
	"google.golang.org/grpc"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csmlog"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/spf13/viper"

	storagev1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common/constants"

	"golang.org/x/sync/singleflight"

	controller "github.com/dell/csm-replication/controllers/csi-migrator"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/dell/csm-replication/core"
	"github.com/dell/csm-replication/pkg/connection"
	csiidentity "github.com/dell/csm-replication/pkg/csi-clients/identity"
	csimigration "github.com/dell/csm-replication/pkg/csi-clients/migration"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme                       = runtime.NewScheme()
	currentSupportedCapabilities = map[migration.MigrateTypes]bool{
		migration.MigrateTypes_NON_REPL_TO_REPL: true,
		migration.MigrateTypes_REPL_TO_NON_REPL: true,
		migration.MigrateTypes_VERSION_UPGRADE:  true,
	}
	ManifestSemver string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(storagev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// MigratorManager - Represents the controller manager and its configuration
type MigratorManager struct {
	Opts    config.ControllerManagerOpts
	Manager ctrl.Manager
	config  *config.Config
}

var (
	getUpdateConfigMapFunc = func(mgr *MigratorManager, ctx context.Context) error {
		return mgr.config.UpdateConfigMap(ctx, nil, mgr.Opts, nil)
	}

	getConfigFunc = func(ctx context.Context, opts config.ControllerManagerOpts) (*config.Config, error) {
		return config.GetConfig(ctx, nil, opts, nil)
	}

	getConnectToCsiFunc = func(csiAddress string) (*grpc.ClientConn, error) {
		return connection.Connect(csiAddress)
	}

	getProbeForeverFunc = func(ctx context.Context, identityClient csiidentity.Identity) (string, error) {
		return identityClient.ProbeForever(ctx)
	}

	getMigrationCapabilitiesFunc = func(ctx context.Context, identityClient csiidentity.Identity) (csiidentity.MigrationCapabilitySet, error) {
		return identityClient.GetMigrationCapabilities(ctx)
	}

	getCtrlNewManager = func(options manager.Options) (manager.Manager, error) {
		return ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	}

	getcreateMigratorManagerFunc = func(ctx context.Context, mgr manager.Manager) (*MigratorManager, error) {
		return createMigratorManager(ctx, mgr)
	}

	getWorkqueueReconcileRequest = func(retryIntervalStart time.Duration, retryIntervalMax time.Duration) workqueue.TypedRateLimiter[reconcile.Request] {
		return workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](retryIntervalStart, retryIntervalMax)
	}

	getPersistentVolumeReconcilerSetupWithManager = func(r *controller.PersistentVolumeReconciler, ctx context.Context, mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(ctx, mgr, limiter, maxReconcilers)
	}

	getMigrationGroupReconcilerSetupWithManager = func(r *controller.MigrationGroupReconciler, mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(mgr, limiter, maxReconcilers)
	}

	getManagerStart = func(mgr manager.Manager) error {
		return mgr.Start(ctrl.SetupSignalHandler())
	}

	osExit = os.Exit

	setupFlags = func() flags {
		flags := flags{}
		flag.StringVar(&flags.metricsAddr, "metrics-addr", ":8001", "The address the metric endpoint binds to.")
		flag.BoolVar(&flags.enableLeaderElection, "leader-election", false,
			"Enable leader election for controller manager. "+
				"Enabling this will ensure there is only one active controller manager.")
		flag.StringVar(&flags.csiAddress, "csi-address", "/var/run/csi.sock", "Address for the csi driver socket")
		flag.StringVar(&flags.domain, "prefix", constants.DefaultMigrationDomain, "Prefix used for creating labels/annotations")
		flag.StringVar(&flags.replicationDomain, "repl-prefix", constants.DefaultDomain, "Replication prefix used for creating labels/annotations")
		flag.IntVar(&flags.workerThreads, "worker-threads", 2, "Number of concurrent reconcilers for each of the controllers")
		flag.DurationVar(&flags.retryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed reconcile request. It doubles with each failure, upto retry-interval-max")
		flag.DurationVar(&flags.retryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed reconcile request")
		flag.DurationVar(&flags.operationTimeout, "timeout", 300*time.Second, "Timeout of waiting for response for CSI Driver")
		flag.DurationVar(&flags.probeFrequency, "probe-frequency", 5*time.Second, "Time between identity ProbeController calls")
		flag.Parse()
		controllers.InitLabelsAndAnnotations(flags.domain)

		return flags
	}
)

func (mgr *MigratorManager) processConfigMapChanges() {
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

func (mgr *MigratorManager) setupConfigMapWatcher() {
	csmlog.Info("Started ConfigMap Watcher")
	viper.WatchConfig()
	viper.OnConfigChange(func(_ fsnotify.Event) {
		mgr.processConfigMapChanges()
	})
}

func createMigratorManager(ctx context.Context, mgr ctrl.Manager) (*MigratorManager, error) {
	// Check if the manager is nil
	if mgr == nil {
		return nil, fmt.Errorf("manager cannot be nil")
	}
	opts := config.GetControllerManagerOpts()
	opts.Mode = "sidecar"
	repConfig, err := getConfigFunc(ctx, opts)
	if err != nil {
		return nil, err
	}

	controllerManager := MigratorManager{
		Opts:    opts,
		Manager: mgr,
		config:  repConfig,
	}
	return &controllerManager, nil
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;watch;list;delete;update;create

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
	replicationDomain          string
	probeFrequency             time.Duration
	maxRetryDurationForActions time.Duration
}

func main() {
	flags := setupFlags()

	// Set controller-runtime logger to discard to prevent goroutine error
	// controller-runtime requires a global logger to be set; we discard its internal logs
	// and use csmlog for all application-specific logging instead
	ctrl.SetLogger(logr.Discard())

	csmlog.Infof("Prefix: %v", flags.domain)
	csmlog.Infof("%s Version: %s, Creation Time: %s", constants.DellCSIMigrator, ManifestSemver, core.CommitTime.Format(time.RFC1123))

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
	csmlog.Infof("CSI driver name: %v", driverName)

	capabilitySet, err := getMigrationCapabilitiesFunc(ctx, identityClient)
	if err != nil {
		csmlog.Errorf("error fetching migration capabilities: %v", err)
		osExit(1)
	}
	if len(capabilitySet) == 0 {
		csmlog.Error("migration not supported: driver doesn't support migration")
		osExit(1)
	}
	for types := range capabilitySet {
		if _, ok := currentSupportedCapabilities[types]; !ok {
			csmlog.Errorf("unknown capability advertised: %v", err)
			osExit(1)
		}
	}
	leaderElectionID := constants.DellCSIMigrator + strings.ReplaceAll(driverName, ".", "-")
	mgr, err := getCtrlNewManager(ctrl.Options{
		Scheme: scheme,
		Metrics: metricsServer.Options{
			BindAddress: flags.metricsAddr,
		},
		WebhookServer:              webhook.NewServer(webhook.Options{Port: 8443}),
		LeaderElection:             flags.enableLeaderElection,
		LeaderElectionResourceLock: "leases",
		LeaderElectionID:           leaderElectionID,
	})
	if err != nil {
		csmlog.Errorf("unable to start manager: %v", err)
		osExit(1)
	}

	MigratorMgr, err := getcreateMigratorManagerFunc(ctx, mgr)
	if err != nil {
		csmlog.Errorf("failed to configure the migrator manager: %v", err)
		osExit(1)
	}
	// Start the watch on configmap
	MigratorMgr.setupConfigMapWatcher()

	// Process the config. Get initial log level and format
	normalizedLogLevel := strings.ToLower(strings.TrimSpace(MigratorMgr.config.LogLevel))
	if normalizedLogLevel != "" {
		level, err := csmlog.ParseLevel(normalizedLogLevel)
		if err != nil {
			csmlog.Errorf("Unable to parse log level: %v", err)
		} else {
			csmlog.Infof("set level to %v", level)
			csmlog.SetLevel(level)
		}
	}
	if MigratorMgr.config.LogFormat != "" {
		switch strings.ToLower(MigratorMgr.config.LogFormat) {
		case "json", "text":
			csmlog.Infof("set format to %v", strings.ToLower(MigratorMgr.config.LogFormat))
			csmlog.SetFormat(strings.ToLower(MigratorMgr.config.LogFormat))
		default:
			csmlog.Errorf("invalid log format %q, falling back to json", MigratorMgr.config.LogFormat)
			csmlog.SetFormat("json")
		}
	}

	csmlog.Info("Starting manager")
	csmlog.Info("Starting controller-runtime")

	expRateLimiter := getWorkqueueReconcileRequest(flags.retryIntervalStart, flags.retryIntervalMax)

	csmlog.Info("Starting PersistentVolume controller")
	if err = getPersistentVolumeReconcilerSetupWithManager(&controller.PersistentVolumeReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(constants.DellCSIReplicator),
		DriverName:        driverName,
		MigrationClient:   csimigration.New(csiConn, flags.operationTimeout),
		ContextPrefix:     flags.pgContextKeyPrefix,
		SingleFlightGroup: singleflight.Group{},
		Domain:            flags.domain,
		ReplDomain:        flags.replicationDomain,
	}, ctx, mgr, expRateLimiter, flags.workerThreads); err != nil {
		csmlog.Errorf("unable to create controller: %v", err)
		osExit(1)
	}

	csmlog.Info("Starting MigrationGroup controller")
	if err = getMigrationGroupReconcilerSetupWithManager(&controller.MigrationGroupReconciler{
		Client:                     mgr.GetClient(),
		Scheme:                     mgr.GetScheme(),
		EventRecorder:              mgr.GetEventRecorderFor(constants.DellCSIMigrator),
		DriverName:                 driverName,
		MigrationClient:            csimigration.New(csiConn, flags.operationTimeout),
		MaxRetryDurationForActions: flags.maxRetryDurationForActions,
	}, mgr, expRateLimiter, flags.workerThreads); err != nil {
		csmlog.Errorf("unable to create controller: %v", err)
		osExit(1)
	}

	csmlog.Infof("Starting workers with %d threads", flags.workerThreads)

	csmlog.Info("starting manager")
	if err := getManagerStart(mgr); err != nil {
		csmlog.Errorf("problem running manager: %v", err)
		osExit(1)
	}
}
