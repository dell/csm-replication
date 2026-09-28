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

// Package metrics provides naming constants for CSM Replication metrics.
package metrics

// Replication metric name constants following the dell_csm_repl_ prefix convention.
const (
	// MetricReplicationPairStatus reports the status of replication pairs.
	// 1 = active, 0 = inactive/unknown.
	MetricReplicationPairStatus = "dell_csm_repl_pair_status"

	// MetricReplicationLagSeconds reports the replication lag in seconds.
	MetricReplicationLagSeconds = "dell_csm_repl_lag_seconds"

	// MetricReplicationBandwidthBytes reports the current replication traffic
	// bandwidth in bytes per second from the storage array.
	MetricReplicationBandwidthBytes = "dell_csm_repl_bandwidth_bytes"

	// MetricReplicationSuccessTotal reports total successful replication operations.
	MetricReplicationSuccessTotal = "dell_csm_repl_success_total"

	// MetricReplicationFailureTotal reports total failed replication operations.
	MetricReplicationFailureTotal = "dell_csm_repl_failure_total"

	// MetricReplicationSuccessRatio reports the ratio of successful replication
	// operations (0.0 to 1.0).
	MetricReplicationSuccessRatio = "dell_csm_repl_success_ratio"

	// MetricReplicationControllerHealth reports the health of the replication controller.
	// 1 = healthy, 0 = unhealthy.
	MetricReplicationControllerHealth = "dell_csm_repl_controller_health"

	// MetricReplicationMetricsStale indicates whether metrics data is stale.
	// 1 = stale, 0 = fresh.
	MetricReplicationMetricsStale = "dell_csm_repl_metrics_stale"

	// MetricReplicationLastCollectionTimestamp reports the last complete collection cycle.
	MetricReplicationLastCollectionTimestamp = "dell_csm_repl_last_collection_timestamp_seconds"
)

// Label constants used across replication metrics.
const (
	LabelDriver     = "driver"
	LabelPolicyName = "policy_name"
	LabelStatus     = "status"
	LabelAction     = "action"
)

// PowerMax-specific SRDF metric name constants.
// Exposed on the same replication metrics endpoint (port 8445) with explicit
// rdf_group and mode labels to satisfy the per-RDF-group requirement.
const (
	// MetricSRDFGroupState reports the current SRDF replication state per RDF group.
	// 1=SYNCHRONIZED, 2=SYNC_IN_PROGRESS, 3=SUSPENDED, 4=FAILEDOVER, 0=UNKNOWN/EMPTY.
	MetricSRDFGroupState = "dell_powermax_srdf_group_state"

	// MetricSRDFLagSeconds reports the SRDF replication lag in seconds between R1 and R2.
	MetricSRDFLagSeconds = "dell_powermax_srdf_lag_seconds"

	// MetricSRDFBandwidthBytes reports the SRDF replication bandwidth in bytes per second.
	MetricSRDFBandwidthBytes = "dell_powermax_srdf_bandwidth_bytes_per_sec"

	// MetricSRDFLinkStatus reports the active/inactive status of the RDF group link.
	// 1=active, 0=inactive.
	MetricSRDFLinkStatus = "dell_powermax_srdf_link_status"
)

// SRDF-specific label constants.
const (
	LabelRGName   = "rg_name"
	LabelRDFGroup = "rdf_group"
	LabelMode     = "mode"
)
