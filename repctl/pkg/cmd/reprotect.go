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

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetReprotectCommand returns 'reprotect' cobra command
/* #nosec G104 */
func GetReprotectCommand() *cobra.Command {
	reprotectCmd := &cobra.Command{
		Use:   "reprotect",
		Short: "allows to execute reprotect action at the specified cluster or rg",
		Example: `
For multi-cluster config:
./repctl --rg <rg-id> reprotect --at <cluster-id>
For single cluster config:
./repctl --rg <rg-id> reprotect`,
		Long: `
This command will perform a reprotect at specified cluster or at the RG.
To perform a reprotect at a cluster, use --at <clusterID> with --rg <rg-id> and to do reprotect at RG, use --rg <rg-id>.
repctl will patch CR at cluster1 with action REPROTECT_LOCAL.`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputCluster := viper.GetString("atTgt")
			verbose := viper.GetBool(config.Verbose)
			wait := viper.GetBool("reprotect-wait")
			input, res := verifyInputForAction(inputCluster, rgName)
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("reprotect: error getting clusters folder path: %s\n", err.Error())
			}
			if input == "cluster" {
				reprotectAtCluster(configFolder, res, rgName, verbose, wait)
			} else if input == "rg" {
				reprotectAtRG(configFolder, res, verbose, wait)
			} else {
				log.Fatal("Unexpected input received")
			}
		},
	}

	reprotectCmd.Flags().String("at", "", "target on which reprotect to execute")
	_ = viper.BindPFlag("atTgt", reprotectCmd.Flags().Lookup("at"))

	reprotectCmd.Flags().Bool("wait", false, "wait for action to complete")
	_ = viper.BindPFlag("reprotect-wait", reprotectCmd.Flags().Lookup("wait"))
	return reprotectCmd
}

func verifyInputForAction(input string, rg string) (res string, tgt string) {
	// Check if cluster or rg is given by the user
	if input == "" {
		if rg != "" {
			input = rg
		} else {
			log.Fatalf("failover: wrong input, no input provided. Either clusterID or RGID is needed.\n")
		}
	}

	configFolder, err := getClustersFolderPath("/.repctl/clusters/")
	if err != nil {
		log.Fatalf("list pvc: error getting clusters folder path: %s", err.Error())
	}

	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{}, configFolder)
	if err != nil {
		log.Fatalf("error in initializing cluster info: %s", err.Error())
	}

	for _, cluster := range clusters.Clusters {
		if input == cluster.GetID() {
			return "cluster", input
		}
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

func reprotectAtRG(configFolder, rgName string, verbose bool, wait bool) {
	if verbose {
		log.Printf("fetching RG and cluster info...\n")
	}
	// fetch the specified RG and the cluster info
	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "")
	if err != nil {
		log.Fatalf("reprotect to RG: error fetching RG info: (%s)\n", err.Error())
	}
	if verbose {
		log.Printf("found specified RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
		log.Print("updating spec...", rg.Name)

	}
	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil {
		log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
	}
	rg.Spec.Action = config.ActionReprotect
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		log.Fatalf("reprotect: error executing UpdateAction %s\n", err.Error())
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
	log.Printf("RG (%s), successfully updated with action: reprotect\n", rg.Name)
}

func reprotectAtCluster(configFolder, inputCluster, rgName string, verbose bool, wait bool) {
	if verbose {
		log.Print("reading cluster configs...")
	}
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{inputCluster}, configFolder)
	if err != nil {
		log.Fatalf("reprotect: error in initializing cluster info: %s\n", err.Error())
	}
	cluster := clusters.Clusters[0]
	if verbose {
		log.Printf("found cluster (%s)\n", cluster.GetID())
	}
	rg, err := cluster.GetReplicationGroups(context.Background(), rgName)
	if err != nil {
		log.Fatalf("reprotect: error in fecthing RG info: %s\n", err.Error())
	}
	if verbose {
		log.Printf("found RG (%s) on cluster, updating spec...\n", rg.Name)
	}
	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil {
		log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
	}
	rg.Spec.Action = config.ActionReprotect
	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		log.Fatalf("reprotect: error executing UpdateAction %s\n", err.Error())
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
	log.Printf("RG (%s), successfully updated with action: reprotect\n", rg.Name)
}
