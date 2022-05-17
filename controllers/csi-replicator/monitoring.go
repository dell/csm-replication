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
	"fmt"
	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"
	"github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"
)

// ReplicationGroupMonitoring structure for monitoring current status of replication groups
type ReplicationGroupMonitoring struct {
	Lock sync.Mutex
	client.Client
	EventRecorder      record.EventRecorder
	Log                logr.Logger
	DriverName         string
	ReplicationClient  csireplication.Replication
	MonitoringInterval time.Duration
}

// Monitor polls RGs over a defined interval and
// updates the ReplicationLinkStatus depending on the response received
// from the driver.
func (r *ReplicationGroupMonitoring) Monitor(ctx context.Context) error {
	ticker := time.NewTicker(r.MonitoringInterval).C
	go func(<-chan time.Time) {
		for {
			r.monitorReplicationGroups(ticker)
		}
	}(ticker)

	return nil
}

func (r *ReplicationGroupMonitoring) monitorReplicationGroups(ticker <-chan time.Time) {

	r.Log.V(common.InfoLevel).Info("Start monitoring replication-group")

	dellCSIReplicationGroupsList := new(storagev1alpha1.DellCSIReplicationGroupList)
	select {
	case <-ticker:
		ctx, cancel := context.WithTimeout(context.Background(), r.MonitoringInterval)
		defer cancel()
		err := r.List(ctx, dellCSIReplicationGroupsList)
		if err != nil {
			r.Log.Error(err, "Error encountered while listing Dell CSI ReplicationGroups")
			return
		}
		for _, rg := range dellCSIReplicationGroupsList.Items {
			rg := rg
			if rg.Spec.DriverName != r.DriverName {
				// silently ignore the RGs not owned by this sidecar
				continue
			}
			r.Log.V(common.DebugLevel).Info("Processing RG " + rg.Name + "for monitoring")
			// Check if there are any PVs in the cluster with the RG label
			var persistentVolumes v1.PersistentVolumeList
			matchingLabels := make(map[string]string)
			matchingLabels[controllers.DriverName] = r.DriverName
			matchingLabels[controllers.ReplicationGroup] = rg.Name
			err := r.List(ctx, &persistentVolumes, client.MatchingLabels(matchingLabels))
			if err != nil {
				// Log the error and continue
				r.Log.Error(err, "failed to fetch associated PVs with this RG")
			} else {
				if len(persistentVolumes.Items) == 0 {
					r.Log.V(common.DebugLevel).Info(
						"Skipping RG " + rg.Name + "as there are no associated PVs")
					// Update status to EMPTY
					err := updateRGLinkStatus(ctx, r.Client, &rg, replication.StorageProtectionGroupStatus_EMPTY.String(), rg.Status.ReplicationLinkState.IsSource, "")
					if err != nil {
						r.Log.Error(err, "Failed to update the RG status")
					}
					continue
				}
			}

			// Fetch the RG details once more as we may have a stale copy
			var refreshedRG storagev1alpha1.DellCSIReplicationGroup
			err = r.Get(ctx, types.NamespacedName{Name: rg.Name}, &refreshedRG)
			if err != nil {
				r.Log.Error(err, "Error encountered while getting RG details")
				r.EventRecorder.Eventf(&rg, v1.EventTypeWarning, "Error", "Failed to get RG details")
				continue
			}

			updateRequired := r.isUpdateRequired(refreshedRG)
			if updateRequired {
				r.Lock.Lock()
				res, err := r.ReplicationClient.GetStorageProtectionGroupStatus(ctx, refreshedRG.Spec.ProtectionGroupID, refreshedRG.Spec.ProtectionGroupAttributes)
				r.Lock.Unlock()
				if err != nil {
					r.Log.Error(err, "Error encountered while getting protection group status")
					err = updateRGLinkStatus(ctx, r.Client, &refreshedRG,
						replication.StorageProtectionGroupStatus_UNKNOWN.String(), refreshedRG.Status.ReplicationLinkState.IsSource,
						err.Error())
					if err != nil {
						r.Log.Error(err, "Failed to update the RG Status")
						continue
					}
					continue
				}
				newStatus := res.GetStatus().State.String()
				// Update the LinkStatus only if it is required
				err = updateRGLinkStatus(ctx, r.Client, &refreshedRG, newStatus, res.GetStatus().IsSource, "")
				if err != nil {
					r.Log.Error(err, "Failed to update the RG status")
					continue
				}
			}
		}
	}
}

func updateRGLinkState(rg *storagev1alpha1.DellCSIReplicationGroup, status string, isSource bool, errorMsg string) {
	lastSuccessfulUpdate := new(metav1.Time)
	if errorMsg != "" {
		if rg.Status.ReplicationLinkState.LastSuccessfulUpdate != nil {
			lastSuccessfulUpdate = rg.Status.ReplicationLinkState.LastSuccessfulUpdate
		}
	} else {
		lastSuccessfulUpdate.Time = time.Now()
	}

	if rg.Status.ReplicationLinkState.IsSource != isSource {
		condition := storagev1alpha1.LastAction{
			Condition: fmt.Sprintf("Replication Link State:IsSource changed from (%v) to (%v)", rg.Status.ReplicationLinkState.IsSource, isSource),
			Time:      &metav1.Time{Time: time.Now()},
		}
		controllers.UpdateConditions(rg, condition, MaxNumberOfConditions)
	}

	rg.Status.ReplicationLinkState = storagev1alpha1.ReplicationLinkState{
		State:                status,
		LastSuccessfulUpdate: lastSuccessfulUpdate,
		ErrorMessage:         errorMsg,
		IsSource:             isSource,
	}
}

func updateRGLinkStatus(ctx context.Context, client client.Client, rg *storagev1alpha1.DellCSIReplicationGroup, status string,
	isSource bool, errMsg string) error {
	log := common.GetLoggerFromContext(ctx)
	updateRGLinkState(rg, status, isSource, errMsg)
	if err := client.Status().Update(ctx, rg); err != nil {
		log.Error(err, "Failed to update the state")
		return err
	}
	return nil
}

func (r *ReplicationGroupMonitoring) isUpdateRequired(rg storagev1alpha1.DellCSIReplicationGroup) bool {
	currTime := time.Now()
	if rg.Status.ReplicationLinkState.LastSuccessfulUpdate.IsZero() {
		return true
	}
	lastTime := rg.Status.ReplicationLinkState.LastSuccessfulUpdate.Time
	// Need to wait for at least MonitoringInterval before updating the RG again
	if currTime.Sub(lastTime) < r.MonitoringInterval {
		return false
	}
	return true
}
