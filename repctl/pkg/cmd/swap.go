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

	"github.com/dell/repctl/pkg/k8s"

	"github.com/dell/repctl/pkg/config"
	"github.com/spf13/viper"

	"github.com/spf13/cobra"
)

/* #nosec G104 */
func GetSwapCommand() *cobra.Command {
	swapCmd := &cobra.Command{
		Use:   "swap",
		Short: "allows to execute swap action at the specified cluster or RG",
		Example: `
For multi-cluster config:
./repctl --rg <rg-id> swap --to-cluster cluster1
For single cluster config:
./repctl swap --to-rg <rg-id>`,
		Long: `
This command will perform a swap at specified cluster or at the RG.
To perform a swap at a cluster, use --to-cluster <clusterID> with <rg-id> and to do failover to RG, use --to-rg <rg-id>.
repctl will patch CR at cluster1 with action SWAP_LOCAL.`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputCluster := viper.GetString("toTgt")
			inputRG := viper.GetString("toTgt-rg")
			verbose := viper.GetBool(config.Verbose)
			verifyInputForAction(inputRG, inputCluster)
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				fmt.Fprintf(os.Stderr, "swap: error getting clusters folder path: %s\n", err.Error())
				os.Exit(1)
			}
			if inputCluster != "" {
				swapAtCluster(configFolder, inputCluster, rgName, verbose)
			} else {
				swapAtRG(configFolder, inputRG, verbose)
			}
		},
	}

	swapCmd.Flags().String("to-cluster", "", "cluster on which swap to execute")
	_ = viper.BindPFlag("toTgt", swapCmd.Flags().Lookup("to-cluster"))

	swapCmd.Flags().String("to-rg", "", "RG on which swap to execute")
	_ = viper.BindPFlag("toTgt-rg", swapCmd.Flags().Lookup("to-rg"))

	return swapCmd
}

func swapAtRG(configFolder string, rgName string, verbose bool) {
	if verbose {
		fmt.Printf("fetching RG and cluster info...\n")
	}
	// fetch the specified RG and the cluster info
	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failover to RG: error fetching RG info: (%s)\n", err.Error())
		os.Exit(1)
	}
	if verbose {
		fmt.Printf("found specified RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
		fmt.Println("updating spec...", rg.Name)

	}
	rg.Spec.Action = config.ActionSwap
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		fmt.Fprintf(os.Stderr, "swap: error executing UpdateAction %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("RG (%s), successfully updated with action: swap\n", rg.Name)
}

func swapAtCluster(configFolder string, inputCluster string, rgName string, verbose bool) {
	if verbose {
		fmt.Println("reading cluster configs...")
	}
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{inputCluster}, configFolder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "swap: error in initializing cluster info: %s\n", err.Error())
		os.Exit(1)
	}
	cluster := clusters.Clusters[0]
	if verbose {
		fmt.Printf("found cluster (%s)\n", cluster.GetID())
	}
	rg, err := cluster.GetReplicationGroups(context.Background(), rgName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "swap: error in fecthing RG info: %s\n", err.Error())
		os.Exit(1)
	}
	if verbose {
		fmt.Printf("found RG (%s) on cluster, updating spec...\n", rg.Name)
	}
	rg.Spec.Action = config.ActionSwap
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		fmt.Fprintf(os.Stderr, "swap: error executing UpdateAction %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("RG (%s), successfully updated with action: swap\n", rg.Name)
}
