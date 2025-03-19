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
	"strconv"
	"time"

	"github.com/bombsimon/logrusr/v4"
	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	repController "github.com/dell/csm-replication/controllers/replication-controller"
	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
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
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	osExit = os.Exit

	getSecretControllerLogger = func(mgr *ControllerManager, request reconcile.Request) logr.Logger {
		return mgr.SecretController.GetLogger().WithName(request.Name)
	}
	getUpdateConfigOnSecretEvent = func(mgr *ControllerManager, ctx context.Context, request reconcile.Request, er record.EventRecorder, secretLog logr.Logger) error {
		return mgr.config.UpdateConfigOnSecretEvent(ctx, mgr.Manager.GetClient(), mgr.Opts, request.Name, er, secretLog)
	}
	getUpdateConfigMap = func(mgr *ControllerManager, ctx context.Context, er record.EventRecorder) error {
		return mgr.config.UpdateConfigMap(ctx, mgr.Manager.GetClient(), mgr.Opts, er, mgr.Manager.GetLogger())
	}
	getConnectionControllerClient = func(scheme *runtime.Scheme) (client.Client, error) {
		return connection.GetControllerClient(nil, scheme)
	}
	getConfig = func(ctx context.Context, client client.Client, opts config.ControllerManagerOpts, er record.EventRecorder, mgrLogger logr.Logger) (*config.Config, error) {
		return config.GetConfig(ctx, client, opts, er, mgrLogger)
	}
	getConfigPrintConfig = func(config *config.Config, logger logr.Logger) {
		config.PrintConfig(logger)
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

	setupFlags = func() (map[string]string, logr.Logger, *logrus.Logger, context.Context) {
		var (
			retryIntervalStart time.Duration
			retryIntervalMax   time.Duration
			workerThreads      int
			domain             string
		)

		var metricsAddr string
		var enableLeaderElection bool

		flag.StringVar(&metricsAddr, "metrics-addr", ":8081", "The address the metric endpoint binds to.")
		flag.StringVar(&domain, "prefix", common.DefaultDomain, "Prefix used for creating labels/annotations")
		flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
			"Enable leader election for dell-replication-controller manager. "+
				"Enabling this will ensure there is only one active dell-replication-controller manager.")
		flag.DurationVar(&retryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed reconcile request. It doubles with each failure, upto retry-interval-max")
		flag.DurationVar(&retryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed reconcile request")
		flag.IntVar(&workerThreads, "worker-threads", 2, "Number of concurrent reconcilers for each of the controllers")
		flag.Parse()

		logrusLog := logrus.New()
		logrusLog.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})

		logger := logrusr.New(logrusLog)
		ctrl.SetLogger(logger)
		setupLog.V(common.InfoLevel).Info(common.DellReplicationController, "Version", core.SemVer, "Commit ID", core.CommitSha32, "Commit SHA", core.CommitTime.Format(time.RFC1123))

		setupLog.V(common.InfoLevel).Info("Prefix", "Domain", domain)
		controllers.InitLabelsAndAnnotations(domain)

		flagMap := make(map[string]string)
		flagMap["metrics-addr"] = metricsAddr
		flagMap["leader-election"] = strconv.FormatBool(enableLeaderElection)
		flagMap["prefix"] = domain
		flagMap["retry-interval-start"] = retryIntervalStart.String()
		flagMap["retry-interval-max"] = retryIntervalMax.String()
		flagMap["worker-threads"] = strconv.Itoa(workerThreads)

		return flagMap, setupLog, logrusLog, context.Background()
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
			LeaderElectionID:           fmt.Sprintf("%s-manager", common.DellReplicationController),
		})
		if err != nil {
			setupLog.Error(err, "unable to start manager")
			osExit(1)
		}

		return mgr
	}

	setupControllerManager = func(ctx context.Context, mgr manager.Manager, setupLog logr.Logger) *ControllerManager {
		controllerMgr, err := createControllerManager(ctx, mgr)
		if err != nil {
			setupLog.Error(err, "failed to configure the controller manager")
			osExit(1)
		}

		return controllerMgr
	}
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
	secretLog := getSecretControllerLogger(mgr, request)

	er := mgr.Manager.GetEventRecorderFor(common.DellReplicationController)
	err := getUpdateConfigOnSecretEvent(mgr, ctx, request, er, secretLog)
	if err != nil {
		secretLog.Error(err, "failed to update the configuration")
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

func (mgr *ControllerManager) processConfigMapChanges(loggerConfig *logrus.Logger) {
	loggerConfig.Info("Received a config change event")
	er := mgr.Manager.GetEventRecorderFor(common.DellReplicationController)
	err := getUpdateConfigMap(mgr, context.Background(), er)
	if err != nil {
		log.Printf("Error parsing the config: %v\n", err)
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

func (mgr *ControllerManager) setupConfigMapWatcher(loggerConfig *logrus.Logger) {
	loggerConfig.Info("Started ConfigMap Watcher")
	viper.WatchConfig()
	viper.OnConfigChange(func(_ fsnotify.Event) {
		mgr.processConfigMapChanges(loggerConfig)
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
	mgrLogger := mgr.GetLogger()

	er := mgr.GetEventRecorderFor(common.DellReplicationController)
	repConfig, err := getConfig(ctx, client, opts, er, mgrLogger)
	if err != nil {
		return nil, err
	}
	getConfigPrintConfig(repConfig, mgrLogger)
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
	flagMap, setupLog, logrusLog, ctx := setupFlags()

	// Create the manager instance
	mgr := createManagerInstance(flagMap)

	if mgr != nil {
		controllerMgr := setupControllerManager(ctx, mgr, setupLog)
		if controllerMgr != nil {
			// Start the watch on configmap
			controllerMgr.setupConfigMapWatcher(logrusLog)

			// Process the config. Get initial log level
			processLogLevel(controllerMgr.config.LogLevel, logrusLog)

			// Start the secret controller
			startSecretController(controllerMgr, setupLog)

			// Create PersistentVolumeClaimReconciler
			expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](stringToTimeDuration(flagMap["retry-interval-start"]), stringToTimeDuration(flagMap["retry-interval-max"]))
			createPersistentVolumeClaimReconciler(mgr, controllerMgr, flagMap["prefix"], stringToInt(flagMap["worker-threads"]), expRateLimiter, setupLog)

			// Create ReplicationGroupReconciler
			createReplicationGroupReconciler(mgr, controllerMgr, flagMap["prefix"], stringToInt(flagMap["worker-threads"]), expRateLimiter, setupLog)

			// Create PersistentVolumeReconciler
			createPersistentVolumeReconciler(mgr, controllerMgr, flagMap["prefix"], stringToInt(flagMap["worker-threads"]), expRateLimiter, setupLog)

		}

		// +kubebuilder:scaffold:builder

		// start manager
		startManager(mgr, setupLog)
	}
}

func startManager(mgr manager.Manager, setupLog logr.Logger) {
	setupLog.Info("starting manager")

	if err := getManagerStart(mgr); err != nil {
		log.Println("problem running manager")
		setupLog.Error(err, "problem running manager")
		osExit(1)
	}
}

func createPersistentVolumeReconciler(mgr manager.Manager, controllerMgr *ControllerManager, domain string, workerThreads int, expRateLimiter workqueue.TypedRateLimiter[reconcile.Request], setupLog logr.Logger) {
	// PV Controller
	if err := getPersistentVolumeReconciler(&repController.PersistentVolumeReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("PersistentVolume"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor(common.DellReplicationController),
		Config:        controllerMgr.config,
		Domain:        domain,
	}, mgr, expRateLimiter, workerThreads); err != nil {
		log.Println("unable to create controller", common.DellReplicationController, "PersistentVolume")
		setupLog.Error(err, "unable to create controller", common.DellReplicationController, "PersistentVolume")
		osExit(1)
	}
}

func createReplicationGroupReconciler(mgr manager.Manager, controllerMgr *ControllerManager, domain string, workerThreads int, expRateLimiter workqueue.TypedRateLimiter[reconcile.Request], setupLog logr.Logger) {
	if err := getReplicationGroupReconciler(&repController.ReplicationGroupReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor(common.DellReplicationController),
		Config:        controllerMgr.config,
		Domain:        domain,
	}, mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", common.DellReplicationController, "DellCSIReplicationGroup")
		osExit(1)
	}
}

func createPersistentVolumeClaimReconciler(mgr manager.Manager, controllerMgr *ControllerManager, domain string, workerThreads int, expRateLimiter workqueue.TypedRateLimiter[reconcile.Request], setupLog logr.Logger) {
	if err := getPersistentVolumeClaimReconciler(&repController.PersistentVolumeClaimReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor(common.DellReplicationController),
		Config:        controllerMgr.config,
		Domain:        domain,
	}, mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "unable to create controller", common.DellReplicationController, "PersistentVolumeClaim")
		osExit(1)
	}
}

func startSecretController(controllerMgr *ControllerManager, setupLog logr.Logger) {
	err := getSecretController(controllerMgr)
	if err != nil {
		setupLog.Error(err, "failed to setup secret controller. Continuing")
	}
}

func processLogLevel(logLevel string, logrusLog *logrus.Logger) {
	level, err := common.ParseLevel(logLevel)
	if err != nil {
		logrusLog.Error("Unable to parse ", err)
	}
	logrusLog.Info("set level to", level)
	logrusLog.SetLevel(level)
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
