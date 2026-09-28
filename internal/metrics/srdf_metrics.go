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
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// SRDFMetrics holds Prometheus metric vectors for PowerMax SRDF replication.
// These provide per-RDF-group granularity with explicit rdf_group and mode labels,
// covering the PowerMax-specific metrics section of the observability spec.
type SRDFMetrics struct {
	groupState *prometheus.GaugeVec
	lagSeconds *prometheus.GaugeVec
	bandwidth  *prometheus.GaugeVec
	linkStatus *prometheus.GaugeVec
}

// NewSRDFMetrics creates and registers all SRDF metrics with the given registry.
func NewSRDFMetrics(registry *prometheus.Registry) *SRDFMetrics {
	labels := []string{LabelDriver, LabelRGName, LabelRDFGroup, LabelMode}

	m := &SRDFMetrics{
		groupState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricSRDFGroupState,
			Help: "Current SRDF replication state per RDF group. 1=SYNCHRONIZED, 2=SYNC_IN_PROGRESS, 3=SUSPENDED, 4=FAILEDOVER, 0=UNKNOWN/EMPTY.",
		}, labels),

		lagSeconds: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricSRDFLagSeconds,
			Help: "SRDF replication lag in seconds between R1 and R2 per RDF group.",
		}, labels),

		bandwidth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricSRDFBandwidthBytes,
			Help: "SRDF replication bandwidth in bytes per second per RDF group.",
		}, labels),

		linkStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: MetricSRDFLinkStatus,
			Help: "Active/inactive status of the SRDF RDF group link. 1=active (SYNCHRONIZED or SYNC_IN_PROGRESS), 0=inactive.",
		}, labels),
	}

	registry.MustRegister(
		m.groupState,
		m.lagSeconds,
		m.bandwidth,
		m.linkStatus,
	)

	return m
}

// srdfStateToFloat maps a replication state string to a numeric gauge value.
func srdfStateToFloat(state string) float64 {
	switch strings.ToUpper(state) {
	case "SYNCHRONIZED":
		return 1
	case "SYNC_IN_PROGRESS":
		return 2
	case "SUSPENDED":
		return 3
	case "FAILEDOVER":
		return 4
	default:
		return 0
	}
}

// ParseSRDFGroupInfo extracts the RDF group number and replication mode from a
// PowerMax SRDF protection group ID (format: csi-rep-sg-{namespace}-{rdfGroup}-{mode}).
// Returns empty strings for non-PowerMax SRDF protection group IDs.
func ParseSRDFGroupInfo(protectionGroupID string) (rdfGroup, mode string) {
	parts := strings.Split(protectionGroupID, "-")
	if len(parts) < 5 {
		return "", ""
	}
	lastMode := parts[len(parts)-1]
	if lastMode != "ASYNC" && lastMode != "SYNC" {
		return "", ""
	}
	return parts[len(parts)-2], lastMode
}

// SetGroupState sets the SRDF group state metric for the given replication group.
func (m *SRDFMetrics) SetGroupState(driver, rgName, rdfGroup, mode, state string) {
	val := srdfStateToFloat(state)
	m.groupState.WithLabelValues(driver, rgName, rdfGroup, mode).Set(val)
	active := float64(0)
	if val == 1 || val == 2 {
		active = 1
	}
	m.linkStatus.WithLabelValues(driver, rgName, rdfGroup, mode).Set(active)
}

// SetLagSeconds sets the SRDF replication lag metric for the given replication group.
func (m *SRDFMetrics) SetLagSeconds(driver, rgName, rdfGroup, mode string, lag float64) {
	m.lagSeconds.WithLabelValues(driver, rgName, rdfGroup, mode).Set(lag)
}

// SetBandwidth sets the SRDF replication bandwidth metric for the given replication group.
func (m *SRDFMetrics) SetBandwidth(driver, rgName, rdfGroup, mode string, bw float64) {
	m.bandwidth.WithLabelValues(driver, rgName, rdfGroup, mode).Set(bw)
}

// DeleteGroup removes all SRDF metric label sets for a replication group,
// preventing stale metrics from persisting after an RG is removed.
func (m *SRDFMetrics) DeleteGroup(driver, rgName, rdfGroup, mode string) {
	m.groupState.DeleteLabelValues(driver, rgName, rdfGroup, mode)
	m.lagSeconds.DeleteLabelValues(driver, rgName, rdfGroup, mode)
	m.bandwidth.DeleteLabelValues(driver, rgName, rdfGroup, mode)
	m.linkStatus.DeleteLabelValues(driver, rgName, rdfGroup, mode)
}
