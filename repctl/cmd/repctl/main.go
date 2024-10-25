/*
 Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package main

import (
	"os"

	"github.com/rifflock/lfshook"
	log "github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"

	"github.com/dell/repctl/pkg/cmd"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	terminal := &prefixed.TextFormatter{
		DisableColors:   false,
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
		ForceFormatting: true,
	}

	file := &prefixed.TextFormatter{
		DisableColors:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
		ForceFormatting: true,
	}

	log.SetFormatter(terminal)
	log.SetOutput(os.Stdout)

	pathMap := lfshook.PathMap{
		log.DebugLevel: "./repctl.log",
		log.InfoLevel:  "./repctl.log",
		log.WarnLevel:  "./repctl.log",
		log.ErrorLevel: "./repctl.log",
		log.FatalLevel: "./repctl.log",
	}
	log.AddHook(lfshook.NewHook(pathMap, file))
}

func main() {
	repctl := cobra.Command{
		Use:     "repctl",
		Short:   "repctl is CLI tool for managing replication in Kubernetes",
		Long:    "repctl is CLI tool for managing replication in Kubernetes",
		Version: "v1.10.0",

		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			metadata.Init(viper.GetString(config.ReplicationPrefix))
		},
	}

	// Init global flags
	repctl.PersistentFlags().StringSlice("clusters", []string{""}, "remote cluster id")
	_ = viper.BindPFlag(config.Clusters, repctl.PersistentFlags().Lookup("clusters"))

	repctl.PersistentFlags().String("driver", "", "name of the driver")
	_ = viper.BindPFlag(config.Driver, repctl.PersistentFlags().Lookup("driver"))

	repctl.PersistentFlags().String("rg", "", "replication group name")
	_ = viper.BindPFlag(config.ReplicationGroup, repctl.PersistentFlags().Lookup("rg"))

	repctl.PersistentFlags().String("prefix", "replication.storage.dell.com", "prefix for replication (default is replication.storage.dell.com)")
	_ = viper.BindPFlag(config.ReplicationPrefix, repctl.PersistentFlags().Lookup("prefix"))

	repctl.PersistentFlags().BoolP("verbose", "l", false, "enables verbosity and debug output")
	_ = viper.BindPFlag(config.Verbose, repctl.PersistentFlags().Lookup("verbose"))

	// Add highest level commands
	repctl.AddCommand(cmd.GetListCommand())
	repctl.AddCommand(cmd.GetClusterCommand())
	repctl.AddCommand(cmd.GetCreateCommand())
	repctl.AddCommand(cmd.GetFailoverCommand())
	repctl.AddCommand(cmd.GetFailbackCommand())
	repctl.AddCommand(cmd.GetReprotectCommand())
	repctl.AddCommand(cmd.GetSwapCommand())
	repctl.AddCommand(cmd.GetExecCommand())
	repctl.AddCommand(cmd.GetEditCommand())
	repctl.AddCommand(cmd.GetMigrateCommand())
	repctl.AddCommand(cmd.GetSnapshotCommand())

	err := repctl.Execute()
	if err != nil {
		log.Fatalf("repctl: error: %s\n", err.Error())
	}
	os.Exit(0)
}
