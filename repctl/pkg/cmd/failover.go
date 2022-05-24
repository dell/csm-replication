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
	"github.com/dell/csm-replication/api/v1alpha1"
	"path"
	"time"

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
./repctl --rg <rg-id> failover --target cluster2 --wait
./repctl --rg <rg-id> failover --target cluster2 --unplanned
For single cluster config:
./repctl --rg <rg-id> failover --target <rg-id>
./repctl --rg <rg-id> failover --target <rg-id> --unplanned`,
		Long: `
This command will perform a planned failover to a cluster or to an RG.
To perform failover to a cluster, use --target <clusterID> with --rg <rg-id1> and to do failover to RG, use --target <rg-id2> with --rg <rg-id1>. repctl will patch the CR at source site with action FAILOVER_REMOTE.
With --unplanned, this command will perform an unplanned failover to given cluster or an rg. repctl will patch CR at cluster2 with action UNPLANNED_FAILOVER_LOCAL`,

		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputTargetCluster := viper.GetString("tgt")
			unplanned := viper.GetBool("unplanned")
			wait := viper.GetBool("failover-wait")
			verbose := viper.GetBool(config.Verbose)
			target := verifyInputForFailoverAction(inputTargetCluster)

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failover: error getting clusters folder path: %s\n", err.Error())
			}

			if target == "cluster" {
				failoverToCluster(configFolder, inputTargetCluster, rgName, unplanned, verbose, wait)
			} else if target == "rg" {
				failoverToRG(configFolder, inputTargetCluster, unplanned, verbose, wait)
			} else {
				log.Fatalf("Unexpected input")
			}
		},
	}

	failoverCmd.Flags().String("target", "", "target on which to execute failover")
	_ = viper.BindPFlag("tgt", failoverCmd.Flags().Lookup("target"))

	failoverCmd.Flags().Bool("unplanned", false, "flag marking failover to be unplanned")
	_ = viper.BindPFlag("unplanned", failoverCmd.Flags().Lookup("unplanned"))

	failoverCmd.Flags().Bool("wait", false, "wait for action to complete")
	_ = viper.BindPFlag("failover-wait", failoverCmd.Flags().Lookup("wait"))

	return failoverCmd
}

func verifyInputForFailoverAction(input string) string {
	// Check if cluster or rg is given by the user
	if input == "" {
		log.Fatalf("failover: wrong input, no input provided. Either clusterID or RGID is needed.\n")
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
			return "cluster"
		}
		rgList, err := cluster.ListReplicationGroups(context.Background())
		if err != nil {
			log.Printf("Encountered error during filtering persistent volume claims. Error: %s",
				err.Error())
			continue
		}
		for _, value := range rgList.Items {
			if value.Name == input {
				return "rg"
			}
		}
	}
	return ""
}

func failoverToRG(configFolder, rgName string, unplanned bool, verbose bool, wait bool) {
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
		rLinkState := rg.Status.ReplicationLinkState
		if rLinkState.LastSuccessfulUpdate == nil {
			log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
		}
		// unplanned failover, update target RG
		rg.Spec.Action = config.ActionFailoverLocalUnplanned
		if verbose {
			log.Print("found flag for unplanned failover, updating remote RG...")
		}
		if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
			log.Fatalf("failover: error executing UpdateAction %s\n", err.Error())
		}
		if wait {
			success := waitForStateToUpdate(rgName, cluster, rLinkState)
			if success {
				log.Printf("RG (%s), successfully updated with action: failover\n", rg.Name)
				return
			}
			log.Printf("RG (%s), timed out with action: failover\n", rg.Name)
			return

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
		rLinkState := rg.Status.ReplicationLinkState
		if rLinkState.LastSuccessfulUpdate == nil {
			log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
		}
		sourceRG.Spec.Action = config.ActionFailoverRemote
		if err := cluster.UpdateReplicationGroup(context.Background(), sourceRG); err != nil {
			log.Fatalf("failover: error executing UpdateAction %s\n", err.Error())
		}
		if wait {
			success := waitForStateToUpdate(rgName, cluster, rLinkState)
			if success {
				log.Printf("RG (%s), successfully updated with action: failover\n", rg.Name)
				return
			}
			log.Printf("RG (%s), timed out with action: failover\n", rg.Name)
			return

		}
		log.Printf("RG (%s), successfully updated with action: failover\n", sourceRG.Name)
	}
}

func failoverToCluster(configFolder, inputTargetCluster, rgName string, unplanned bool, verbose bool, wait bool) {
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
		rLinkState := rg.Status.ReplicationLinkState
		if rLinkState.LastSuccessfulUpdate == nil {
			log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
		}
		rg.Spec.Action = config.ActionFailoverLocalUnplanned
		if verbose {
			log.Print("found flag for unplanned failover, updating remote RG...")
		}
		if err := targetCluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
			log.Fatalf("failover: error executing UpdateAction %s\n", err.Error())
		}
		if wait {
			success := waitForStateToUpdate(rgName, targetCluster, rLinkState)
			if success {
				log.Printf("RG (%s), successfully updated with action: failover\n", rg.Name)
				return
			}
			log.Printf("RG (%s), timed out with action: failover\n", rg.Name)
			return

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
		rLinkState := rg.Status.ReplicationLinkState
		if rLinkState.LastSuccessfulUpdate == nil {
			log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
		}
		rg.Spec.Action = config.ActionFailoverRemote
		if err := sourceCluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
			log.Fatalf("failover: error executing UpdateAction %s\n", err.Error())
		}
		if wait {
			success := waitForStateToUpdate(rgName, sourceCluster, rLinkState)
			if success {
				log.Printf("RG (%s), successfully updated with action: failover\n", rg.Name)
				return
			}
			log.Printf("RG (%s), timed out with action: failover\n", rg.Name)
			return

		}
		log.Printf("RG (%s), successfully updatedd with action: failover\n", rg.Name)
	}
}

func waitForStateToUpdate(rgName string, cluster k8s.ClusterInterface, repllinkstate v1alpha1.ReplicationLinkState) bool {

	ret := make(chan bool)
	go func() {
		log.Print("Waiting for action to complete ...")
		for {
			select {
			case <-time.After(5 * time.Minute):
				ret <- false
			default:
				rg, err := cluster.GetReplicationGroups(context.Background(), rgName)
				if err != nil {
					log.Fatalf("failover: error in fecthing RG info: %s\n", err.Error())
				}
				if rg.Status.ReplicationLinkState.LastSuccessfulUpdate.Time != repllinkstate.LastSuccessfulUpdate.Time {
					ret <- true
					return
				}
				time.Sleep(5 * time.Second)
			}
		}
	}()
	res := <-ret
	return res
}
