package replication

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/dell/csm-replication/pkg/connection"
	csiext "github.com/dell/dell-csi-extensions/replication"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	ctrl "sigs.k8s.io/controller-runtime"
)

func createFakeConnection() *grpc.ClientConn {
	conn, _ := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	return conn
}

type fields struct {
	conn           *grpc.ClientConn
	log            logr.Logger
	timeout        time.Duration
	rgPendingState *connection.PendingState
}

func Test_replication_CreateRemoteVolume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	type args struct {
		ctx          context.Context
		volumeHandle string
		params       map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csiext.CreateRemoteVolumeResponse
		wantErr bool
	}{
		{"CreateRemoteVolume Failed", fields{createFakeConnection(), ctrl.Log.WithName("/replication.v1.Replication/CreateRemoteVolume"), 10, &connection.PendingState{}}, args{ctx, "csi-replicator-vol", map[string]string{"key1": "val1"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &replication{
				conn:           tt.fields.conn,
				log:            tt.fields.log,
				timeout:        tt.fields.timeout,
				rgPendingState: tt.fields.rgPendingState,
			}
			got, err := r.CreateRemoteVolume(tt.args.ctx, tt.args.volumeHandle, tt.args.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("replication.CreateRemoteVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("replication.CreateRemoteVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_replication_DeleteLocalVolume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	type args struct {
		ctx          context.Context
		volumeHandle string
		params       map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csiext.DeleteLocalVolumeResponse
		wantErr bool
	}{
		{"DeleteLocalVolume Failed", fields{createFakeConnection(), ctrl.Log.WithName("DeleteLocalVolume"), 10, &connection.PendingState{}}, args{ctx, "csi-replicator-vol", map[string]string{"key1": "val1"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &replication{
				conn:           tt.fields.conn,
				log:            tt.fields.log,
				timeout:        tt.fields.timeout,
				rgPendingState: tt.fields.rgPendingState,
			}
			got, err := r.DeleteLocalVolume(tt.args.ctx, tt.args.volumeHandle, tt.args.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("replication.DeleteLocalVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("replication.DeleteLocalVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_replication_CreateStorageProtectionGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	type args struct {
		ctx          context.Context
		volumeHandle string
		params       map[string]string
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csiext.CreateStorageProtectionGroupResponse
		wantErr bool
	}{
		{"CreateStorageProtectionGroup Failed", fields{createFakeConnection(), ctrl.Log.WithName("CreateStorageProtectionGroup"), 10, &connection.PendingState{}}, args{ctx, "csi-replicator-vol", map[string]string{"key1": "val1"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &replication{
				conn:           tt.fields.conn,
				log:            tt.fields.log,
				timeout:        tt.fields.timeout,
				rgPendingState: tt.fields.rgPendingState,
			}
			got, err := r.CreateStorageProtectionGroup(tt.args.ctx, tt.args.volumeHandle, tt.args.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("replication.CreateStorageProtectionGroup() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("replication.CreateStorageProtectionGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_replication_DeleteStorageProtectionGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	groupID := "rg1"
	type args struct {
		ctx             context.Context
		groupID         string
		groupAttributes map[string]string
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    error
		wantErr bool
	}{
		{"DeleteStorageProtectionGroup Failed", fields{createFakeConnection(), ctrl.Log.WithName("DeleteStorageProtectionGroup"), 10, &connection.PendingState{}}, args{ctx, groupID, map[string]string{"key1": "val1"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &replication{
				conn:           tt.fields.conn,
				log:            tt.fields.log,
				timeout:        tt.fields.timeout,
				rgPendingState: tt.fields.rgPendingState,
			}
			err := r.DeleteStorageProtectionGroup(tt.args.ctx, tt.args.groupID, tt.args.groupAttributes)
			if (err != nil) != tt.wantErr {
				t.Errorf("replication.DeleteStorageProtectionGroup() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_replication_GetStorageProtectionGroupStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	protectionGroupID := "rg1"
	type args struct {
		ctx               context.Context
		protectionGroupID string
		attributes        map[string]string
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csiext.GetStorageProtectionGroupStatusResponse
		wantErr bool
	}{
		{"GetStorageProtectionGroupStatus Failed", fields{createFakeConnection(), ctrl.Log.WithName("GetStorageProtectionGroupStatus"), 10, &connection.PendingState{}}, args{ctx, protectionGroupID, map[string]string{"key1": "val1"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &replication{
				conn:           tt.fields.conn,
				log:            tt.fields.log,
				timeout:        tt.fields.timeout,
				rgPendingState: tt.fields.rgPendingState,
			}
			_, err := r.GetStorageProtectionGroupStatus(tt.args.ctx, tt.args.protectionGroupID, tt.args.attributes)
			if (err != nil) != tt.wantErr {
				t.Errorf("replication.GetStorageProtectionGroupStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_replication_ExecuteAction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	protectionGroupID := "rg1"
	remoteProtectionGroupID := "remoteProtectionGID"
	remoteAttributes := map[string]string{"key2": "val2"}
	type args struct {
		ctx                     context.Context
		protectionGroupID       string
		actionType              *csiext.ExecuteActionRequest_Action
		attributes              map[string]string
		remoteProtectionGroupID string
		remoteAttributes        map[string]string
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csiext.ExecuteActionResponse
		wantErr bool
	}{
		{"ExecuteAction Failed", fields{createFakeConnection(), ctrl.Log.WithName("ExecuteAction"), 10, &connection.PendingState{}}, args{ctx, protectionGroupID, nil, map[string]string{"key1": "val1"}, remoteProtectionGroupID, remoteAttributes}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &replication{
				conn:           tt.fields.conn,
				log:            tt.fields.log,
				timeout:        tt.fields.timeout,
				rgPendingState: tt.fields.rgPendingState,
			}
			_, err := r.ExecuteAction(tt.args.ctx, tt.args.protectionGroupID, tt.args.actionType, tt.args.attributes, tt.args.remoteProtectionGroupID, tt.args.remoteAttributes)
			if (err != nil) != tt.wantErr {
				t.Errorf("replication.GetStorageProtectionGroupStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
