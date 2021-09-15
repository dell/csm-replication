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

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dell/csm-replication/api/v1alpha1"

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func GetExecCommand() *cobra.Command {
	execCmd := &cobra.Command{
		Use:   "exec",
		Short: "allows to execute a maintenance action on current source site",
		Example: `
For single or multi-cluster config:
./repctl --rg <rg-id> exec -a <ACTION>
repctl --rg <rg-id> exec -a suspend
repctl --rg <rg-id> exec -a resume
repctl --rg <rg-id> exec -a sync`,
		Long: `
This command will perform a maintenance on current source site. repctl will patch the CR with specified <ACTION>`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputAction := viper.GetString("action")
			verbose := viper.GetBool(config.Verbose)
			action, err := getSupportedMaintenanceAction(inputAction)
			if err != nil {
				fmt.Fprintf(os.Stderr, "exec: error in supported action: %s\n", err.Error())
				os.Exit(1)
			}
			if verbose {
				fmt.Printf("Proceeding for action (%s)...\n", action)
			}
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				fmt.Fprintf(os.Stderr, "exec: error getting clusters folder path: %s\n", err.Error())
				os.Exit(1)
			}
			if verbose {
				fmt.Println("reading cluster configs...")
			}
			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters([]string{}, configFolder)
			if err != nil {
				fmt.Fprintf(os.Stderr, "exec: error in initializing cluster info: %s\n", err.Error())
				os.Exit(1)
			}
			found := false
			for _, cluster := range clusters.Clusters {
				rg, err := cluster.GetReplicationGroups(context.Background(), rgName)
				if err != nil {
					// skip the cluster on which RG with rgName is not found
					continue
				}
				if rg.Status.ReplicationLinkState.IsSource {
					found = true
					if verbose {
						fmt.Printf("found RG (%s) on cluster (%s), updating spec...\n", rg.Name, cluster.GetID())
					}
					rg.Spec.Action = action
					if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
						fmt.Fprintf(os.Stderr, "exec: error executing action %s\n", err.Error())
						os.Exit(1)
					}
					fmt.Printf("RG (%s) on cluster (%s), successfully updated with action: (%s)\n", rg.Name, cluster.GetID(), action)
					break
				}
			}
			if !found {
				fmt.Fprintf(os.Stderr, "exec: no matching cluster found with RG as source (%s)\n", rgName)
				os.Exit(1)
			}
		},
	}
	execCmd.Flags().StringP("action", "a", "", "maintenance action to be executed")
	_ = viper.BindPFlag("action", execCmd.Flags().Lookup("action"))

	return execCmd
}

func getSupportedMaintenanceAction(action string) (string, error) {
	switch strings.ToLower(action) {
	case "resume":
		return config.ActionResume, nil
	case "suspend":
		return config.ActionSuspend, nil
	case "sync":
		return config.ActionSync, nil
	}
	return "", fmt.Errorf("Not a supported action")
}

// GetRGAndClusterFromRGID returns a ClusterInterface and DellCSIReplicationGroup given the rgID
// filter can be used to fetch a source, a target
func GetRGAndClusterFromRGID(configFolder, rgID, filter string) (k8s.ClusterInterface, *v1alpha1.DellCSIReplicationGroup, error) {
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{}, configFolder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec: error in initializing cluster info: %s\n", err.Error())
		os.Exit(1)
	}
	for _, cluster := range clusters.Clusters {
		rg, err := cluster.GetReplicationGroups(context.Background(), rgID)
		if err != nil {
			// skip the cluster on which RG with rgID is not found
			continue
		}
		if filter == "" {
			return cluster, rg, nil
		}
		if (rg.Status.ReplicationLinkState.IsSource && filter == "src") || (!rg.Status.ReplicationLinkState.IsSource && filter == "tgt") {
			return cluster, rg, nil
		}
	}
	return nil, nil, fmt.Errorf("no matching cluster having (%s) RG:(%s) found", filter, rgID)
}
