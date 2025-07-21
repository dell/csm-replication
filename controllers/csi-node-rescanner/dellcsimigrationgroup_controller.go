/*
Copyright © 2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package csinoderescanner

import (
	"context"
	"fmt"
	"time"

	storagev1 "github.com/dell/csm-replication/api/v1"
	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common/logger"
	"github.com/dell/csm-replication/pkg/utils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	reconciler "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// DeletingState name deletion state of MigrationGroup
	DeletingState = "Deleting"
	// MigratedState name of post migrate call state of MigrationGroup
	MigratedState = "Migrated"
	// MaxRetryDurationForActions maximum amount of time between retries of failed action
	MaxRetryDurationForActions = 1 * time.Hour
	// NodeLabelFilter is filter to get all pmax nodes
	NodeLabelFilter = "powermax-node"
)

// ActionType refers to the next action
type ActionType string

// NodeRescanReconciler reconciles PersistentVolume resources
type NodeRescanReconciler struct {
	Client                     client.Client
	Log                        logr.Logger
	Scheme                     *runtime.Scheme
	EventRecorder              record.EventRecorder
	DriverName                 string
	NodeName                   string
	MaxRetryDurationForActions time.Duration
}

// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsimigrationgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=replication.storage.dell.com,resources=dellcsimigrationgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch

var myNode *corev1.Pod

// Reconcile contains reconciliation logic that updates MigrationGroup depending on it's current state
func (r *NodeRescanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("MigrationGroup", req.NamespacedName)
	ctx = context.WithValue(ctx, logger.LoggerContextKey, log)

	log.V(logger.InfoLevel).Info("Begin reconcile - Node ReScanner")

	mg := new(storagev1.DellCSIMigrationGroup)
	err := r.Client.Get(ctx, req.NamespacedName, mg)
	if err != nil {
		log.Error(err, "MG not found", "mg", mg)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	currentState := mg.Status.State

	switch currentState {
	case DeletingState:
		return r.processMGinDeletingState(ctx, mg.DeepCopy())
	case MigratedState:
		return r.processMGForRescan(ctx, mg.DeepCopy())
	default:
		log.Info(fmt.Sprintf("Ignoring MG (%s) for rescan in %s state", mg.Name, currentState))
		return ctrl.Result{}, nil
	}
}

// Update mg spec with current state
func (r *NodeRescanReconciler) processMGForRescan(ctx context.Context, mg *storagev1.DellCSIMigrationGroup) (ctrl.Result, error) {
	// Get self POD details
	// Get all Node Pods in driver's namespace
	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.MatchingLabels{"app": NodeLabelFilter},
	}
	err := r.Client.List(ctx, podList, opts...)
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	log := logger.GetLoggerFromContext(ctx)
	// Check if rescanned label is already on the pod
	for _, pod := range podList.Items {
		if pod.Spec.NodeName == r.NodeName {
			myNode = pod.DeepCopy()
			log.V(logger.DebugLevel).Info(fmt.Sprintf("Found node: %+v", myNode))
			labels := pod.GetLabels()
			if _, ok := labels[controller.NodeReScanned]; ok {
				r.Log.Info("rescan done on node: ", r.NodeName)
				return ctrl.Result{}, nil
			}
		}
	}
	if myNode.Name == "" {
		return ctrl.Result{}, errors.NewBadRequest("no node name found")
	}

	log.V(logger.DebugLevel).Info("Begin rescan on node for MG spec", "Name: ", mg.Name, "Node:", myNode.Name)
	// Perform rescan on the node
	err = utils.RescanNode(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Update label on the node
	controller.AddLabel(myNode, controller.NodeReScanned, "yes")
	log.V(logger.InfoLevel).Info("Updating", "label", "yes")
	err = r.Client.Update(ctx, myNode)
	if err != nil {
		log.Error(err, "Failed to update", "label", "yes")
		return ctrl.Result{}, err
	}
	log.V(logger.InfoLevel).Info("Pod was successfully updated with", "Node-Rescanned", "yes")
	return ctrl.Result{}, err
}

// Update mg spec with current state
func (r *NodeRescanReconciler) processMGinDeletingState(ctx context.Context, mg *storagev1.DellCSIMigrationGroup) (ctrl.Result, error) {
	// Get self POD details
	// Get all Node Pods in driver's namespace
	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.MatchingLabels{"app": NodeLabelFilter},
	}
	err := r.Client.List(ctx, podList, opts...)
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	log := logger.GetLoggerFromContext(ctx)
	// Check if rescanned label is already on the pod
	for _, pod := range podList.Items {
		if pod.Spec.NodeName == r.NodeName {
			myNode = pod.DeepCopy()
			log.V(logger.DebugLevel).Info(fmt.Sprintf("Found node: %+v", myNode))
			labels := pod.GetLabels()
			if _, ok := labels[controller.NodeReScanned]; ok {
				// Remove label from the pod
				log.V(logger.DebugLevel).Info("Begin deletion of label on node for MG spec", "Name: ", mg.Name, "Node:", myNode.Name)
				// Update label on the node
				controller.DeleteLabel(myNode, controller.NodeReScanned)
				log.V(logger.InfoLevel).Info("deleting", "label:", controller.NodeReScanned)
				err = r.Client.Update(ctx, myNode)
				if err != nil {
					log.Error(err, "Failed to delete", "label", controller.NodeReScanned)
					return ctrl.Result{}, err
				}
				log.V(logger.InfoLevel).Info("Pod was successfully updated with", "Node-Rescanned", "nil")
				return ctrl.Result{}, nil
			}
		}
	}
	return ctrl.Result{}, err
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *NodeRescanReconciler) SetupWithManager(mgr ctrl.Manager, limiter workqueue.TypedRateLimiter[reconcile.Request], maxReconcilers int) error {
	if r.MaxRetryDurationForActions == 0 {
		r.MaxRetryDurationForActions = MaxRetryDurationForActions
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&storagev1.DellCSIMigrationGroup{}).
		WithOptions(reconciler.Options{
			RateLimiter:             limiter,
			MaxConcurrentReconciles: maxReconcilers,
		}).
		Complete(r)
}
