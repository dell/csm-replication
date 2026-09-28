/*
 Copyright © 2021-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"fmt"
	"sync"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/internal/metrics"
	csireplication "github.com/dell/csm-replication/pkg/csi-clients/replication"
	"github.com/dell/csmlog"
	"github.com/dell/dell-csi-extensions/replication"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReplicationGroupMonitoring structure for monitoring current status of replication groups
type ReplicationGroupMonitoring struct {
	Lock sync.Mutex
	client.Client
	EventRecorder      record.EventRecorder
	DriverName         string
	ReplicationClient  csireplication.Replication
	MonitoringInterval time.Duration
}

// Monitor polls RGs over a defined interval and
// updates the ReplicationLinkStatus depending on the response received
// from the driver.
func (r *ReplicationGroupMonitoring) Monitor(ctx context.Context) error {
	go func() {
		ticker := time.NewTicker(r.MonitoringInterval).C
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker:
				r.monitorReplicationGroups()
			}
		}
	}()

	return nil
}

func (r *ReplicationGroupMonitoring) monitorReplicationGroups() {
	csmlog.WithFields(csmlog.Fields{
		"controller": "replicationgroup-monitoring",
		"driverName": r.DriverName,
	}).Info("Start monitoring replication-group")

	dellCSIReplicationGroupsList := new(repv1.DellCSIReplicationGroupList)
	ctx, cancel := context.WithTimeout(context.Background(), r.MonitoringInterval)
	defer cancel()
	collectionComplete := true
	err := r.List(ctx, dellCSIReplicationGroupsList)
	if err != nil {
		csmlog.Errorf("Error encountered while listing Dell CSI ReplicationGroups: %v", err)
		if replMetrics := metrics.GetGlobalReplicationMetrics(); replMetrics != nil {
			replMetrics.SetMetricsStale(r.DriverName, true)
		}
		return
	}
	for _, rg := range dellCSIReplicationGroupsList.Items {
		rg := rg
		if rg.Spec.DriverName != r.DriverName {
			// silently ignore the RGs not owned by this sidecar
			continue
		}
		csmlog.WithFields(csmlog.Fields{
			"controller": "replicationgroup-monitoring",
			"driverName": r.DriverName,
			"rgName":     rg.Name,
			"pgID":       rg.Spec.ProtectionGroupID,
		}).Info("Processing RG for monitoring")
		// Check if there are any PVs in the cluster with the RG label
		var persistentVolumes v1.PersistentVolumeList
		matchingLabels := make(map[string]string)
		matchingLabels[controllers.DriverName] = r.DriverName
		matchingLabels[controllers.ReplicationGroup] = rg.Name
		err := r.List(ctx, &persistentVolumes, client.MatchingLabels(matchingLabels))
		if err != nil {
			collectionComplete = false
			// Log the error and continue
			csmlog.WithFields(csmlog.Fields{
				"controller": "replicationgroup-monitoring",
				"driverName": r.DriverName,
				"rgName":     rg.Name,
				"pgID":       rg.Spec.ProtectionGroupID,
			}).Errorf("failed to fetch associated PVs with this RG: %v", err)
		} else {
			if len(persistentVolumes.Items) == 0 {
				csmlog.WithFields(csmlog.Fields{
					"controller": "replicationgroup-monitoring",
					"driverName": r.DriverName,
					"rgName":     rg.Name,
					"pgID":       rg.Spec.ProtectionGroupID,
				}).Info("Skipping RG as there are no associated PVs")
				// Update status to EMPTY
				err := updateRGLinkStatus(ctx, r.Client, &rg, replication.StorageProtectionGroupStatus_EMPTY.String(), rg.Status.ReplicationLinkState.IsSource, "")
				if err != nil {
					collectionComplete = false
					csmlog.Errorf("Failed to update the RG status: %v", err)
				}
				continue
			}
		}

		// Fetch the RG details once more as we may have a stale copy
		var refreshedRG repv1.DellCSIReplicationGroup
		err = r.Get(ctx, types.NamespacedName{Name: rg.Name}, &refreshedRG)
		if err != nil {
			collectionComplete = false
			csmlog.Errorf("Error encountered while getting RG details: %v", err)
			r.EventRecorder.Eventf(&rg, v1.EventTypeWarning, "Error", "Failed to get RG details")
			continue
		}

		updateRequired := r.isUpdateRequired(refreshedRG)
		if updateRequired {
			r.Lock.Lock()
			res, err := r.ReplicationClient.GetStorageProtectionGroupStatus(ctx, refreshedRG.Spec.ProtectionGroupID, refreshedRG.Spec.ProtectionGroupAttributes)
			r.Lock.Unlock()
			if err != nil {
				collectionComplete = false
				csmlog.WithFields(csmlog.Fields{
					"pgID":       refreshedRG.Spec.ProtectionGroupID,
					"rgName":     refreshedRG.Name,
					"driverName": r.DriverName,
				}).Errorf("Error encountered while getting protection group status: %v", err)
				// Update controller health and mark metrics as stale on CSI call failure
				if replMetrics := metrics.GetGlobalReplicationMetrics(); replMetrics != nil {
					replMetrics.SetControllerHealth(r.DriverName, false)
					replMetrics.SetMetricsStale(r.DriverName, true)
				}
				err = updateRGLinkStatus(ctx, r.Client, &refreshedRG,
					replication.StorageProtectionGroupStatus_UNKNOWN.String(), refreshedRG.Status.ReplicationLinkState.IsSource,
					err.Error())
				if err != nil {
					csmlog.Errorf("Failed to update the RG Status: %v", err)
					continue
				}
				continue
			}
			newStatus := res.GetStatus().State.String()
			csmlog.WithFields(csmlog.Fields{
				"pgID":       refreshedRG.Spec.ProtectionGroupID,
				"rgName":     refreshedRG.Name,
				"driverName": r.DriverName,
				"status":     newStatus,
			}).Info("Sync cycle completed for replication group")
			// Update the LinkStatus only if it is required
			err = updateRGLinkStatus(ctx, r.Client, &refreshedRG, newStatus, res.GetStatus().IsSource, "")
			if err != nil {
				collectionComplete = false
				csmlog.Errorf("Failed to update the RG status: %v", err)
				continue
			}

			// Record metrics for replication status
			if replMetrics := metrics.GetGlobalReplicationMetrics(); replMetrics != nil {
				replMetrics.SetControllerHealth(r.DriverName, true)

				policyName := refreshedRG.Spec.ProtectionGroupID

				active := newStatus == replication.StorageProtectionGroupStatus_SYNCHRONIZED.String() ||
					newStatus == replication.StorageProtectionGroupStatus_SYNC_IN_PROGRESS.String()
				replMetrics.SetPairStatus(r.DriverName, policyName, newStatus, active)

				// Use LastSyncTimestamp as the sentinel for "driver provided lag data".
				// When the driver populates LastSyncTimestamp, use LagSeconds even if
				// it is 0 (perfectly synchronized). Only fall back to the K8s
				// LastSuccessfulUpdate timestamp when the driver had no sync data.
				if res.GetStatus().LastSyncTimestamp > 0 {
					replMetrics.SetLagSeconds(r.DriverName, policyName, float64(res.GetStatus().LagSeconds))
				} else if refreshedRG.Status.ReplicationLinkState.LastSuccessfulUpdate != nil {
					lag := time.Since(refreshedRG.Status.ReplicationLinkState.LastSuccessfulUpdate.Time).Seconds()
					replMetrics.SetLagSeconds(r.DriverName, policyName, lag)
				} else {
					replMetrics.SetLagSeconds(r.DriverName, policyName, 0)
				}

				if res.GetStatus().BandwidthBytesPerSec > 0 {
					replMetrics.SetBandwidthBytes(r.DriverName, policyName, float64(res.GetStatus().BandwidthBytesPerSec))
				} else {
					replMetrics.SetBandwidthBytes(r.DriverName, policyName, 0)
				}
			}

			// Record PowerMax-specific SRDF metrics with explicit rdf_group and mode labels.
			// ParseSRDFGroupInfo returns non-empty values only for PowerMax SRDF
			// protection group IDs ending with ASYNC or SYNC.
			if srdfMetrics := metrics.GetGlobalSRDFMetrics(); srdfMetrics != nil {
				if rdfGroup, mode := metrics.ParseSRDFGroupInfo(refreshedRG.Spec.ProtectionGroupID); rdfGroup != "" {
					srdfMetrics.SetGroupState(r.DriverName, refreshedRG.Name, rdfGroup, mode, newStatus)

					if res.GetStatus().LastSyncTimestamp > 0 {
						srdfMetrics.SetLagSeconds(r.DriverName, refreshedRG.Name, rdfGroup, mode, float64(res.GetStatus().LagSeconds))
					} else if refreshedRG.Status.ReplicationLinkState.LastSuccessfulUpdate != nil {
						lag := time.Since(refreshedRG.Status.ReplicationLinkState.LastSuccessfulUpdate.Time).Seconds()
						srdfMetrics.SetLagSeconds(r.DriverName, refreshedRG.Name, rdfGroup, mode, lag)
					} else {
						srdfMetrics.SetLagSeconds(r.DriverName, refreshedRG.Name, rdfGroup, mode, 0)
					}

					srdfMetrics.SetBandwidth(r.DriverName, refreshedRG.Name, rdfGroup, mode, float64(res.GetStatus().BandwidthBytesPerSec))
				}
			}

		}
	}
	if replMetrics := metrics.GetGlobalReplicationMetrics(); replMetrics != nil {
		replMetrics.SetMetricsStale(r.DriverName, !collectionComplete)
		if collectionComplete {
			replMetrics.SetLastCollectionTimestamp(r.DriverName, float64(time.Now().Unix()))
		}
	}
}

func updateRGLinkState(rg *repv1.DellCSIReplicationGroup, status string, isSource bool, errorMsg string) {
	lastSuccessfulUpdate := new(metav1.Time)
	if errorMsg != "" {
		if rg.Status.ReplicationLinkState.LastSuccessfulUpdate != nil {
			lastSuccessfulUpdate = rg.Status.ReplicationLinkState.LastSuccessfulUpdate
		}
	} else {
		lastSuccessfulUpdate.Time = time.Now()
	}

	previousState := rg.Status.ReplicationLinkState.State
	if rg.Status.ReplicationLinkState.IsSource != isSource {
		condition := repv1.LastAction{
			Condition: fmt.Sprintf("Replication Link State:IsSource changed from (%v) to (%v)", rg.Status.ReplicationLinkState.IsSource, isSource),
			Time:      &metav1.Time{Time: time.Now()},
		}
		controllers.UpdateConditions(rg, condition, MaxNumberOfConditions)
	}

	// Log replication link state transition if state changed
	if previousState != "" && previousState != status {
		csmlog.WithFields(csmlog.Fields{
			"rgName":     rg.Name,
			"pgID":       rg.Spec.ProtectionGroupID,
			"status":     status,
			"previous":   previousState,
			"driverName": rg.Spec.DriverName,
		}).Infof("Replication link state transition: %s -> %s", previousState, status)
	}

	rg.Status.ReplicationLinkState = repv1.ReplicationLinkState{
		State:                status,
		LastSuccessfulUpdate: lastSuccessfulUpdate,
		ErrorMessage:         errorMsg,
		IsSource:             isSource,
	}
}

func updateRGLinkStatus(ctx context.Context, client client.Client, rg *repv1.DellCSIReplicationGroup, status string,
	isSource bool, errMsg string,
) error {
	updateRGLinkState(rg, status, isSource, errMsg)
	if err := client.Status().Update(ctx, rg); err != nil {
		csmlog.Errorf("Failed to update the state: %v", err)
		return err
	}
	return nil
}

func (r *ReplicationGroupMonitoring) isUpdateRequired(rg repv1.DellCSIReplicationGroup) bool {
	currTime := time.Now()
	lastTime := rg.Status.ReplicationLinkState.LastSuccessfulUpdate
	if lastTime.IsZero() {
		return true
	}

	// Skip update if any action is being executed
	if rg.Spec.Action != "" {
		return false
	}

	// Need to wait for at least MonitoringInterval before updating the RG again
	return currTime.Sub(lastTime.Time) >= r.MonitoringInterval
}
