/*
Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"github.com/dell/dell-csi-extensions/migration"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"log"

	"github.com/dell/csm-replication/pkg/config"
	"github.com/fsnotify/fsnotify"
	"github.com/maxan98/logrusr"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"os"
	"strings"
	"time"

	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"

	"golang.org/x/sync/singleflight"

	controller "github.com/dell/csm-replication/controllers/csi-migrator"

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
	// +kubebuilder:scaffold:scheme
}

// MigratorManager - Represents the controller manager and its configuration
type MigratorManager struct {
	Opts    config.ControllerManagerOpts
	Manager ctrl.Manager
	config  *config.Config
}

func (mgr *MigratorManager) processConfigMapChanges(loggerConfig *logrus.Logger) {
	log.Println("Received a config change event")
	err := mgr.config.UpdateConfigMap(context.Background(), nil, mgr.Opts, nil, mgr.Manager.GetLogger())
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
	viper.OnConfigChange(func(e fsnotify.Event) {
		mgr.processConfigMapChanges(loggerConfig)
	})
}

func createMigratorManager(ctx context.Context, mgr ctrl.Manager) (*MigratorManager, error) {
	opts := config.GetControllerManagerOpts()
	opts.Mode = "sidecar"
	mgrLogger := mgr.GetLogger()
	repConfig, err := config.GetConfig(ctx, nil, opts, nil, mgrLogger)
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

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		csiAddress           string
		workerThreads        int
		retryIntervalStart   time.Duration
		retryIntervalMax     time.Duration
		operationTimeout     time.Duration
		pgContextKeyPrefix   string
		domain               string
		replicationDomain    string
		probeFrequency       time.Duration
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
	flag.Parse()
	controllers.InitLabelsAndAnnotations(domain)
	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})

	logger := logrusr.NewLogger(logrusLog)
	ctrl.SetLogger(logger)

	setupLog.V(1).Info("Prefix", "Domain", domain)
	setupLog.V(1).Info(common.DellCSIMigrator, "Version", core.SemVer, "Commit ID", core.CommitSha32, "Commit SHA", core.CommitTime.Format(time.RFC1123))

	ctx := context.Background()

	// Connect to csi
	csiConn, err := connection.Connect(csiAddress, setupLog)
	if err != nil {
		setupLog.Error(err, "failed to connect to CSI driver")
		os.Exit(1)
	}

	identityClient := csiidentity.New(csiConn, ctrl.Log.WithName("identity-client"), operationTimeout, probeFrequency)

	driverName, err := identityClient.ProbeForever(ctx)
	if err != nil {
		setupLog.Error(err, "error waiting for the CSI driver to be ready")
		os.Exit(1)
	}
	setupLog.V(1).Info("CSI driver name", "driverName", driverName)

	capabilitySet, err := identityClient.GetMigrationCapabilities(ctx)
	if err != nil {
		setupLog.Error(err, "error fetching migration capabilities")
		os.Exit(1)
	}
	if len(capabilitySet) == 0 {
		setupLog.Error(fmt.Errorf("driver doesn't support migration"), "migration not supported")
		os.Exit(1)
	}
	for types := range capabilitySet {
		if _, ok := currentSupportedCapabilities[types]; !ok {
			setupLog.Error(err, "unknown capability advertised")
			os.Exit(1)
		}
	}
	leaderElectionID := common.DellCSIMigrator + strings.ReplaceAll(driverName, ".", "-")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         metricsAddr,
		Port:                       8443,
		LeaderElection:             enableLeaderElection,
		LeaderElectionResourceLock: "leases",
		LeaderElectionID:           leaderElectionID,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	MigratorMgr, err := createMigratorManager(ctx, mgr)
	if err != nil {
		setupLog.Error(err, "failed to configure the migrator manager")
		os.Exit(1)
	}
	// Start the watch on configmap
	MigratorMgr.setupConfigMapWatcher(logrusLog)

	// Process the config. Get initial log level
	level, err := common.ParseLevel(MigratorMgr.config.LogLevel)
	if err != nil {
		log.Println("Unable to parse ", err)
	}
	log.Println("set level to", level)
	logrusLog.SetLevel(level)

	expRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(retryIntervalStart, retryIntervalMax)

	if err = (&controller.PersistentVolumeReconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("PersistentVolume"),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor(common.DellCSIReplicator),
		DriverName:        driverName,
		MigrationClient:   csimigration.New(csiConn, ctrl.Log.WithName("migration-client"), operationTimeout),
		ContextPrefix:     pgContextKeyPrefix,
		SingleFlightGroup: singleflight.Group{},
		Domain:            domain,
		ReplDomain:        replicationDomain,
	}).SetupWithManager(ctx, mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PersistentVolume")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}
