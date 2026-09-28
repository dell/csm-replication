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
	"strconv"
	"strings"
	"time"

	metricscommon "github.com/dell/csm-metrics-common/pkg/server"
	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	repController "github.com/dell/csm-replication/controllers/replication-controller"
	"github.com/dell/csm-replication/internal/metrics"
	"github.com/dell/csm-replication/pkg/common/constants"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/dell/csmlog"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/spf13/viper"

	"github.com/dell/csm-replication/core"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()

	osExit = os.Exit

	getUpdateConfigOnSecretEvent = func(mgr *ControllerManager, ctx context.Context, request reconcile.Request, er record.EventRecorder) error {
		return mgr.config.UpdateConfigOnSecretEvent(ctx, mgr.Manager.GetClient(), mgr.Opts, request.Name, er)
	}
	getUpdateConfigMap = func(mgr *ControllerManager, ctx context.Context, er record.EventRecorder) error {
		return mgr.config.UpdateConfigMap(ctx, mgr.Manager.GetClient(), mgr.Opts, er)
	}
	getConnectionControllerClient = func(scheme *runtime.Scheme) (client.Client, error) {
		return connection.GetControllerClient(nil, scheme)
	}
	getConfig = func(ctx context.Context, client client.Client, opts config.ControllerManagerOpts, er record.EventRecorder) (*config.Config, error) {
		return config.GetConfig(ctx, client, opts, er)
	}
	getConfigPrintConfig = func(config *config.Config) {
		config.PrintConfig()
	}
	getManagerStart = func(mgr manager.Manager) error {
		return mgr.Start(ctrl.SetupSignalHandler())
	}
	getPersistentVolumeReconciler = func(r *repController.PersistentVolumeReconciler, mgr manager.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(mgr, limiter, maxReconcilers)
	}
	getReplicationGroupReconciler = func(r *repController.ReplicationGroupReconciler, mgr manager.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(mgr, limiter, maxReconcilers)
	}
	getPersistentVolumeClaimReconciler = func(r *repController.PersistentVolumeClaimReconciler, mgr manager.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
		return r.SetupWithManager(mgr, limiter, maxReconcilers)
	}
	getSecretController = func(controllerManager *ControllerManager) error {
		return controllerManager.startSecretController()
	}
	getCtrlNewManager = func(options manager.Options) (manager.Manager, error) {
		return ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	}

	watchConfigFunc    = viper.WatchConfig
	onConfigChangeFunc = viper.OnConfigChange

	newMetricsServerFunc   = metricscommon.NewMetricsServer
	startMetricsServerFunc = func(s *metricscommon.MetricsServer) error {
		return s.Start()
	}

	setupFlags = func() (map[string]string, context.Context) {
		var (
			retryIntervalStart     time.Duration
			retryIntervalMax       time.Duration
			workerThreads          int
			domain                 string
			disablePVCRemap        bool
			enableKubevirtPVCRemap bool
		)

		var metricsAddr string
		var enableLeaderElection bool
		var allowPVCCreationOnTarget bool

		flag.StringVar(&metricsAddr, "metrics-addr", ":8081", "The address the metric endpoint binds to.")
		flag.StringVar(&domain, "prefix", constants.DefaultDomain, "Prefix used for creating labels/annotations")
		flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
			"Enable leader election for dell-replication-controller manager. "+
				"Enabling this will ensure there is only one active dell-replication-controller manager.")
		flag.DurationVar(&retryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed reconcile request. It doubles with each failure, upto retry-interval-max")
		flag.DurationVar(&retryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed reconcile request")
		flag.IntVar(&workerThreads, "worker-threads", 2, "Number of concurrent reconcilers for each of the controllers")
		flag.BoolVar(&disablePVCRemap, "disable-pvc-remap", false, "disables PVC remapping functionality")
		flag.BoolVar(&enableKubevirtPVCRemap, "enable-kubevirt-pvc-remap", false, "enables KubeVirt PVC remapping functionality")
		flag.BoolVar(&allowPVCCreationOnTarget, "allow-pvc-creation-on-target", false, "allow PVC creation on target cluster")
		flag.Parse()

		csmlog.Infof("%s Version: %s, Creation Time: %s", constants.DellReplicationController, ManifestSemver, core.CommitTime.Format(time.RFC1123))

		csmlog.Infof("Prefix Domain: %s", domain)
		controllers.InitLabelsAndAnnotations(domain)

		flagMap := make(map[string]string)
		flagMap["metrics-addr"] = metricsAddr
		flagMap["leader-election"] = strconv.FormatBool(enableLeaderElection)
		flagMap["prefix"] = domain
		flagMap["retry-interval-start"] = retryIntervalStart.String()
		flagMap["retry-interval-max"] = retryIntervalMax.String()
		flagMap["worker-threads"] = strconv.Itoa(workerThreads)
		flagMap["disable-pvc-remap"] = strconv.FormatBool(disablePVCRemap)
		flagMap["enable-kubevirt-pvc-remap"] = strconv.FormatBool(enableKubevirtPVCRemap)
		flagMap["allow-pvc-creation-on-target"] = strconv.FormatBool(allowPVCCreationOnTarget)

		return flagMap, context.Background()
	}

	createManagerInstance = func(flagMap map[string]string) manager.Manager {
		mgr, err := getCtrlNewManager(ctrl.Options{
			Scheme: scheme,
			Metrics: metricsServer.Options{
				BindAddress: flagMap["metrics-addr"],
			},
			WebhookServer:              webhook.NewServer(webhook.Options{Port: 9443}),
			LeaderElection:             stringToBoolean(flagMap["leader-election"]),
			LeaderElectionResourceLock: "leases",
			LeaderElectionID:           fmt.Sprintf("%s-manager", constants.DellReplicationController),
		})
		if err != nil {
			csmlog.Errorf("unable to start manager: %v", err)
			osExit(1)
		}

		return mgr
	}

	setupControllerManager = func(ctx context.Context, mgr manager.Manager) *ControllerManager {
		controllerMgr, err := createControllerManager(ctx, mgr)
		if err != nil {
			csmlog.Errorf("failed to configure the controller manager: %v", err)
			osExit(1)
		}

		return controllerMgr
	}

	ManifestSemver string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(repv1.AddToScheme(scheme))
	utilruntime.Must(s1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// ControllerManager - Represents the controller manager and its configuration
type ControllerManager struct {
	Opts             config.ControllerManagerOpts
	Manager          ctrl.Manager
	SecretController controller.Controller
	config           *config.Config
}

func (mgr *ControllerManager) reconcileSecretUpdates(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	er := mgr.Manager.GetEventRecorderFor(constants.DellReplicationController)
	err := getUpdateConfigOnSecretEvent(mgr, ctx, request, er)
	if err != nil {
		csmlog.Errorf("failed to update the configuration: %v", err)
	}
	return reconcile.Result{}, nil
}

func (mgr *ControllerManager) startSecretController() error {
	secretController, err := controller.New("secret-controller", mgr.Manager, controller.Options{
		Reconciler: reconcile.Func(mgr.reconcileSecretUpdates),
	})
	mgr.SecretController = secretController
	if err != nil {
		return err
	}
	err = secretController.Watch(source.Kind(mgr.Manager.GetCache(), &corev1.Secret{}, &handler.TypedEnqueueRequestForObject[*corev1.Secret]{},
		predicate.NewTypedPredicateFuncs[*corev1.Secret](func(object *corev1.Secret) bool {
			return object.GetNamespace() == mgr.Opts.WatchNamespace
		})))

	return err
}

func (mgr *ControllerManager) processConfigMapChanges() {
	csmlog.Info("Received a config change event")
	er := mgr.Manager.GetEventRecorderFor(constants.DellReplicationController)
	err := getUpdateConfigMap(mgr, context.Background(), er)
	if err != nil {
		csmlog.Errorf("Error parsing the config: %v", err)
		return
	}
	mgr.config.Lock.Lock()
	defer mgr.config.Lock.Unlock()
	setLogLevel(mgr.config.LogLevel)
	setLogFormat(mgr.config.LogFormat)
}

func (mgr *ControllerManager) setupConfigMapWatcher() {
	csmlog.Info("Started ConfigMap Watcher")
	watchConfigFunc()
	onConfigChangeFunc(func(_ fsnotify.Event) {
		mgr.processConfigMapChanges()
	})
}

func createControllerManager(ctx context.Context, mgr ctrl.Manager) (*ControllerManager, error) {
	opts := config.GetControllerManagerOpts()
	opts.Mode = "controller"
	// We need to create a new client as the informer caches have not started yet
	client, err := getConnectionControllerClient(scheme)
	if err != nil {
		return nil, err
	}
	er := mgr.GetEventRecorderFor(constants.DellReplicationController)
	repConfig, err := getConfig(ctx, client, opts, er)
	if err != nil {
		return nil, err
	}
	getConfigPrintConfig(repConfig)
	controllerManager := ControllerManager{
		Opts:    opts,
		Manager: mgr,
		config:  repConfig,
	}
	return &controllerManager, nil
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;watch;list;delete;update;create
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

func main() {
	flagMap, ctx := setupFlags()

	// Set controller-runtime logger to discard to prevent goroutine error
	// controller-runtime requires a global logger to be set; we discard its internal logs
	// and use csmlog for all application-specific logging instead
	ctrl.SetLogger(logr.Discard())

	// Create the manager instance
	mgr := createManagerInstance(flagMap)

	if mgr != nil {
		controllerMgr := setupControllerManager(ctx, mgr)
		if controllerMgr != nil {
			// Start the watch on configmap
			controllerMgr.setupConfigMapWatcher()

			// Process the config. Get initial log level and format
			setLogLevel(controllerMgr.config.LogLevel)
			setLogFormat(controllerMgr.config.LogFormat)

			initReplicationMetrics()

			// Log controller-runtime startup (mimics controller-runtime logging)
			csmlog.Info("Starting manager")
			csmlog.Info("Starting controller-runtime")

			// Start the secret controller
			// Log controller startup (mimics controller-runtime logging)
			csmlog.Info("Starting Secret controller")
			startSecretController(controllerMgr)

			// Create PersistentVolumeClaimReconciler
			// Log controller startup (mimics controller-runtime logging)
			csmlog.Info("Starting PersistentVolumeClaim controller")
			expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](stringToTimeDuration(flagMap["retry-interval-start"]), stringToTimeDuration(flagMap["retry-interval-max"]))
			createPersistentVolumeClaimReconciler(mgr, controllerMgr, flagMap["prefix"], stringToInt(flagMap["worker-threads"]), expRateLimiter, stringToBoolean(flagMap["allow-pvc-creation-on-target"]))

			// Create ReplicationGroupReconciler
			// Log controller startup (mimics controller-runtime logging)
			csmlog.Info("Starting ReplicationGroup controller")
			createReplicationGroupReconciler(mgr, controllerMgr, flagMap["prefix"], stringToInt(flagMap["worker-threads"]), expRateLimiter, stringToBoolean(flagMap["disable-pvc-remap"]), stringToBoolean(flagMap["enable-kubevirt-pvc-remap"]))

			// Create PersistentVolumeReconciler
			// Log controller startup (mimics controller-runtime logging)
			csmlog.Info("Starting PersistentVolume controller")
			createPersistentVolumeReconciler(mgr, controllerMgr, flagMap["prefix"], stringToInt(flagMap["worker-threads"]), expRateLimiter)

			// Log worker thread configuration (mimics controller-runtime logging)
			csmlog.Infof("Starting workers with %d threads", stringToInt(flagMap["worker-threads"]))
		}

		// +kubebuilder:scaffold:builder

		// start manager
		startManager(mgr)
	}
}

func startManager(mgr manager.Manager) {
	// Log manager start (mimics controller-runtime logging)
	csmlog.Info("starting manager")

	if err := getManagerStart(mgr); err != nil {
		csmlog.Errorf("problem running manager: %v", err)
		osExit(1)
	}
}

func createPersistentVolumeReconciler(mgr manager.Manager, controllerMgr *ControllerManager, domain string, workerThreads int, expRateLimiter workqueue.TypedRateLimiter[reconcile.Request]) {
	// PV Controller
	if err := getPersistentVolumeReconciler(&repController.PersistentVolumeReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor(constants.DellReplicationController),
		Config:        controllerMgr.config,
		Domain:        domain,
	}, mgr, expRateLimiter, workerThreads); err != nil {
		csmlog.Errorf("unable to create controller %s PersistentVolume", constants.DellReplicationController)
		csmlog.Errorf("unable to create controller: %v", err)
		osExit(1)
	}
}

func createReplicationGroupReconciler(mgr manager.Manager, controllerMgr *ControllerManager, domain string, workerThreads int, expRateLimiter workqueue.TypedRateLimiter[reconcile.Request], disablePVCRemap bool, enableKubevirtPVCRemap bool) {
	if err := getReplicationGroupReconciler(&repController.ReplicationGroupReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		EventRecorder:          mgr.GetEventRecorderFor(constants.DellReplicationController),
		Config:                 controllerMgr.config,
		Domain:                 domain,
		DisablePVCRemap:        disablePVCRemap,
		EnableKubevirtPVCRemap: enableKubevirtPVCRemap,
	}, mgr, expRateLimiter, workerThreads); err != nil {
		csmlog.Errorf("unable to create controller: %v", err)
		osExit(1)
	}
}

func createPersistentVolumeClaimReconciler(mgr manager.Manager, controllerMgr *ControllerManager, domain string, workerThreads int, expRateLimiter workqueue.TypedRateLimiter[reconcile.Request], allowPVCCreationOnTarget bool) {
	if err := getPersistentVolumeClaimReconciler(&repController.PersistentVolumeClaimReconciler{
		Client:                   mgr.GetClient(),
		Scheme:                   mgr.GetScheme(),
		EventRecorder:            mgr.GetEventRecorderFor(constants.DellReplicationController),
		Config:                   controllerMgr.config,
		Domain:                   domain,
		AllowPVCCreationOnTarget: allowPVCCreationOnTarget,
	}, mgr, expRateLimiter, workerThreads); err != nil {
		csmlog.Errorf("unable to create controller: %v", err)
		osExit(1)
	}
}

func startSecretController(controllerMgr *ControllerManager) {
	err := getSecretController(controllerMgr)
	if err != nil {
		csmlog.Errorf("failed to setup secret controller. Continuing: %v", err)
	}
}

func setLogLevel(logLevel string) {
	normalizedLogLevel := strings.ToLower(strings.TrimSpace(logLevel))
	if normalizedLogLevel == "" {
		return
	}
	level, err := csmlog.ParseLevel(normalizedLogLevel)
	if err != nil {
		csmlog.Errorf("Unable to parse log level: %v", err)
		return
	}
	csmlog.Infof("set level to %v", level)
	csmlog.SetLevel(level)
}

func setLogFormat(logFormat string) {
	if logFormat != "" {
		switch strings.ToLower(logFormat) {
		case "json", "text":
			csmlog.Infof("set format to %v", strings.ToLower(logFormat))
			csmlog.SetFormat(strings.ToLower(logFormat))
		default:
			csmlog.Errorf("invalid log format %q, falling back to json", logFormat)
			csmlog.SetFormat("json")
		}
	}
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

func initReplicationMetrics() {
	if os.Getenv(constants.EnvReplicationMetricsEnabled) != "true" {
		csmlog.Info("Replication metrics are disabled")
		return
	}

	driverName := constants.DellReplicationController

	registry := prometheus.NewRegistry()
	replMetrics := metrics.NewReplicationMetrics(registry)
	metrics.SetGlobalReplicationMetrics(replMetrics)
	replMetrics.SetControllerHealth(driverName, true)

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
