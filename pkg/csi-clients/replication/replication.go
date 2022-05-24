/*
 Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package replication

import (
	"context"
	"github.com/dell/csm-replication/pkg/connection"
	"time"

	csiext "github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
)

// Replication is an interface that defines calls used for querying replication management calls to the driver
type Replication interface {
	CreateRemoteVolume(context.Context, string, map[string]string) (*csiext.CreateRemoteVolumeResponse, error)
	CreateStorageProtectionGroup(context.Context, string, map[string]string) (*csiext.CreateStorageProtectionGroupResponse, error)
	DeleteStorageProtectionGroup(context.Context, string, map[string]string) error
	ExecuteAction(context.Context, string, *csiext.ExecuteActionRequest_Action, map[string]string, string, map[string]string) (*csiext.ExecuteActionResponse, error)
	GetStorageProtectionGroupStatus(context.Context, string, map[string]string) (*csiext.GetStorageProtectionGroupStatusResponse, error)
}

// New returns new implementation of Replication interface
func New(conn *grpc.ClientConn, log logr.Logger, timeout time.Duration) Replication {
	return &replication{
		conn:           conn,
		log:            log,
		timeout:        timeout,
		rgPendingState: connection.PendingState{MaxPending: 50, Log: log},
	}
}

type replication struct {
	conn           *grpc.ClientConn
	log            logr.Logger
	timeout        time.Duration
	rgPendingState connection.PendingState
}

// GetStorageProtectionGroupStatus queries client for current status of protection group
func (r *replication) GetStorageProtectionGroupStatus(ctx context.Context, protectionGroupID string, attributes map[string]string) (*csiext.GetStorageProtectionGroupStatusResponse, error) {
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	client := csiext.NewReplicationClient(r.conn)

	req := &csiext.GetStorageProtectionGroupStatusRequest{
		ProtectionGroupId:         protectionGroupID,
		ProtectionGroupAttributes: attributes,
	}

	var rgID = connection.RgIDType(protectionGroupID)
	err := r.updatePendingState(rgID)
	if err != nil {
		return nil, err
	}
	defer rgID.ClearPending(&r.rgPendingState)

	res, err := client.GetStorageProtectionGroupStatus(tctx, req)

	return res, err
}

// ExecuteAction queries client to execute on of supported replication actions
func (r *replication) ExecuteAction(ctx context.Context, protectionGroupID string, actionType *csiext.ExecuteActionRequest_Action,
	attributes map[string]string, remoteProtectionGroupID string, remoteAttributes map[string]string) (*csiext.ExecuteActionResponse, error) {
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	client := csiext.NewReplicationClient(r.conn)

	var rgID = connection.RgIDType(protectionGroupID)
	err := r.updatePendingState(rgID)
	if err != nil {
		return nil, err
	}
	defer rgID.ClearPending(&r.rgPendingState)

	req := &csiext.ExecuteActionRequest{
		ProtectionGroupId:               protectionGroupID,
		ActionTypes:                     actionType,
		ProtectionGroupAttributes:       attributes,
		RemoteProtectionGroupId:         remoteProtectionGroupID,
		RemoteProtectionGroupAttributes: remoteAttributes,
	}

	res, err := client.ExecuteAction(tctx, req)
	return res, err
}

// CreateRemoteVolume queries client to make sure that remote volume was created
func (r *replication) CreateRemoteVolume(ctx context.Context, volumeHandle string, params map[string]string) (*csiext.CreateRemoteVolumeResponse, error) {
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	client := csiext.NewReplicationClient(r.conn)

	req := &csiext.CreateRemoteVolumeRequest{
		VolumeHandle: volumeHandle,
		Parameters:   params,
	}

	res, err := client.CreateRemoteVolume(tctx, req)
	return res, err
}

// CreateStorageProtectionGroup queries client to make sure that protection group  was created
func (r *replication) CreateStorageProtectionGroup(ctx context.Context, volumeHandle string, params map[string]string) (*csiext.CreateStorageProtectionGroupResponse, error) {
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	client := csiext.NewReplicationClient(r.conn)

	req := &csiext.CreateStorageProtectionGroupRequest{
		VolumeHandle: volumeHandle,
		Parameters:   params,
	}
	return client.CreateStorageProtectionGroup(tctx, req)
}

// DeleteStorageProtectionGroup sends deletion request of storage protection group
func (r *replication) DeleteStorageProtectionGroup(ctx context.Context, groupID string, groupAttributes map[string]string) error {
	tctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	client := csiext.NewReplicationClient(r.conn)

	var rgID = connection.RgIDType(groupID)
	err := r.updatePendingState(rgID)
	if err != nil {
		return err
	}
	defer rgID.ClearPending(&r.rgPendingState)

	req := &csiext.DeleteStorageProtectionGroupRequest{
		ProtectionGroupId:         groupID,
		ProtectionGroupAttributes: groupAttributes,
	}
	_, err = client.DeleteStorageProtectionGroup(tctx, req)
	return err
}

func (r *replication) updatePendingState(rgID connection.RgIDType) error {
	if err := rgID.CheckAndUpdatePendingState(&r.rgPendingState); err != nil {
		return err
	}
	return nil
}
