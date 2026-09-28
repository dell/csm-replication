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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReplicationMetrics(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	require.NotNil(t, m, "NewReplicationMetrics should return a non-nil metrics instance")
	assert.NotNil(t, m.pairStatus, "pairStatus gauge should be initialized")
	assert.NotNil(t, m.lagSeconds, "lagSeconds gauge should be initialized")
	assert.NotNil(t, m.bandwidthBytes, "bandwidthBytes gauge should be initialized")
	assert.NotNil(t, m.successTotal, "successTotal counter should be initialized")
	assert.NotNil(t, m.failureTotal, "failureTotal counter should be initialized")
	assert.NotNil(t, m.successRatio, "successRatio gauge should be initialized")
	assert.NotNil(t, m.controllerHealth, "controllerHealth gauge should be initialized")
	assert.NotNil(t, m.metricsStale, "metricsStale gauge should be initialized")
	assert.NotNil(t, m.lastCollectionTimestamp, "lastCollectionTimestamp gauge should be initialized")
	assert.NotNil(t, m.successCount, "successCount map should be initialized")
	assert.NotNil(t, m.failureCount, "failureCount map should be initialized")
}

func TestSetPairStatusActive(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetPairStatus("csi-powerstore", "pg-1", "SYNCHRONIZED", true)

	expected := `
		# HELP dell_csm_repl_pair_status Status of replication pairs. 1 = active, 0 = inactive/unknown.
		# TYPE dell_csm_repl_pair_status gauge
		dell_csm_repl_pair_status{driver="csi-powerstore",policy_name="pg-1",status="SYNCHRONIZED"} 1
	`
	err := testutil.CollectAndCompare(m.pairStatus, strings.NewReader(expected))
	assert.NoError(t, err)
}

func TestSetPairStatusInactive(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetPairStatus("csi-powerstore", "pg-1", "UNKNOWN", false)

	expected := `
		# HELP dell_csm_repl_pair_status Status of replication pairs. 1 = active, 0 = inactive/unknown.
		# TYPE dell_csm_repl_pair_status gauge
		dell_csm_repl_pair_status{driver="csi-powerstore",policy_name="pg-1",status="UNKNOWN"} 0
	`
	err := testutil.CollectAndCompare(m.pairStatus, strings.NewReader(expected))
	assert.NoError(t, err)
}

// TestSetPairStatusTransitionDeletesStale verifies that when the status label
// transitions (e.g. SYNCHRONIZED → SUSPENDED), the OLD {status="SYNCHRONIZED"}
// series is deleted from the GaugeVec so it does not linger as a stale value=1
// that would block max-by alert expressions from firing.
func TestSetPairStatusTransitionDeletesStale(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	// First poll: pair is SYNCHRONIZED (active=true → value=1)
	m.SetPairStatus("csi-powerstore", "pg-1", "SYNCHRONIZED", true)

	after1st := `
		# HELP dell_csm_repl_pair_status Status of replication pairs. 1 = active, 0 = inactive/unknown.
		# TYPE dell_csm_repl_pair_status gauge
		dell_csm_repl_pair_status{driver="csi-powerstore",policy_name="pg-1",status="SYNCHRONIZED"} 1
	`
	require.NoError(t, testutil.CollectAndCompare(m.pairStatus, strings.NewReader(after1st)))

	// Second poll: pair transitions to SUSPENDED (active=false → value=0).
	// The old SYNCHRONIZED series must be deleted; only SUSPENDED should remain.
	m.SetPairStatus("csi-powerstore", "pg-1", "SUSPENDED", false)

	after2nd := `
		# HELP dell_csm_repl_pair_status Status of replication pairs. 1 = active, 0 = inactive/unknown.
		# TYPE dell_csm_repl_pair_status gauge
		dell_csm_repl_pair_status{driver="csi-powerstore",policy_name="pg-1",status="SUSPENDED"} 0
	`
	// Critically: the SYNCHRONIZED=1 series must NOT be present; if it were,
	// max by (driver, policy_name) would return 1 and REP-03 would not fire.
	assert.NoError(t, testutil.CollectAndCompare(m.pairStatus, strings.NewReader(after2nd)),
		"stale SYNCHRONIZED series must be deleted on status transition")
}

// TestSetPairStatusSameStatusNoDelete verifies that calling SetPairStatus with
// the same status label twice does not delete the series (regression guard).
func TestSetPairStatusSameStatusNoDelete(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetPairStatus("csi-powerstore", "pg-1", "SYNCHRONIZED", true)
	m.SetPairStatus("csi-powerstore", "pg-1", "SYNCHRONIZED", true) // same status, no-op delete

	expected := `
		# HELP dell_csm_repl_pair_status Status of replication pairs. 1 = active, 0 = inactive/unknown.
		# TYPE dell_csm_repl_pair_status gauge
		dell_csm_repl_pair_status{driver="csi-powerstore",policy_name="pg-1",status="SYNCHRONIZED"} 1
	`
	assert.NoError(t, testutil.CollectAndCompare(m.pairStatus, strings.NewReader(expected)))
}

// TestSetPairStatusMultiplePolicies verifies that status tracking is isolated per
// (driver, policyName) — a transition on one policy must not affect another.
func TestSetPairStatusMultiplePolicies(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetPairStatus("csi-powerstore", "pg-1", "SYNCHRONIZED", true)
	m.SetPairStatus("csi-powerstore", "pg-2", "SYNCHRONIZED", true)

	// Transition pg-1 to SUSPENDED; pg-2 must remain SYNCHRONIZED=1
	m.SetPairStatus("csi-powerstore", "pg-1", "SUSPENDED", false)

	expected := `
		# HELP dell_csm_repl_pair_status Status of replication pairs. 1 = active, 0 = inactive/unknown.
		# TYPE dell_csm_repl_pair_status gauge
		dell_csm_repl_pair_status{driver="csi-powerstore",policy_name="pg-1",status="SUSPENDED"} 0
		dell_csm_repl_pair_status{driver="csi-powerstore",policy_name="pg-2",status="SYNCHRONIZED"} 1
	`
	assert.NoError(t, testutil.CollectAndCompare(m.pairStatus, strings.NewReader(expected)))
}

func TestSetLagSeconds(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetLagSeconds("csi-powerstore", "pg-1", 42.5)

	expected := `
		# HELP dell_csm_repl_lag_seconds Replication lag in seconds between source and target.
		# TYPE dell_csm_repl_lag_seconds gauge
		dell_csm_repl_lag_seconds{driver="csi-powerstore",policy_name="pg-1"} 42.5
	`
	err := testutil.CollectAndCompare(m.lagSeconds, strings.NewReader(expected))
	assert.NoError(t, err)
}

func TestSetBandwidthBytes(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetBandwidthBytes("csi-powerstore", "pg-1", 1048576)

	expected := `
		# HELP dell_csm_repl_bandwidth_bytes Current replication traffic bandwidth in bytes per second.
		# TYPE dell_csm_repl_bandwidth_bytes gauge
		dell_csm_repl_bandwidth_bytes{driver="csi-powerstore",policy_name="pg-1"} 1.048576e+06
	`
	err := testutil.CollectAndCompare(m.bandwidthBytes, strings.NewReader(expected))
	assert.NoError(t, err)
}

func TestRecordActionSuccess(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.RecordAction("csi-powerstore", "FAILOVER", "success")

	val := testutil.ToFloat64(m.successTotal.WithLabelValues("csi-powerstore", "FAILOVER"))
	assert.Equal(t, float64(1), val, "success counter should be 1")

	ratio := testutil.ToFloat64(m.successRatio.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(1), ratio, "ratio should be 1.0 after one success")
}

func TestRecordActionFailure(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.RecordAction("csi-powerstore", "FAILOVER", "failure")

	val := testutil.ToFloat64(m.failureTotal.WithLabelValues("csi-powerstore", "FAILOVER"))
	assert.Equal(t, float64(1), val, "failure counter should be 1")

	ratio := testutil.ToFloat64(m.successRatio.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(0), ratio, "ratio should be 0.0 after one failure")
}

func TestRecordActionMixedResults(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.RecordAction("csi-powerstore", "FAILOVER", "success")
	m.RecordAction("csi-powerstore", "FAILOVER", "success")
	m.RecordAction("csi-powerstore", "FAILOVER", "failure")

	successVal := testutil.ToFloat64(m.successTotal.WithLabelValues("csi-powerstore", "FAILOVER"))
	assert.Equal(t, float64(2), successVal, "success counter should be 2")

	failureVal := testutil.ToFloat64(m.failureTotal.WithLabelValues("csi-powerstore", "FAILOVER"))
	assert.Equal(t, float64(1), failureVal, "failure counter should be 1")

	ratio := testutil.ToFloat64(m.successRatio.WithLabelValues("csi-powerstore"))
	assert.InDelta(t, 2.0/3.0, ratio, 0.001, "ratio should be ~0.667 after 2 success + 1 failure")
}

func TestSetControllerHealthHealthy(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetControllerHealth("csi-powerstore", true)

	val := testutil.ToFloat64(m.controllerHealth.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(1), val, "controller health should be 1 when healthy")
}

func TestSetControllerHealthUnhealthy(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetControllerHealth("csi-powerstore", false)

	val := testutil.ToFloat64(m.controllerHealth.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(0), val, "controller health should be 0 when unhealthy")
}

func TestSetControllerHealthToggle(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetControllerHealth("csi-powerstore", true)
	val := testutil.ToFloat64(m.controllerHealth.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(1), val)

	m.SetControllerHealth("csi-powerstore", false)
	val = testutil.ToFloat64(m.controllerHealth.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(0), val)

	m.SetControllerHealth("csi-powerstore", true)
	val = testutil.ToFloat64(m.controllerHealth.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(1), val)
}

func TestSetMetricsStaleStale(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetMetricsStale("csi-powerstore", true)

	val := testutil.ToFloat64(m.metricsStale.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(1), val, "metricsStale should be 1 when stale=true")
}

func TestSetMetricsStaleFresh(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetMetricsStale("csi-powerstore", false)

	val := testutil.ToFloat64(m.metricsStale.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(0), val, "metricsStale should be 0 when stale=false")
}

func TestSetMetricsStaleToggle(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetMetricsStale("csi-powerstore", true)
	val := testutil.ToFloat64(m.metricsStale.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(1), val)

	m.SetMetricsStale("csi-powerstore", false)
	val = testutil.ToFloat64(m.metricsStale.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(0), val)

	m.SetMetricsStale("csi-powerstore", true)
	val = testutil.ToFloat64(m.metricsStale.WithLabelValues("csi-powerstore"))
	assert.Equal(t, float64(1), val)
}

func TestSetLastCollectionTimestamp(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	m.SetLastCollectionTimestamp("csi-powerstore", 1234)

	expected := `
		# HELP dell_csm_repl_last_collection_timestamp_seconds Unix timestamp of the last complete replication metrics collection cycle.
		# TYPE dell_csm_repl_last_collection_timestamp_seconds gauge
		dell_csm_repl_last_collection_timestamp_seconds{driver="csi-powerstore"} 1234
	`
	assert.NoError(t, testutil.CollectAndCompare(m.lastCollectionTimestamp, strings.NewReader(expected)))
}

func TestGetGlobalReplicationMetricsBeforeInit(t *testing.T) {
	// Reset the global to nil for this test
	SetGlobalReplicationMetrics(nil)

	got := GetGlobalReplicationMetrics()
	assert.Nil(t, got, "GetGlobalReplicationMetrics should return nil before initialization")
}

func TestSetAndGetGlobalReplicationMetrics(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	SetGlobalReplicationMetrics(m)
	got := GetGlobalReplicationMetrics()
	assert.Equal(t, m, got, "GetGlobalReplicationMetrics should return the same instance that was set")

	// Clean up
	SetGlobalReplicationMetrics(nil)
}

func TestMultipleDriversTrackedIndependently(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	m := NewReplicationMetrics(registry)

	// Driver A: 3 success, 1 failure => ratio = 0.75
	m.RecordAction("driver-a", "FAILOVER", "success")
	m.RecordAction("driver-a", "FAILOVER", "success")
	m.RecordAction("driver-a", "FAILOVER", "success")
	m.RecordAction("driver-a", "FAILOVER", "failure")

	// Driver B: 1 success, 1 failure => ratio = 0.5
	m.RecordAction("driver-b", "REPROTECT", "success")
	m.RecordAction("driver-b", "REPROTECT", "failure")

	ratioA := testutil.ToFloat64(m.successRatio.WithLabelValues("driver-a"))
	assert.InDelta(t, 0.75, ratioA, 0.001, "driver-a ratio should be 0.75")

	ratioB := testutil.ToFloat64(m.successRatio.WithLabelValues("driver-b"))
	assert.InDelta(t, 0.5, ratioB, 0.001, "driver-b ratio should be 0.5")
}
