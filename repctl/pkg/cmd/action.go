/*
 *
 * Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

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

package cmd

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetExecCommand returns 'exec' cobra command
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
				log.Error(fmt.Sprintf("exec: error in supported action: %s", err.Error()))
				return
			}

			if verbose {
				log.Printf("Proceeding for action (%s)...", action)
			}
			configFolder, err := getClustersFolderPathFunction(clusterPath)
			if err != nil {
				log.Fatalf("exec: error getting clusters folder path: %s", err.Error())
			}
			if verbose {
				log.Printf("reading cluster configs...")
			}
			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters([]string{}, configFolder)
			if err != nil {
				log.Fatalf("exec: error in initializing cluster info: %s", err.Error())
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
						log.Printf("found RG (%s) on cluster (%s), updating spec...", rg.Name, cluster.GetID())
					}
					rg.Spec.Action = action
					if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
						log.Fatalf("exec: error executing action %s", err.Error())
					}
					log.Printf("RG (%s) on cluster (%s), successfully updated with action: (%s)", rg.Name, cluster.GetID(), action)
					break
				}
			}
			if !found {
				log.Error(fmt.Sprintf("exec: no matching cluster found with RG as source (%s)", rgName))
				return
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
	return "", fmt.Errorf("not a supported action")
}

// GetRGAndClusterFromRGID returns a ClusterInterface and DellCSIReplicationGroup given the rgID
// filter can be used to fetch a source, a target
func GetRGAndClusterFromRGID(configFolder, rgID, filter string) (k8s.ClusterInterface, *repv1.DellCSIReplicationGroup, error) {
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{}, configFolder)
	if err != nil {
		log.Fatalf("exec: error in initializing cluster info: %s", err.Error())
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
