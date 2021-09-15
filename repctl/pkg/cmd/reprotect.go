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

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

/* #nosec G104 */
func GetReprotectCommand() *cobra.Command {
	reprotectCmd := &cobra.Command{
		Use:   "reprotect",
		Short: "allows to execute reprotect action at the specified cluster or rg",
		Example: `
For multi-cluster config:
./repctl --rg <rg-id> reprotect --to-cluster cluster1
For sigle cluster config:
./repctl reprotect  --to-rg <rg-id>`,
		Long: `
This command will perform a reprotect at specified cluster or at the RG.
To perform a reprotect at a cluster, use --to-cluster <clusterID> with <rg-id> and to do failover to RG, use --to-rg <rg-id>.
repctl will patch CR at cluster1 with action REPROTECT_LOCAL.`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputCluster := viper.GetString("atTgt")
			inputRG := viper.GetString("atTgt-rg")
			verbose := viper.GetBool(config.Verbose)
			verifyInputForAction(inputRG, inputCluster)
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				fmt.Fprintf(os.Stderr, "reprotect: error getting clusters folder path: %s\n", err.Error())
				os.Exit(1)
			}
			if inputCluster != "" {
				reprotectAtCluster(configFolder, inputCluster, rgName, verbose)
			} else {
				reprotectAtRG(configFolder, inputRG, verbose)
			}
		},
	}

	reprotectCmd.Flags().String("to-cluster", "", "cluster on which reprotect to execute")
	_ = viper.BindPFlag("atTgt", reprotectCmd.Flags().Lookup("to-cluster"))

	reprotectCmd.Flags().String("to-rg", "", "RG on which reprotect to execute")
	_ = viper.BindPFlag("atTgt-rg", reprotectCmd.Flags().Lookup("to-rg"))
	return reprotectCmd
}

func reprotectAtRG(configFolder, rgName string, verbose bool) {
	if verbose {
		fmt.Printf("fetching RG and cluster info...\n")
	}
	// fetch the specified RG and the cluster info
	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "reprotect to RG: error fetching RG info: (%s)\n", err.Error())
		os.Exit(1)
	}
	if verbose {
		fmt.Printf("found specified RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
		fmt.Println("updating spec...", rg.Name)

	}
	rg.Spec.Action = config.ActionReprotect
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		fmt.Fprintf(os.Stderr, "reprotect: error executing UpdateAction %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("RG (%s), successfully updated with action: reprotect\n", rg.Name)
}

func reprotectAtCluster(configFolder, inputCluster, rgName string, verbose bool) {
	if verbose {
		fmt.Println("reading cluster configs...")
	}
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{inputCluster}, configFolder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reprotect: error in initializing cluster info: %s\n", err.Error())
		os.Exit(1)
	}
	cluster := clusters.Clusters[0]
	if verbose {
		fmt.Printf("found cluster (%s)\n", cluster.GetID())
	}
	rg, err := cluster.GetReplicationGroups(context.Background(), rgName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reprotect: error in fecthing RG info: %s\n", err.Error())
		os.Exit(1)
	}
	if verbose {
		fmt.Printf("found RG (%s) on cluster, updating spec...\n", rg.Name)
	}
	rg.Spec.Action = config.ActionReprotect
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		fmt.Fprintf(os.Stderr, "reprotect: error executing UpdateAction %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("RG (%s), successfully updated with action: reprotect\n", rg.Name)
}
