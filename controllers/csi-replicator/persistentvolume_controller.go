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

package csireplicator

import (
	"context"
	"encoding/json"
	"fmt"
	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	v1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	reconcile "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"
	"strings"
)

// PersistentVolumeReconciler reconciles PersistentVolume resources
type PersistentVolumeReconciler struct {
	client.Client
	Log               logr.Logger
	Scheme            *runtime.Scheme
	EventRecorder     record.EventRecorder
	DriverName        string
	ReplicationClient csireplication.Replication
	ContextPrefix     string
	SingleFlightGroup singleflight.Group
	Domain            string
}

const protectionIndexKey = "protection_id"

// Reconcile contains reconciliation logic that updates PersistentVolume depending on it's current state
func (r *PersistentVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("persistentvolume", req.NamespacedName)
	ctx = context.WithValue(ctx, common.LoggerContextKey, log)

	log.V(common.InfoLevel).Info("Begin reconcile - PV Controller")

	pv := new(v1.PersistentVolume)
	if err := r.Get(ctx, req.NamespacedName, pv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	storageClass := new(storageV1.StorageClass)
	err := r.Get(ctx, client.ObjectKey{
		Name: pv.Spec.StorageClassName,
	}, storageClass)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "The storage class specified in the PV doesn't exist", "StorageClassName", pv.Spec.StorageClassName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch storage class of the PV", "StorageClassName", pv.Spec.StorageClassName)
		return ctrl.Result{}, err
	}

	if !shouldContinue(storageClass, log, r.DriverName) {
		return ctrl.Result{}, nil
	}

	var buffer []byte
	isPVUpdated := false
	if _, ok := pv.Annotations[controller.CreatedBy]; !ok {
		if _, ok = pv.Annotations[controller.RemoteVolumeAnnotation]; !ok {
			res, err := r.ReplicationClient.CreateRemoteVolume(ctx, pv.Spec.CSI.VolumeHandle, storageClass.Parameters)
			if err != nil {
				log.Error(err, "Failed to create the remote volume", "RemoteVolume", res.RemoteVolume)
				return ctrl.Result{}, err
			}
			buffer, err = json.Marshal(res.RemoteVolume)
			if err != nil {
				log.Error(err, "Failed to marshal", "RemoteVolume", res.RemoteVolume)
				return ctrl.Result{}, err
			}
			pvObj := pv.DeepCopy()
			controller.AddAnnotation(pvObj, controller.RemoteVolumeAnnotation, string(buffer))
			if err := r.Update(ctx, pvObj); err != nil {
				log.Error(err, "Failed to add label and annotation to the PV", string(buffer))
				return ctrl.Result{}, err
			}
			isPVUpdated = true

		}
	}

	var (
		replicationGroupName string
	)

	log.V(common.InfoLevel).Info("Adding replication-group, remote storage class annotation and label to the PersistentVolume")

	if _, ok := pv.Annotations[controller.ReplicationGroup]; !ok {
		if _, ok = pv.Annotations[controller.CreatedBy]; ok {
			log.V(common.DebugLevel).Info("This PV was created by sync controller. It is expected to have RG annotation. Re-queuing..")
			return ctrl.Result{Requeue: true, RequeueAfter: controller.DefaultRetryInterval}, nil
		}
		if replicationGroupName, err = r.createProtectionGroupAndRG(ctx, pv.Spec.CSI.VolumeHandle, storageClass.Parameters); err != nil {
			return ctrl.Result{}, err
		}

		if replicationGroupName == "" {
			log.V(common.DebugLevel).Info("In corner cases we have seen RGName being empty, in that case retry..")
			return ctrl.Result{Requeue: true, RequeueAfter: controller.DefaultRetryInterval}, nil
		}
		if isPVUpdated {
			if err := r.Get(ctx, req.NamespacedName, pv); err != nil {
				log.Error(err, "Failed to get PV", "pv", pv, "NamespacedName", req.NamespacedName)
				return ctrl.Result{}, err
			}
		}
		if err := r.processVolumeForReplicationGroup(ctx, pv, replicationGroupName,
			log, storageClass.Parameters); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.V(common.DebugLevel).Info("Checking if PV protection complete annotation is applied")

	if _, ok := pv.Annotations[controller.PVProtectionComplete]; !ok {
		controller.AddAnnotation(pv, controller.PVProtectionComplete, "yes")
		if err := r.Update(ctx, pv); err != nil {
			log.Error(err, "Failed to add PV protection complete annotation to the PV")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *PersistentVolumeReconciler) processVolumeForReplicationGroup(ctx context.Context, volume *v1.PersistentVolume,
	replicationGroupName string, log logr.Logger,
	scParams map[string]string) error {

	log.V(common.InfoLevel).Info("Begin process volume for replication-group")

	log.V(common.DebugLevel).Info("Adding replication-group and remote-cluster annotation to the PV")

	if _, ok := volume.Annotations[controller.ReplicationGroup]; !ok {
		controller.AddAnnotation(volume, controller.ReplicationGroup, replicationGroupName)
	}

	if _, ok := volume.Annotations[controller.RemoteStorageClassAnnotation]; !ok {
		controller.AddAnnotation(volume, controller.RemoteStorageClassAnnotation, scParams[controller.StorageClassRemoteStorageClassParam])
	}

	if _, ok := volume.Labels[controller.ReplicationGroup]; !ok {
		controller.AddLabel(volume, controller.ReplicationGroup, replicationGroupName)
	}

	// Add retention policy annotation for syncing deletion across clusters.
	// Adds `retain` as default retention policy for PV, in case no or incorrect value
	// is specified by the user.

	log.V(common.DebugLevel).Info("Adding retention policy annotation for syncing deletion across clusters")

	if scParams[controller.RemotePVRetentionPolicy] == controller.RemoteRetentionValueDelete {
		controller.AddAnnotation(volume, controller.RemotePVRetentionPolicy, controller.RemoteRetentionValueDelete)
	} else {
		controller.AddAnnotation(volume, controller.RemotePVRetentionPolicy, controller.RemoteRetentionValueRetain)
	}

	log.V(common.DebugLevel).Info("Adding replication-group and remote-cluster label to the PV")

	if scParams[controller.StorageClassRemoteClusterParam] != "" {
		controller.AddAnnotation(volume, controller.RemoteClusterID, scParams[controller.StorageClassRemoteClusterParam])
		controller.AddLabel(volume, controller.DriverName, r.DriverName)
		controller.AddLabel(volume, controller.RemoteClusterID, scParams[controller.StorageClassRemoteClusterParam])
	}

	log.V(common.DebugLevel).Info("Adding driver specific labels")

	if r.ContextPrefix != "" {
		if volume.Spec.CSI != nil {
			for key, value := range volume.Spec.CSI.VolumeAttributes {
				if strings.HasPrefix(key, r.ContextPrefix) {
					labelKey := fmt.Sprintf("%s%s", r.Domain, strings.TrimPrefix(key, r.ContextPrefix))
					controller.AddLabel(volume, labelKey, value)
				}
			}
			controller.AddAnnotation(volume, controller.ContextPrefix, r.ContextPrefix)
		}
	}
	controller.AddAnnotation(volume, controller.PVProtectionComplete, "yes")

	if err := r.Update(ctx, volume); err != nil {
		log.Error(err, "Failed to add label and annotation to the PV")
		return err
	}

	log.V(common.InfoLevel).Info("Label and annotation added to the pv", "PVName", volume.Name)
	r.EventRecorder.Eventf(volume, v1.EventTypeNormal, "Updated", "DellCSIReplicationGroup[%s] annotation added to the pv", replicationGroupName)

	return nil
}

func (r *PersistentVolumeReconciler) createProtectionGroupAndRG(ctx context.Context, volumeHandle string, scParams map[string]string) (string, error) {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Creating protection-group anf RG")

	res, err := r.ReplicationClient.CreateStorageProtectionGroup(ctx, volumeHandle, scParams)
	if err != nil {
		log.Error(err, "Failed to create protection group", "volumeHandle", volumeHandle)
		return "", err
	}

	log.V(common.DebugLevel).Info("Checking if a DellCSIReplicationGroup instance already exists for the ProtectionGroup")

	rgList := new(storagev1alpha1.DellCSIReplicationGroupList)
	if err = r.List(ctx, rgList, client.MatchingFields{
		protectionIndexKey: res.GetLocalProtectionGroupId(),
	}); err != nil {
		log.Error(err, "Failed to check for a pre-existing replication-group for this pv")
		return "", err
	}

	var replicationGroup *storagev1alpha1.DellCSIReplicationGroup
	if len(rgList.Items) == 0 {
		// DellCSIReplicationGroup instance doesn't exists for the ProtectionGroup;
		// creating a new one
		var err error
		replicationGroup, err = r.createReplicationGroupOnce(ctx, res, scParams[controller.StorageClassRemoteClusterParam], scParams[controller.RemoteRGRetentionPolicy])
		if err != nil {
			return "", err
		}
	} else {
		// DellCSIReplicationGroup instance already exits; using the same one
		replicationGroup = &rgList.Items[0]
		log.V(common.InfoLevel).Info("DellCSIReplicationGroup instance already exists for the protection group of this PV", "DellCSIReplicationGroupName", replicationGroup.Name)
	}

	return replicationGroup.Name, nil
}

func (r *PersistentVolumeReconciler) createReplicationGroupOnce(ctx context.Context, res *replication.CreateStorageProtectionGroupResponse, remoteClusterID string, remoteRGRetentionPolicy string) (*storagev1alpha1.DellCSIReplicationGroup, error) {
	rgObj, err, _ := r.SingleFlightGroup.Do(res.GetLocalProtectionGroupId(), func() (interface{}, error) {
		return r.createReplicationGroup(ctx, res, remoteClusterID, remoteRGRetentionPolicy)
	})
	if err != nil {
		return nil, err
	}
	return rgObj.(*storagev1alpha1.DellCSIReplicationGroup), nil
}

func (r *PersistentVolumeReconciler) createReplicationGroup(ctx context.Context, res *replication.CreateStorageProtectionGroupResponse, remoteClusterID string, remoteRGRetentionPolicy string) (*storagev1alpha1.DellCSIReplicationGroup, error) {
	log := common.GetLoggerFromContext(ctx)
	log.V(common.InfoLevel).Info("Creating replication-group")

	annotations := make(map[string]string)
	labels := make(map[string]string)
	labels[controller.RemoteClusterID] = remoteClusterID
	labels[controller.DriverName] = r.DriverName
	// Add key-value pairs from protection-group-attributes, with
	// certain prefix, as labels to the DellCSIReplicationGroup
	if r.ContextPrefix != "" {
		for key, value := range res.GetLocalProtectionGroupAttributes() {
			if strings.HasPrefix(key, r.ContextPrefix) {
				labels[r.Domain+strings.TrimPrefix(key, r.ContextPrefix)] = value
			}
		}
		annotations[controller.ContextPrefix] = r.ContextPrefix
	}
	annotations[controller.RemoteClusterID] = remoteClusterID

	// Add retention policy annotation for syncing deletion across clusters.
	// Adds `retain` as default retention policy for RG, in case no or incorrect value
	// is specified by the user.

	log.V(common.DebugLevel).Info("Adding retention policy annotation")

	if remoteRGRetentionPolicy == controller.RemoteRetentionValueDelete {
		annotations[controller.RemoteRGRetentionPolicy] = controller.RemoteRetentionValueDelete
	} else {
		annotations[controller.RemoteRGRetentionPolicy] = controller.RemoteRetentionValueRetain
	}

	replicationGroup := &storagev1alpha1.DellCSIReplicationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "rg-" + uuid.New().String(),
			Finalizers:  []string{controller.ReplicationFinalizer},
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: storagev1alpha1.DellCSIReplicationGroupSpec{
			DriverName:                      r.DriverName,
			ProtectionGroupID:               res.GetLocalProtectionGroupId(),
			ProtectionGroupAttributes:       res.GetLocalProtectionGroupAttributes(),
			RemoteProtectionGroupID:         res.GetRemoteProtectionGroupId(),
			RemoteProtectionGroupAttributes: res.GetRemoteProtectionGroupAttributes(),
			RemoteClusterID:                 remoteClusterID,
		},
	}

	if err := r.Create(ctx, replicationGroup); err != nil {
		log.Error(err, "Failed to create the replication-group")
		return nil, err
	}
	log.V(common.InfoLevel).Info("DellCSIReplicationGroup instance created for the protection group of the PV", "DellCSIReplicationGroupName", replicationGroup.Name)
	return replicationGroup, nil
}

// SetupWithManager start using reconciler by creating new controller managed by provided manager
func (r *PersistentVolumeReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, limiter ratelimiter.RateLimiter, maxReconcilers int) error {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &storagev1alpha1.DellCSIReplicationGroup{}, protectionIndexKey, func(object client.Object) []string {
		replicationGroup := object.(*storagev1alpha1.DellCSIReplicationGroup)
		if replicationGroup.Spec.ProtectionGroupID != "" {
			return []string{replicationGroup.Spec.ProtectionGroupID}
		}
		return nil
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.PersistentVolume{}).
		WithOptions(reconcile.Options{
			MaxConcurrentReconciles: maxReconcilers,
			RateLimiter:             limiter,
		}).
		Complete(r)
}
