/*
 Copyright © 2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package utils

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/dell/gobrick/pkg/scsi"
	log "github.com/sirupsen/logrus"
)

// RescanNode is called to rescan the nodes
func RescanNode(ctx context.Context) error {
	log.Info("Calling NodeRescan")
	return rescanSCSIHostAll(ctx)
}

func rescanSCSIHostAll(ctx context.Context) error {
	hostsDir := "/sys/class/scsi_host"
	hostFiles, err := os.ReadDir(fmt.Sprintf("%s/", hostsDir))
	if err != nil {
		log.Errorf("rescanSCSIHOSTALL failed to read scsi_host dir, err: %s", err.Error())
		return err
	}
	log.Infof("found (%d) files in hostsDir (%s)", len(hostFiles), hostsDir)
	scsiHost := scsi.NewSCSI("")
	for host := 0; host < len(hostFiles); host++ {
		// at least one target port not found, do full scsi rescan
		hctl := scsi.HCTL{
			Host:    strconv.Itoa(host),
			Lun:     "-",
			Channel: "-",
			Target:  "-",
		}
		err := scsiHost.RescanSCSIHostByHCTL(ctx, hctl)
		if err != nil {
			log.Error(ctx, err.Error())
			continue
		}
	}
	return nil
}
