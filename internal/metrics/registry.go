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

import "sync"

var (
	globalReplicationMetrics *ReplicationMetrics
	globalSRDFMetrics        *SRDFMetrics
	mu                       sync.RWMutex
)

// SetGlobalReplicationMetrics sets the global ReplicationMetrics singleton.
func SetGlobalReplicationMetrics(m *ReplicationMetrics) {
	mu.Lock()
	defer mu.Unlock()
	globalReplicationMetrics = m
}

// GetGlobalReplicationMetrics returns the global ReplicationMetrics singleton, or nil if not initialized.
func GetGlobalReplicationMetrics() *ReplicationMetrics {
	mu.RLock()
	defer mu.RUnlock()
	return globalReplicationMetrics
}

// SetGlobalSRDFMetrics sets the global SRDFMetrics singleton.
func SetGlobalSRDFMetrics(m *SRDFMetrics) {
	mu.Lock()
	defer mu.Unlock()
	globalSRDFMetrics = m
}

// GetGlobalSRDFMetrics returns the global SRDFMetrics singleton, or nil if not initialized.
func GetGlobalSRDFMetrics() *SRDFMetrics {
	mu.RLock()
	defer mu.RUnlock()
	return globalSRDFMetrics
}
