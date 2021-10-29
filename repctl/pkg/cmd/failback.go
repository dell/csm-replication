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

	"github.com/dell/repctl/pkg/k8s"

	"github.com/dell/repctl/pkg/config"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetFailbackCommand returns 'failback' cobra command
/* #nosec G104 */
func GetFailbackCommand() *cobra.Command {
	failbackCmd := &cobra.Command{
		Use:   "failback",
		Short: "allows to execute failback action from target cluster/rg to source cluster/rg",
		Example: `
For multi-cluster config:
./repctl --rg <rg-id> failback --to-cluster cluster1
./repctl --rg <rg-id> failback --to-cluster cluster1 --discard
For single cluster config:
./repctl failback --to-rg <rg-id>
./repctl failback --to-rg <rg-id> --discard
`,
		Long: `
This command will perform a planned failback to a cluster or to an RG.
To perform failback to a cluster, use --to-cluster <clusterID> with <rg-id> and to do failback to RG, use --to-rg <rg-id>. repctl will patch the CR at source site with action FAILBACK_LOCAL.
With --discard, this command will perform an failback but discard any writes at target. repctl will patch the CR at source site with action ACTION_FAILBACK_DISCARD_CHANGES_LOCAL`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputSourceCluster := viper.GetString("src")
			inputSourceRG := viper.GetString("src-rg")
			discard := viper.GetBool("discard")
			verbose := viper.GetBool(config.Verbose)
			verifyInputForAction(inputSourceRG, inputSourceCluster)

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failback: error getting clusters folder path: %s\n", err.Error())
			}

			if inputSourceCluster != "" {
				failbackToCluster(configFolder, inputSourceCluster, rgName, discard, verbose)
			} else {
				failbackToRG(configFolder, inputSourceRG, discard, verbose)
			}
		},
	}

	failbackCmd.Flags().String("to-cluster", "", "cluster to which execute failback")
	_ = viper.BindPFlag("src", failbackCmd.Flags().Lookup("to-cluster"))

	failbackCmd.Flags().String("to-rg", "", "RG to which execute failover")
	_ = viper.BindPFlag("src-rg", failbackCmd.Flags().Lookup("to-rg"))

	failbackCmd.Flags().Bool("discard", false, "flag marking failback to discard any writes at target")
	_ = viper.BindPFlag("discard", failbackCmd.Flags().Lookup("discard"))

	return failbackCmd
}

func failbackToRG(configFolder, rgName string, discard, verbose bool) {
	if verbose {
		log.Printf("fetching RG and cluster info...\n")
	}
	// fetch the source RG and the cluster info
	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "src")
	if err != nil {
		log.Fatalf("failback to RG: error fetching source RG info: (%s)\n", err.Error())
	}
	if verbose {
		log.Printf("found RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
	}

	rg.Spec.Action = config.ActionFailbackLocal
	if discard {
		rg.Spec.Action = config.ActionFailbackLocalDiscard
		if verbose {
			log.Print("found flag for discarding local changes...")
		}
	}
	if verbose {
		log.Print("updating spec...")
	}
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		log.Fatalf("failback: error executing UpdateAction %s\n", err.Error())
	}
	log.Printf("RG (%s), successfully updated with action: faiback\n", rg.Name)
}

func failbackToCluster(configFolder, inputSourceCluster, rgName string, discard, verbose bool) {
	if verbose {
		log.Print("reading cluster configs...")
	}
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{inputSourceCluster}, configFolder)
	if err != nil {
		log.Fatalf("failback: error in initializing cluster info: %s\n", err.Error())
	}
	sourceCluster := clusters.Clusters[0]
	if verbose {
		log.Printf("found source cluster (%s)\n", sourceCluster.GetID())
	}
	rg, err := sourceCluster.GetReplicationGroups(context.Background(), rgName)
	if err != nil {
		log.Fatalf("failback: error in fecthing RG info: %s\n", err.Error())
	}
	if verbose {
		log.Printf("found RG (%s) on cluster (%s)...\n", rg.Name, sourceCluster.GetID())
	}
	if !rg.Status.ReplicationLinkState.IsSource {
		log.Fatalf("failback: error executing failback to target site.")
	}
	rg.Spec.Action = config.ActionFailbackLocal
	if discard {
		rg.Spec.Action = config.ActionFailbackLocalDiscard
		if verbose {
			log.Print("found flag for discarding local changes...")
		}
	}
	if verbose {
		log.Printf("found RG (%s) on source cluster, updating spec...\n", rg.Name)
	}
	if err := sourceCluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		log.Fatalf("failback: error executing UpdateAction %s\n", err.Error())
	}
	log.Printf("RG (%s), successfully updated with action: faiback\n", rg.Name)
}
