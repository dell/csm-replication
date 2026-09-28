/*
 Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"context"
	"strings"

	controller "github.com/dell/csm-replication/controllers"
	storageV1 "k8s.io/api/storage/v1"
)

func shouldContinue(_ context.Context, class *storageV1.StorageClass, driverName string) bool {
	// Check for the driver match
	if class.Provisioner != driverName {
		return false
	}

	// Check for the replication params to make sure, this PVC
	// has a replica created for it
	if value, ok := class.Parameters[controller.StorageClassReplicationParam]; !ok || value != controller.StorageClassReplicationParamEnabledValue {
		return false
	}

	// Check for PowerMax SRDF Metro and skip since it is only supported at the driver level
	if value, ok := class.Parameters["replication.storage.dell.com/RdfMode"]; ok && strings.ToUpper(value) == "METRO" {
		return false
	}

	// Check for PowerStore Metro and skip since it is only supported at the driver level
	if value, ok := class.Parameters["replication.storage.dell.com/mode"]; ok && strings.ToUpper(value) == "METRO" {
		return false
	}

	// Check for the remote-storage-class-param on the local storage-class
	_, ok := class.Parameters[controller.StorageClassRemoteStorageClassParam]
	if !ok {
		return false
	}
	return true
}
