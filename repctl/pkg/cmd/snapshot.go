/*
 Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.

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

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetSnapshotCommand returns 'snapshot' cobra command
/* #nosec G104 */
func GetSnapshotCommand() *cobra.Command {
	snapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "allows to execute snapshot actionat the specified cluster or rg",
		Example: `
For single cluster config:
./repctl --rg <rg-id> snapshot`,
		Long: `
This command will perform a snapshot at specified cluster or at the RG.\n
Note: More information to be added.`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputCluster := viper.GetString("target")
			verbose := viper.GetBool(config.Verbose)
			wait := viper.GetBool("snapshot-wait")
			input, res := verifyInputForSnapshotAction(inputCluster, rgName)

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("reprotect: error getting clusters folder path: %s\n", err.Error())
			}

			if input == "rg" {
				createSnapshot(configFolder, res, verbose, wait)
			} else {
				log.Fatal("Unexpected input received")
			}
		},
	}

	snapshotCmd.Flags().String("at", "", "target to execute the snapshot")
	_ = viper.BindPFlag("target", snapshotCmd.Flags().Lookup("at"))

	snapshotCmd.Flags().Bool("wait", false, "wait for action to complete")
	_ = viper.BindPFlag("snapshot-waitt", snapshotCmd.Flags().Lookup("wait"))
	return snapshotCmd
}

func verifyInputForSnapshotAction(input string, rg string) (res string, tgt string) {
	if input == "" {
		if rg != "" {
			input = rg
		} else {
			log.Fatalf("snapshot: wrong input, no input provided. Replication Group ID is needed.\n")
		}
	}

	configFolder, err := getClustersFolderPath("/.repctl/clusters/")
	if err != nil {
		log.Fatalf("snapshot: error getting clusters folder path: %s", err.Error())
	}

	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{}, configFolder)
	if err != nil {
		log.Fatalf("error in initializing cluster info: %s", err.Error())
	}

	for _, cluster := range clusters.Clusters {
		rgList, err := cluster.ListReplicationGroups(context.Background())
		if err != nil {
			log.Printf("Encountered error during filtering persistent volume claims. Error: %s",
				err.Error())
			continue
		}
		for _, value := range rgList.Items {
			if value.Name == input {
				return "rg", input
			}
		}
	}
	return "", ""
}

func createSnapshot(configFolder, rgName string, verbose bool, wait bool) {
	if verbose {
		log.Printf("fetching RG and cluster info...\n")
	}

	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "")
	if err != nil {
		log.Fatalf("snapshot to RG: error fetching RG info: (%s)\n", err.Error())
	}

	if verbose {
		log.Printf("found specified RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
		log.Print("updating spec...", rg.Name)

	}

	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil {
		log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
		return
	}

	rg.Spec.Action = config.ActionCreateSnapshot
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		log.Fatalf("snapshot: error executing UpdateAction %s\n", err.Error())
		return
	}

	if wait {
		success := waitForStateToUpdate(rgName, cluster, rLinkState)
		if success {
			log.Printf("Successfully executed action on RG (%s)\n", rg.Name)
			return
		}

		log.Printf("RG (%s), timed out with action: failover\n", rg.Name)
		return
	}

	log.Printf("RG (%s), successfully updated with action: snapshot\n", rg.Name)
}
