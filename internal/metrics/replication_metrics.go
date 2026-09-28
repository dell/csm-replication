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

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// ReplicationMetrics holds Prometheus metric vectors for CSM Replication.
type ReplicationMetrics struct {
	pairStatus              *prometheus.GaugeVec
	lagSeconds              *prometheus.GaugeVec
	bandwidthBytes          *prometheus.GaugeVec
	successTotal            *prometheus.CounterVec
	failureTotal            *prometheus.CounterVec
	successRatio            *prometheus.GaugeVec
	controllerHealth        *prometheus.GaugeVec
	metricsStale            *prometheus.GaugeVec
	lastCollectionTimestamp *prometheus.GaugeVec

	// Track success/failure counts for ratio calculation
	successCount map[string]float64
	failureCount map[string]float64
	countMu      sync.Mutex

	// pairStatusPrev tracks the last-reported status label per (driver+policyName) so
	// SetPairStatus can delete the stale series when the status transitions.
	// Without this, the old {status="SYNCHRONIZED"} gauge (value=1) would persist in
	// the Prometheus registry for up to 5 minutes, causing max-by expressions to
	// always return 1 even after the pair transitions to SUSPENDED or FAILED.
	pairStatusPrev   map[string]string
	pairStatusPrevMu sync.Mutex
}

// NewReplicationMetrics creates and registers all replication metrics with the given registry.
func NewReplicationMetrics(registry *prometheus.Registry) *ReplicationMetrics {
	m := &ReplicationMetrics{
		pairStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricReplicationPairStatus,
			Help: "Status of replication pairs. 1 = active, 0 = inactive/unknown.",
		}, []string{LabelDriver, LabelPolicyName, LabelStatus}),

		lagSeconds: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricReplicationLagSeconds,
			Help: "Replication lag in seconds between source and target.",
		}, []string{LabelDriver, LabelPolicyName}),

		bandwidthBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricReplicationBandwidthBytes,
			Help: "Current replication traffic bandwidth in bytes per second.",
		}, []string{LabelDriver, LabelPolicyName}),

		successTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: MetricReplicationSuccessTotal,
			Help: "Total successful replication operations.",
		}, []string{LabelDriver, LabelAction}),

		failureTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: MetricReplicationFailureTotal,
			Help: "Total failed replication operations.",
		}, []string{LabelDriver, LabelAction}),

		successRatio: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricReplicationSuccessRatio,
			Help: "Ratio of successful replication operations (0.0 to 1.0).",
		}, []string{LabelDriver}),

		controllerHealth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricReplicationControllerHealth,
			Help: "Health of the replication controller. 1 = healthy, 0 = unhealthy.",
		}, []string{LabelDriver}),

		metricsStale: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricReplicationMetricsStale,
			Help: "1 when replication metrics are stale (monitoring loop failing), 0 when fresh.",
		}, []string{LabelDriver}),

		lastCollectionTimestamp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricReplicationLastCollectionTimestamp,
			Help: "Unix timestamp of the last complete replication metrics collection cycle.",
		}, []string{LabelDriver}),

		successCount:   make(map[string]float64),
		failureCount:   make(map[string]float64),
		pairStatusPrev: make(map[string]string),
	}

	registry.MustRegister(
		m.pairStatus,
		m.lagSeconds,
		m.bandwidthBytes,
		m.successTotal,
		m.failureTotal,
		m.successRatio,
		m.controllerHealth,
		m.metricsStale,
		m.lastCollectionTimestamp,
	)

	return m
}

// SetPairStatus sets the status gauge for a replication pair.
//
// When the status label transitions (e.g. SYNCHRONIZED → SUSPENDED), the previous
// label combination is deleted from the GaugeVec so that stale series do not
// interfere with max-by / min-by Prometheus alert expressions. Without deletion,
// the old {status="SYNCHRONIZED", value=1} series would linger for up to 5 minutes,
// causing max by (driver, policy_name)(dell_csm_repl_pair_status) to always return 1
// even after the pair becomes inactive.
func (m *ReplicationMetrics) SetPairStatus(driver, policyName, status string, active bool) {
	key := driver + "/" + policyName

	m.pairStatusPrevMu.Lock()
	prev := m.pairStatusPrev[key]
	if prev != "" && prev != status {
		m.pairStatus.DeleteLabelValues(driver, policyName, prev)
	}
	m.pairStatusPrev[key] = status
	m.pairStatusPrevMu.Unlock()

	val := float64(0)
	if active {
		val = 1
	}
	m.pairStatus.WithLabelValues(driver, policyName, status).Set(val)
}

// SetLagSeconds sets the replication lag in seconds.
func (m *ReplicationMetrics) SetLagSeconds(driver, policyName string, lag float64) {
	m.lagSeconds.WithLabelValues(driver, policyName).Set(lag)
}

// SetBandwidthBytes sets the replication bandwidth in bytes per second.
func (m *ReplicationMetrics) SetBandwidthBytes(driver, policyName string, bw float64) {
	m.bandwidthBytes.WithLabelValues(driver, policyName).Set(bw)
}

// RecordAction records a replication action result and updates the success ratio.
func (m *ReplicationMetrics) RecordAction(driver, action, status string) {
	m.countMu.Lock()
	defer m.countMu.Unlock()

	if status == "success" {
		m.successTotal.WithLabelValues(driver, action).Inc()
		m.successCount[driver]++
	} else {
		m.failureTotal.WithLabelValues(driver, action).Inc()
		m.failureCount[driver]++
	}

	total := m.successCount[driver] + m.failureCount[driver]
	if total > 0 {
		m.successRatio.WithLabelValues(driver).Set(m.successCount[driver] / total)
	}
}

// SetControllerHealth sets the controller health gauge.
func (m *ReplicationMetrics) SetControllerHealth(driver string, healthy bool) {
	val := float64(0)
	if healthy {
		val = 1
	}
	m.controllerHealth.WithLabelValues(driver).Set(val)
}

// SetMetricsStale marks the per-driver metrics as stale (1) or fresh (0).
// Call with stale=true when the monitoring loop fails to update metrics,
// and stale=false when metrics are successfully refreshed.
func (m *ReplicationMetrics) SetMetricsStale(driver string, stale bool) {
	val := float64(0)
	if stale {
		val = 1
	}
	m.metricsStale.WithLabelValues(driver).Set(val)
}

// SetLastCollectionTimestamp records when a complete monitoring cycle finished.
func (m *ReplicationMetrics) SetLastCollectionTimestamp(driver string, timestamp float64) {
	m.lastCollectionTimestamp.WithLabelValues(driver).Set(timestamp)
}
