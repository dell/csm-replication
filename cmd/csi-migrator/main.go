/*
 Copyright © 2022-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

	"github.com/dell/dell-csi-extensions/migration"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/bombsimon/logrusr/v4"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	storagev1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"

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
	setupLog                     = ctrl.Log.WithName("setup")
	currentSupportedCapabilities = map[migration.MigrateTypes]bool{
		migration.MigrateTypes_NON_REPL_TO_REPL: true,
		migration.MigrateTypes_REPL_TO_NON_REPL: true,
		migration.MigrateTypes_VERSION_UPGRADE:  true,
	}
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
		return mgr.config.UpdateConfigMap(ctx, nil, mgr.Opts, nil, mgr.Manager.GetLogger())
	}

	getConfigFunc = func(ctx context.Context, opts config.ControllerManagerOpts, mgrLogr logr.Logger) (*config.Config, error) {
		return config.GetConfig(ctx, nil, opts, nil, mgrLogr)
	}

	getConnectToCsiFunc = func(csiAddress string, setupLog logr.Logger) (*grpc.ClientConn, error) {
		return connection.Connect(csiAddress, setupLog)
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

	getParseLevelFunc = func(level string) (logrus.Level, error) {
		return common.ParseLevel(level)
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

	setupFlags = func() (flags, *logrus.Logger) {
		flags := flags{}
		flag.StringVar(&flags.metricsAddr, "metrics-addr", ":8001", "The address the metric endpoint binds to.")
		flag.BoolVar(&flags.enableLeaderElection, "leader-election", false,
			"Enable leader election for controller manager. "+
				"Enabling this will ensure there is only one active controller manager.")
		flag.StringVar(&flags.csiAddress, "csi-address", "/var/run/csi.sock", "Address for the csi driver socket")
		flag.StringVar(&flags.domain, "prefix", common.DefaultMigrationDomain, "Prefix used for creating labels/annotations")
		flag.StringVar(&flags.replicationDomain, "repl-prefix", common.DefaultDomain, "Replication prefix used for creating labels/annotations")
		flag.IntVar(&flags.workerThreads, "worker-threads", 2, "Number of concurrent reconcilers for each of the controllers")
		flag.DurationVar(&flags.retryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed reconcile request. It doubles with each failure, upto retry-interval-max")
		flag.DurationVar(&flags.retryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed reconcile request")
		flag.DurationVar(&flags.operationTimeout, "timeout", 300*time.Second, "Timeout of waiting for response for CSI Driver")
		flag.DurationVar(&flags.probeFrequency, "probe-frequency", 5*time.Second, "Time between identity ProbeController calls")
		flag.Parse()
		controllers.InitLabelsAndAnnotations(flags.domain)
		logrusLog := logrus.New()
		logrusLog.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})

		logger := logrusr.New(logrusLog)
		ctrl.SetLogger(logger)

		setupLog.V(1).Info("Prefix", "Domain", flags.domain)
		setupLog.V(1).Info(common.DellCSIMigrator, "Version", core.SemVer, "Commit ID", core.CommitSha32, "Commit SHA", core.CommitTime.Format(time.RFC1123))

		return flags, logrusLog
	}
)

func (mgr *MigratorManager) processConfigMapChanges(loggerConfig *logrus.Logger) {
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

func (mgr *MigratorManager) setupConfigMapWatcher(loggerConfig *logrus.Logger) {
	log.Println("Started ConfigMap Watcher")
	viper.WatchConfig()
	viper.OnConfigChange(func(_ fsnotify.Event) {
		mgr.processConfigMapChanges(loggerConfig)
	})
}

func createMigratorManager(ctx context.Context, mgr ctrl.Manager) (*MigratorManager, error) {
	// Check if the manager is nil
	if mgr == nil {
		return nil, fmt.Errorf("manager cannot be nil")
	}
	opts := config.GetControllerManagerOpts()
	opts.Mode = "sidecar"
	mgrLogger := mgr.GetLogger()
	repConfig, err := getConfigFunc(ctx, opts, mgrLogger)
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
	flags, logrusLog := setupFlags()

	ctx := context.Background()

	// Connect to csi
	csiConn, err := getConnectToCsiFunc(flags.csiAddress, setupLog)
	if err != nil {
		setupLog.Error(err, "failed to connect to CSI driver")
		osExit(1)
	}

	identityClient := csiidentity.New(csiConn, ctrl.Log.WithName("identity-client"), flags.operationTimeout, flags.probeFrequency)

	driverName, err := getProbeForeverFunc(ctx, identityClient)
	if err != nil {
		setupLog.Error(err, "error waiting for the CSI driver to be ready")
		osExit(1)
	}
	setupLog.V(1).Info("CSI driver name", "driverName", driverName)

	capabilitySet, err := getMigrationCapabilitiesFunc(ctx, identityClient)
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
	leaderElectionID := common.DellCSIMigrator + strings.ReplaceAll(driverName, ".", "-")
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
		setupLog.Error(err, "unable to start manager")
		osExit(1)
	}

	MigratorMgr, err := getcreateMigratorManagerFunc(ctx, mgr)
	if err != nil {
		setupLog.Error(err, "failed to configure the migrator manager")
		osExit(1)
	}
	// Start the watch on configmap
	MigratorMgr.setupConfigMapWatcher(logrusLog)

	// Process the config. Get initial log level
	level, err := getParseLevelFunc(MigratorMgr.config.LogLevel)
	if err != nil {
		log.Println("Unable to parse ", err)
	}
	log.Println("set level to", level)
	logrusLog.SetLevel(level)

	expRateLimiter := getWorkqueueReconcileRequest(flags.retryIntervalStart, flags.retryIntervalMax)

	if err = getPersistentVolumeReconcilerSetupWithManager(&controller.PersistentVolumeReconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolume"),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(common.DellCSIReplicator),
		DriverName:        driverName,
		MigrationClient:   csimigration.New(csiConn, ctrl.Log.WithName("migration-client"), flags.operationTimeout),
		ContextPrefix:     flags.pgContextKeyPrefix,
		SingleFlightGroup: singleflight.Group{},
		Domain:            flags.domain,
		ReplDomain:        flags.replicationDomain,
	}, ctx, mgr, expRateLimiter, flags.workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PersistentVolume")
		osExit(1)
	}

	if err = getMigrationGroupReconcilerSetupWithManager(&controller.MigrationGroupReconciler{
		Client:                     mgr.GetClient(),
		Log:                        ctrl.Log.WithName("controllers").WithName("DellCSIMigrationGroup"),
		Scheme:                     mgr.GetScheme(),
		EventRecorder:              mgr.GetEventRecorderFor(common.DellCSIMigrator),
		DriverName:                 driverName,
		MigrationClient:            csimigration.New(csiConn, ctrl.Log.WithName("migration-client"), flags.operationTimeout),
		MaxRetryDurationForActions: flags.maxRetryDurationForActions,
	}, mgr, expRateLimiter, flags.workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DellCSIMigrationGroup")
		osExit(1)
	}

	setupLog.Info("starting manager")
	if err := getManagerStart(mgr); err != nil {
		setupLog.Error(err, "problem running manager")
		osExit(1)
	}
}
