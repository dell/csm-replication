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
