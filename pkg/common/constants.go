/*
 Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package common

import (
	"fmt"
	"github.com/maxan98/logrusr"
	"github.com/sirupsen/logrus"
	"strings"
)

// Constants
const (
	DefaultConfigFileName     = "config"
	DefaultConfigDir          = "deploy"
	EnvWatchNameSpace         = "X_CSI_REPLICATION_WATCH_NAMESPACE"
	EnvConfigFileName         = "X_CSI_REPLICATION_CONFIG_FILE_NAME"
	EnvConfigDirName          = "X_CSI_REPLICATION_CONFIG_DIR"
	EnvInClusterConfig        = "X_CSI_REPLICATION_IN_CLUSTER"
	EnvUseConfFileFormat      = "X_CSI_USE_CONF_FILE_FORMAT"
	DefaultNameSpace          = "dell-replication-controller"
	DefaultDomain             = "replication.storage.dell.com"
	DellReplicationController = "dell-replication-controller"
	// DellCSIReplicator - Name of the sidecar controller manager
	DellCSIReplicator = "dell-csi-replicator"
	Monitoring        = "rg-monitoring"
)

const (
	// PanicLevel represents logrus panic log level
	PanicLevel = logrusr.PanicLevel
	// FatalLevel represents logrus fatal log level
	FatalLevel = logrusr.FatalLevel
	// ErrorLevel represents logrus error log level
	ErrorLevel = logrusr.ErrorLevel
	// WarnLevel represents logrus warning log level
	WarnLevel = logrusr.WarnLevel
	// InfoLevel represents logrus info log level
	InfoLevel = logrusr.InfoLevel
	// DebugLevel represents logrus debug log level
	DebugLevel = logrusr.DebugLevel
	// TraceLevel represents logrus trace log level
	TraceLevel = logrusr.TraceLevel
)

// ParseLevel returns correct logrus Level from given string name
func ParseLevel(level string) (logrus.Level, error) {
	switch strings.ToLower(level) {
	case "panic":
		return PanicLevel + 4, nil
	case "fatal":
		return FatalLevel + 4, nil
	case "error":
		return ErrorLevel + 4, nil
	case "warn", "warning":
		return WarnLevel + 4, nil
	case "info":
		return InfoLevel + 4, nil
	case "debug":
		return DebugLevel + 4, nil
	case "trace":
		return TraceLevel + 4, nil

	}

	return InfoLevel + 4, fmt.Errorf("not a valid logrus level, falling back to InfoLevel %s", level)
}
