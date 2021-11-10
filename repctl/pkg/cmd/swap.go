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
	"github.com/spf13/viper"

	"github.com/spf13/cobra"
)

// GetSwapCommand returns 'swap' cobra command
/* #nosec G104 */
func GetSwapCommand() *cobra.Command {
	swapCmd := &cobra.Command{
		Use:   "swap",
		Short: "allows to execute swap action at the specified cluster or RG",
		Example: `
For multi-cluster config:
./repctl --rg <rg-id> swap --at <cluster-id> --wait
For single cluster config:
./repctl --rg <rg-id> swap`,
		Long: `
This command will perform a swap at specified cluster or at the RG.
To perform a swap at a cluster, use --to-cluster <clusterID> with <rg-id> and to do failover to RG, use --to-rg <rg-id>.
repctl will patch CR at cluster1 with action SWAP_LOCAL.`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputCluster := viper.GetString("toTgt")
			verbose := viper.GetBool(config.Verbose)
			wait := viper.GetBool("swap-wait")
			input, res := verifyInputForAction(inputCluster, rgName)
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("swap: error getting clusters folder path: %s", err.Error())
			}
			if input == "cluster" {
				swapAtCluster(configFolder, res, rgName, verbose, wait)
			} else if input == "rg" {
				swapAtRG(configFolder, res, verbose, wait)
			}else {
				log.Fatal("Unexpected input received")
			}
		},
	}

	swapCmd.Flags().String("at", "", "target on which swap to execute")
	_ = viper.BindPFlag("toTgt", swapCmd.Flags().Lookup("at"))


	swapCmd.Flags().Bool("wait", false, "wait for action to complete")
	_ = viper.BindPFlag("swap-wait", swapCmd.Flags().Lookup("wait"))

	return swapCmd
}

func swapAtRG(configFolder string, rgName string, verbose bool, wait bool) {
	if verbose {
		log.Printf("fetching RG and cluster info...")
	}
	// fetch the specified RG and the cluster info
	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "")
	if err != nil {
		log.Fatalf("failover to RG: error fetching RG info: (%s)", err.Error())
	}
	if verbose {
		log.Printf("found specified RG (%s) on cluster (%s)...", rg.Name, cluster.GetID())
		log.Print("updating spec...", rg.Name)

	}
	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil{
		log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
	}
	rg.Spec.Action = config.ActionSwap
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		log.Fatalf("swap: error executing UpdateAction %s", err.Error())
	}
	if wait {
		success := waitForStateToUpdate(rgName, cluster,rLinkState)
		if success{
			log.Printf("Successfully executed action on RG (%s)\n", rg.Name)
			return
		}
		log.Printf("RG (%s), timed out with action: failover\n", rg.Name)
		return
	}
	log.Printf("RG (%s), successfully updated with action: swap", rg.Name)
}

func swapAtCluster(configFolder string, inputCluster string, rgName string, verbose bool, wait bool) {
	if verbose {
		log.Print("reading cluster configs...")
	}
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{inputCluster}, configFolder)
	if err != nil {
		log.Fatalf("swap: error in initializing cluster info: %s", err.Error())
	}
	cluster := clusters.Clusters[0]
	if verbose {
		log.Printf("found cluster (%s)", cluster.GetID())
	}
	rg, err := cluster.GetReplicationGroups(context.Background(), rgName)
	if err != nil {
		log.Fatalf("swap: error in fecthing RG info: %s", err.Error())
	}
	if verbose {
		log.Printf("found RG (%s) on cluster, updating spec...", rg.Name)
	}
	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil{
		log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
	}
	rg.Spec.Action = config.ActionSwap
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		log.Fatalf("swap: error executing UpdateAction %s", err.Error())
	}
	if wait {
		success := waitForStateToUpdate(rgName, cluster,rLinkState)
		if success{
			log.Printf("Successfully executed action on RG (%s)\n", rg.Name)
			return
		}
		log.Printf("RG (%s), timed out with action: failover\n", rg.Name)
		return
	}
	log.Printf("RG (%s), successfully updated with action: swap", rg.Name)
}
