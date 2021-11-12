/*
Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	repV1Alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	repController "github.com/dell/csm-replication/controllers/replication-controller"
	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/config"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/fsnotify/fsnotify"
	"github.com/maxan98/logrusr"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/workqueue"
	"log"
	"os"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	"github.com/spf13/viper"

	"github.com/dell/csm-replication/core"
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
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(repV1Alpha1.AddToScheme(scheme))
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
	secretLog := mgr.SecretController.GetLogger().WithName(request.Name)
	er := mgr.Manager.GetEventRecorderFor(common.DellReplicationController)
	err := mgr.config.UpdateConfigOnSecretEvent(ctx, mgr.Manager.GetClient(), mgr.Opts, request.Name, er, secretLog)
	if err != nil {
		secretLog.Error(err, "Failed to update the configuration")
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
	err = secretController.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(func(object ctrlClient.Object) bool {
			return object.GetNamespace() == mgr.Opts.WatchNamespace
		}))

	return err
}

func (mgr *ControllerManager) processConfigMapChanges(loggerConfig *logrus.Logger) {
	log.Println("Received a config change event")
	er := mgr.Manager.GetEventRecorderFor(common.DellReplicationController)
	err := mgr.config.UpdateConfigMap(context.Background(), mgr.Manager.GetClient(), mgr.Opts, er, mgr.Manager.GetLogger())
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
	log.Println("Set level to", level)
	loggerConfig.SetLevel(level)
}

func (mgr *ControllerManager) setupConfigMapWatcher(loggerConfig *logrus.Logger) {
	log.Println("Started ConfigMap Watcher")
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		mgr.processConfigMapChanges(loggerConfig)
	})
}

func createControllerManager(ctx context.Context, mgr ctrl.Manager) (*ControllerManager, error) {
	opts := config.GetControllerManagerOpts()
	// We need to create a new client as the informer caches have not started yet
	client, err := connection.GetControllerClient(nil, scheme)
	if err != nil {
		return nil, err
	}
	mgrLogger := mgr.GetLogger()
	er := mgr.GetEventRecorderFor(common.DellReplicationController)
	repConfig, err := config.GetConfig(ctx, client, opts, er, mgrLogger)
	if err != nil {
		return nil, err
	}
	repConfig.PrintConfig(mgrLogger)
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
	flag.Parse()
	setupLog.V(common.InfoLevel).Info("Prefix", "Domain", domain)
	controllers.InitLabelsAndAnnotations(domain)

	logrusLog := logrus.New()
	logrusLog.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})

	logger := logrusr.NewLogger(logrusLog)
	ctrl.SetLogger(logger)
	setupLog.V(common.InfoLevel).Info(common.DellReplicationController, "Version", core.SemVer, "Commit ID", core.CommitSha32, "Commit SHA", core.CommitTime.Format(time.RFC1123))

	ctx := context.Background()

	// Create the manager instance
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         metricsAddr,
		Port:                       9443,
		LeaderElection:             enableLeaderElection,
		LeaderElectionResourceLock: "leases",
		LeaderElectionID:           fmt.Sprintf("%s-manager", common.DellReplicationController),
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	controllerMgr, err := createControllerManager(ctx, mgr)
	if err != nil {
		setupLog.Error(err, "Filed to configure the controller manager")
		os.Exit(1)
	}

	// Start the watch on configmap
	controllerMgr.setupConfigMapWatcher(logrusLog)

	// Process the config. Get initial log level
	level, err := common.ParseLevel(controllerMgr.config.LogLevel)
	if err != nil {
		log.Println("Unable to parse ", err)
	}
	log.Println("Set level to", level)
	logrusLog.SetLevel(level)

	// Start the secret controller
	err = controllerMgr.startSecretController()
	if err != nil {
		setupLog.Error(err, "Failed to setup secret controller. Continuing")
	}

	expRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(retryIntervalStart, retryIntervalMax)
	if err = (&repController.PersistentVolumeClaimReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor(common.DellReplicationController),
		Config:        controllerMgr.config,
		Domain:        domain,
	}).SetupWithManager(mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "Unable to create controller", common.DellReplicationController, "PersistentVolumeClaim")
		os.Exit(1)
	}

	if err = (&repController.ReplicationGroupReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("DellCSIReplicationGroup"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor(common.DellReplicationController),
		Config:        controllerMgr.config,
		Domain:        domain,
	}).SetupWithManager(mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "Unable to create controller", common.DellReplicationController, "DellCSIReplicationGroup")
		os.Exit(1)
	}

	// PV Controller
	if err = (&repController.PersistentVolumeReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("PersistentVolume"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor(common.DellReplicationController),
		Config:        controllerMgr.config,
		Domain:        domain,
	}).SetupWithManager(mgr, expRateLimiter, workerThreads); err != nil {
		setupLog.Error(err, "Unable to create controller", common.DellReplicationController, "PersistentVolume")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	setupLog.Info("Starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
