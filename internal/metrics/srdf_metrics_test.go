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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gaugeValue(t *testing.T, g *prometheus.GaugeVec, labels prometheus.Labels) float64 {
	t.Helper()
	m := &dto.Metric{}
	require.NoError(t, g.With(labels).Write(m))
	return m.GetGauge().GetValue()
}

func TestNewSRDFMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewSRDFMetrics(reg)
	assert.NotNil(t, m)
	assert.NotNil(t, m.groupState)
	assert.NotNil(t, m.lagSeconds)
	assert.NotNil(t, m.bandwidth)
	assert.NotNil(t, m.linkStatus)
}

func TestSRDFMetrics_SetGroupState(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewSRDFMetrics(reg)

	labels := prometheus.Labels{
		LabelDriver:   "csi-powermax.dellemc.com",
		LabelRGName:   "rg-1",
		LabelRDFGroup: "1",
		LabelMode:     "ASYNC",
	}

	cases := []struct {
		state          string
		wantState      float64
		wantLinkStatus float64
	}{
		{"SYNCHRONIZED", 1, 1},
		{"SYNC_IN_PROGRESS", 2, 1},
		{"SUSPENDED", 3, 0},
		{"FAILEDOVER", 4, 0},
		{"UNKNOWN", 0, 0},
		{"EMPTY", 0, 0},
	}

	for _, tc := range cases {
		m.SetGroupState("csi-powermax.dellemc.com", "rg-1", "1", "ASYNC", tc.state)
		assert.Equal(t, tc.wantState, gaugeValue(t, m.groupState, labels), "state=%s groupState", tc.state)
		assert.Equal(t, tc.wantLinkStatus, gaugeValue(t, m.linkStatus, labels), "state=%s linkStatus", tc.state)
	}
}

func TestSRDFMetrics_SetLagSeconds(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewSRDFMetrics(reg)

	labels := prometheus.Labels{
		LabelDriver:   "csi-powermax.dellemc.com",
		LabelRGName:   "rg-1",
		LabelRDFGroup: "2",
		LabelMode:     "SYNC",
	}

	m.SetLagSeconds("csi-powermax.dellemc.com", "rg-1", "2", "SYNC", 30.5)
	assert.InDelta(t, 30.5, gaugeValue(t, m.lagSeconds, labels), 0.001)

	m.SetLagSeconds("csi-powermax.dellemc.com", "rg-1", "2", "SYNC", 0)
	assert.Equal(t, float64(0), gaugeValue(t, m.lagSeconds, labels))
}

func TestSRDFMetrics_SetBandwidth(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewSRDFMetrics(reg)

	labels := prometheus.Labels{
		LabelDriver:   "csi-powermax.dellemc.com",
		LabelRGName:   "rg-1",
		LabelRDFGroup: "3",
		LabelMode:     "ASYNC",
	}

	m.SetBandwidth("csi-powermax.dellemc.com", "rg-1", "3", "ASYNC", 1048576)
	assert.InDelta(t, 1048576.0, gaugeValue(t, m.bandwidth, labels), 0.001)
}

func TestSRDFMetrics_DeleteGroup(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewSRDFMetrics(reg)

	m.SetGroupState("csi-powermax.dellemc.com", "rg-1", "1", "ASYNC", "SYNCHRONIZED")
	m.SetLagSeconds("csi-powermax.dellemc.com", "rg-1", "1", "ASYNC", 5)
	m.SetBandwidth("csi-powermax.dellemc.com", "rg-1", "1", "ASYNC", 1024)
	m.DeleteGroup("csi-powermax.dellemc.com", "rg-1", "1", "ASYNC")

	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		for _, metric := range mf.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == LabelRGName && label.GetValue() == "rg-1" {
					t.Errorf("stale metric for rg-1 still present in %s", mf.GetName())
				}
			}
		}
	}
}

func TestParseSRDFGroupInfo(t *testing.T) {
	cases := []struct {
		pgID      string
		wantGroup string
		wantMode  string
	}{
		{"csi-rep-sg-default-1-ASYNC", "1", "ASYNC"},
		{"csi-rep-sg-ns-namespace-13-SYNC", "13", "SYNC"},
		{"csi-rep-sg-default-1-METRO", "", ""},
		{"csi-rep-sg-default-1", "", ""},
		{"invalid", "", ""},
	}

	for _, tc := range cases {
		g, mode := ParseSRDFGroupInfo(tc.pgID)
		assert.Equal(t, tc.wantGroup, g, "pgID=%s", tc.pgID)
		assert.Equal(t, tc.wantMode, mode, "pgID=%s", tc.pgID)
	}
}

func TestSRDFGlobalRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewSRDFMetrics(reg)

	SetGlobalSRDFMetrics(m)
	assert.Equal(t, m, GetGlobalSRDFMetrics())

	SetGlobalSRDFMetrics(nil)
	assert.Nil(t, GetGlobalSRDFMetrics())
}
