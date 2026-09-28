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

package csireplicator

import (
	"os"
	"testing"

	"github.com/dell/csm-replication/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMain(m *testing.M) {
	metrics.SetGlobalReplicationMetrics(metrics.NewReplicationMetrics(prometheus.NewRegistry()))
	metrics.SetGlobalSRDFMetrics(metrics.NewSRDFMetrics(prometheus.NewRegistry()))
	code := m.Run()
	metrics.SetGlobalReplicationMetrics(nil)
	metrics.SetGlobalSRDFMetrics(nil)
	os.Exit(code)
}
