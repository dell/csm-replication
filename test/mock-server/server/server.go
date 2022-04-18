package server

import (
	"bytes"
	context2 "context"
	"encoding/json"
	"github.com/dell/dell-csi-extensions/migration"

	"fmt"
	commonext "github.com/dell/dell-csi-extensions/common"
	"github.com/dell/dell-csi-extensions/replication"
	"golang.org/x/net/context"
	"io/ioutil"
	"net/http"
)

// Replication mock controller that implements replication related calls
type Replication struct{}

// VolumeMigrate - mocks Migrate function
func (s *Replication) VolumeMigrate(ctx context2.Context, request *migration.VolumeMigrateRequest) (*migration.VolumeMigrateResponse, error) {
	panic("change IP")
	rep := &migration.VolumeMigrateResponse{
		MigratedVolume: &migration.Volume{
			CapacityBytes: 3221225472,
			VolumeId:      "k8s-af51320e72=_=_=155=_=_=System=_=_=cluster1",
			FsType:        "",
			ReadOnly:      false,
			MountOptions:  nil,
			VolumeContext: map[string]string{
				"AccessZone":        "System",
				"AzServiceIP":       "IP",
				"ClusterName":       "cluster1",
				"ID":                "155",
				"Name":              "k8s-af51320e72",
				"Path":              "/ifs/data/csi/k8s-af51320e72",
				"RootClientEnabled": "false",
			},
		},
	}
	return rep, nil
}

// GetMigrationCapabilities - mocks GetMigrationCapabilities func
func (s *Replication) GetMigrationCapabilities(ctx context2.Context, request *migration.GetMigrationCapabilityRequest) (*migration.GetMigrationCapabilityResponse, error) {
	return &migration.GetMigrationCapabilityResponse{
		Capabilities: []*migration.MigrationCapability{
			{
				Type: &migration.MigrationCapability_Rpc{
					Rpc: &migration.MigrationCapability_RPC{
						Type: migration.MigrateTypes_NON_REPL_TO_REPL,
					},
				},
			},
			{
				Type: &migration.MigrationCapability_Rpc{
					Rpc: &migration.MigrationCapability_RPC{
						Type: migration.MigrateTypes_REPL_TO_NON_REPL,
					},
				},
			},
			{
				Type: &migration.MigrationCapability_Rpc{
					Rpc: &migration.MigrationCapability_RPC{
						Type: migration.MigrateTypes_VERSION_UPGRADE,
					},
				},
			},
		},
	}, nil
}

// ProbeController calls stub for ProbeController
func (s *Replication) ProbeController(ctx context.Context, in *commonext.ProbeControllerRequest) (*commonext.ProbeControllerResponse, error) {
	out := &commonext.ProbeControllerResponse{}
	err := FindStub("Replication", "ProbeController", in, out)
	return out, err
}

// GetReplicationCapabilities calls stub for GetReplicationCapabilities
func (s *Replication) GetReplicationCapabilities(ctx context.Context, in *replication.GetReplicationCapabilityRequest) (*replication.GetReplicationCapabilityResponse, error) {
	out := &replication.GetReplicationCapabilityResponse{}
	outTemp := make(map[string][]int)
	err := FindStub("Replication", "GetReplicationCapabilities", in, &outTemp)
	for _, capability := range outTemp["capabilities"] {
		out.Capabilities = append(out.Capabilities, &replication.ReplicationCapability{
			Type: &replication.ReplicationCapability_Rpc{
				Rpc: &replication.ReplicationCapability_RPC{
					Type: replication.ReplicationCapability_RPC_Type(capability),
				},
			},
		})
	}
	for _, action := range outTemp["actions"] {
		out.Actions = append(out.Actions, &replication.SupportedActions{
			Actions: &replication.SupportedActions_Type{
				Type: replication.ActionTypes(action),
			},
		})
	}
	return out, err
}

// CreateStorageProtectionGroup calls stub for CreateStorageProtectionGroup
func (s *Replication) CreateStorageProtectionGroup(ctx context.Context, in *replication.CreateStorageProtectionGroupRequest) (*replication.CreateStorageProtectionGroupResponse, error) {
	out := &replication.CreateStorageProtectionGroupResponse{}
	err := FindStub("Replication", "CreateStorageProtectionGroup", in, out)
	return out, err
}

// CreateRemoteVolume calls stub for CreateRemoteVolume
func (s *Replication) CreateRemoteVolume(ctx context.Context, in *replication.CreateRemoteVolumeRequest) (*replication.CreateRemoteVolumeResponse, error) {
	out := &replication.CreateRemoteVolumeResponse{}
	err := FindStub("Replication", "CreateRemoteVolume", in, out)
	return out, err
}

// DeleteStorageProtectionGroup calls stub for DeleteStorageProtectionGroup
func (s *Replication) DeleteStorageProtectionGroup(ctx context.Context, in *replication.DeleteStorageProtectionGroupRequest) (*replication.DeleteStorageProtectionGroupResponse, error) {
	out := &replication.DeleteStorageProtectionGroupResponse{}
	err := FindStub("Replication", "DeleteStorageProtectionGroup", in, out)
	return out, err
}

// GetStorageProtectionGroupStatus calls stub for GetStorageProtectionGroupStatus
func (s *Replication) GetStorageProtectionGroupStatus(ctx context.Context, in *replication.GetStorageProtectionGroupStatusRequest) (*replication.GetStorageProtectionGroupStatusResponse, error) {
	out := &replication.GetStorageProtectionGroupStatusResponse{}
	err := FindStub("Replication", "GetStorageProtectionGroupStatus", in, out)
	return out, err
}

// ExecuteAction calls stub for ExecuteAction
func (s *Replication) ExecuteAction(ctx context.Context, in *replication.ExecuteActionRequest) (*replication.ExecuteActionResponse, error) {
	out := &replication.ExecuteActionResponse{}
	err := FindStub("Replication", "ExecuteAction", in, out)
	return out, err
}

type payload struct {
	Service string      `json:"service"`
	Method  string      `json:"method"`
	Data    interface{} `json:"data"`
}

type response struct {
	Data  interface{} `json:"data"`
	Error string      `json:"error"`
}

// FindStub makes request to mock grpc server
func FindStub(service, method string, in, out interface{}) error {
	url := "http://localhost:4771/find"
	pyl := payload{
		Service: service,
		Method:  method,
		Data:    in,
	}
	byt, err := json.Marshal(pyl)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(byt)
	resp, err := http.DefaultClient.Post(url, "application/json", reader)
	if err != nil {
		return fmt.Errorf("error request to stub server %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf(string(body))
	}

	respRPC := new(response)
	err = json.NewDecoder(resp.Body).Decode(respRPC)
	if err != nil {
		return fmt.Errorf("decoding json response %v", err)
	}

	if respRPC.Error != "" {
		return fmt.Errorf(respRPC.Error)
	}

	data, _ := json.Marshal(respRPC.Data)
	return json.Unmarshal(data, out)
}
