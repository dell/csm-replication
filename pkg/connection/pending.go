/*
 Copyright © 2021-2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package connection

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RgIDType represents replication group id
type RgIDType string

// PendingState type limits the number of pending requests by making sure there are no other requests for the same RG,
// otherwise a "pending" error is returned.
// Additionally, no more than maxPending requests are processed at a time without returning an "overload" error.
type PendingState struct {
	MaxPending   int
	npending     int
	pendingMutex sync.Mutex
	pendingMap   map[RgIDType]time.Time
	Log          logr.Logger
}

// CheckAndUpdatePendingState sets state of current RG as pending if allowed by current capacity
func (rgID RgIDType) CheckAndUpdatePendingState(ps *PendingState) error {
	ps.pendingMutex.Lock()
	defer ps.pendingMutex.Unlock()
	if ps.pendingMap == nil {
		ps.pendingMap = make(map[RgIDType]time.Time)
	}
	startTime := ps.pendingMap[rgID]
	if startTime.IsZero() == false {
		ps.Log.Info(fmt.Sprintf("rgID %s pending %s", rgID, time.Now().Sub(startTime)))
		return status.Errorf(codes.Unavailable, "pending")
	}
	if ps.MaxPending > 0 && ps.npending >= ps.MaxPending {
		return status.Errorf(codes.Unavailable, "overload")
	}
	ps.pendingMap[rgID] = time.Now()
	ps.npending++
	return nil
}

// ClearPending removes current replication group from pending list
func (rgID RgIDType) ClearPending(ps *PendingState) {
	ps.pendingMutex.Lock()
	defer ps.pendingMutex.Unlock()
	if ps.pendingMap == nil {
		ps.pendingMap = make(map[RgIDType]time.Time)
	}
	delete(ps.pendingMap, rgID)
	ps.npending--
}
