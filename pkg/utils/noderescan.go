package utils

import (
	log "github.com/sirupsen/logrus"
	"os/exec"
)

func RescanNode() error {
	log.Info("Calling NodeRescan")
	rescanCmd := exec.Command("rescan-scsi-bus.sh")
	_, err := rescanCmd.Output()
	if err != nil {
		return err
	}
	return nil
}