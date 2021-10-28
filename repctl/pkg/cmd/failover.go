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
	"path"

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/spf13/cobra"
)

// GetFailoverCommand returns 'failover' cobra command
/* #nosec G104 */
func GetFailoverCommand() *cobra.Command {
	failoverCmd := &cobra.Command{
		Use:   "failover",
		Short: "allows to execute failover action from source cluster/rg to target cluster/rg",
		Example: `
For multi-cluster config:
./repctl --rg <rg-id> failover --to-cluster cluster2	
./repctl --rg <rg-id> failover --to-cluster cluster2 --unplanned
For single cluster config:
./repctl failover --to-rg <rg-id>
./repctl failover --to-rg <rg-id> --unplanned`,
		Long: `
This command will perform a planned failover to a cluster or to an RG.
To perform failover to a cluster, use --to-cluster <clusterID> with <rg-id> and to do failover to RG, use --to-rg <rg-id>. repctl will patch the CR at source site with action FAILOVER_REMOTE.
With --unplanned, this command will perform an unplanned failover to given cluster or an rg. repctl will patch CR at cluster2 with action UNPLANNED_FAILOVER_LOCAL`,

		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputTargetCluster := viper.GetString("tgt")
			inputTargetRG := viper.GetString("tgt-rg")
			unplanned := viper.GetBool("unplanned")
			verbose := viper.GetBool(config.Verbose)
			verifyInputForAction(inputTargetRG, inputTargetCluster)

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failover: error getting clusters folder path: %s\n", err.Error())
			}

			if inputTargetCluster != "" {
				failoverToCluster(configFolder, inputTargetCluster, rgName, unplanned, verbose)
			} else {
				failoverToRG(configFolder, inputTargetRG, unplanned, verbose)
			}
		},
	}

	failoverCmd.Flags().String("to-cluster", "", "cluster to which execute failover")
	_ = viper.BindPFlag("tgt", failoverCmd.Flags().Lookup("to-cluster"))

	failoverCmd.Flags().String("to-rg", "", "RG to which execute failover")
	_ = viper.BindPFlag("tgt-rg", failoverCmd.Flags().Lookup("to-rg"))

	failoverCmd.Flags().Bool("unplanned", false, "flag marking failover to be unplanned")
	_ = viper.BindPFlag("unplanned", failoverCmd.Flags().Lookup("unplanned"))

	return failoverCmd
}

func verifyInputForAction(rg string, clusterID string) {
	// Check if cluster or rg is given by the user
	if len(rg) < 1 && len(clusterID) < 1 {
		log.Fatalf("failover: wrong input, no input provided. Either clusterID or RGID is needed.\n")
	}

	// Check if both cluster and rg is given by the user
	if len(rg) > 0 && len(clusterID) > 0 {
		log.Fatalf("failover: wrong input, can not failover to both cluster and RG.\n")
	}
}

func failoverToRG(configFolder, rgName string, unplanned, verbose bool) {
	if verbose {
		log.Printf("fetching RG and cluster info...\n")
	}
	// fetch the target RG and the cluster info
	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "tgt")
	if err != nil {
		log.Fatalf("failover to RG: error fetching target RG info: (%s)\n", err.Error())
	}
	if verbose {
		log.Printf("found target RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
	}
	// check if the action is unplanned failover
	if unplanned {
		// unplanned failover, update target RG
		rg.Spec.Action = config.ActionFailoverLocalUnplanned
		if verbose {
			log.Print("found flag for unplanned failover, updating remote RG...")
		}
		if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
			log.Fatalf("failover: error executing UpdateAction %s\n", err.Error())
		}
		log.Printf("RG (%s), successfully updated with action: unplanned failover\n", rg.Name)
	} else {
		// proceed for planned failover
		sourceRGID := rg.GetAnnotations()[path.Join(viper.GetString(config.ReplicationPrefix), "remoteReplicationGroupName")]
		if sourceRGID == "" {
			log.Fatalf("failover: error in fecthing source RG name: %s\n", err.Error())
		}
		if verbose {
			log.Printf("source RG (%s) ...\n", sourceRGID)
		}
		sourceRG, err := cluster.GetReplicationGroups(context.Background(), sourceRGID)
		if err != nil {
			log.Fatalf("failover: error in fecthing RG info: %s\n", err.Error())
		}
		if verbose {
			log.Printf("found RG (%s) on source cluster, updating spec...\n", rg.Name)
		}
		sourceRG.Spec.Action = config.ActionFailoverRemote
		if err := cluster.UpdateReplicationGroup(context.Background(), sourceRG); err != nil {
			log.Fatalf("failover: error executing UpdateAction %s\n", err.Error())
		}
		log.Printf("RG (%s), successfully updated with action: failover\n", sourceRG.Name)
	}
}

func failoverToCluster(configFolder, inputTargetCluster, rgName string, unplanned, verbose bool) {
	if verbose {
		log.Print("reading cluster configs...")
	}
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters([]string{inputTargetCluster}, configFolder)
	if err != nil {
		log.Fatalf("failover: error in initializing cluster info: %s\n", err.Error())
	}
	targetCluster := clusters.Clusters[0]
	if verbose {
		log.Printf("found target cluster (%s)\n", targetCluster.GetID())
	}
	rg, err := targetCluster.GetReplicationGroups(context.Background(), rgName)
	if err != nil {
		log.Fatalf("failover: error in fecthing RG info: %s\n", err.Error())
	}
	if verbose {
		log.Printf("found RG (%s) on target cluster...\n", rg.Name)
	}
	if rg.Status.ReplicationLinkState.IsSource {
		log.Fatalf("failover: error executing failover to source site.")
	}
	// check if this is an unplanned failover
	if unplanned {
		rg.Spec.Action = config.ActionFailoverLocalUnplanned
		if verbose {
			log.Print("found flag for unplanned failover, updating remote RG...")
		}
		if err := targetCluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
			log.Fatalf("failover: error executing UpdateAction %s\n", err.Error())
		}
		log.Printf("RG (%s), successfully updated with action: unplanned failover\n", rg.Name)
	} else {
		// proceed for planned failover
		if verbose {
			log.Print("fetching source cluster...")
		}
		// fetch CR on source cluster (remote to this target  cluster)
		clusters, err = mc.GetAllClusters([]string{rg.Spec.RemoteClusterID}, configFolder)
		if err != nil {
			log.Fatalf("failover: error in fetching source cluster info: %s\n", err.Error())
		}
		sourceCluster := clusters.Clusters[0]
		if verbose {
			log.Printf("found source cluster (%s)\n", sourceCluster.GetID())
		}
		rg, err = sourceCluster.GetReplicationGroups(context.Background(), rgName)
		if err != nil {
			log.Fatalf("failover: error in fecthing RG info: %s\n", err.Error())
		}
		if verbose {
			log.Printf("found RG (%s) on source cluster, updating spec...\n", rg.Name)
		}
		rg.Spec.Action = config.ActionFailoverRemote
		if err := sourceCluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
			log.Fatalf("failover: error executing UpdateAction %s\n", err.Error())
		}
		log.Printf("RG (%s), successfully updated with action: failover\n", rg.Name)
	}
}
