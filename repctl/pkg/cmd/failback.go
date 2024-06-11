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

	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/pvcremap"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetFailbackCommand returns 'failback' cobra command
func GetFailbackCommand() *cobra.Command {
	failbackCmd := &cobra.Command{
		Use:   "failback",
		Short: "allows to execute failback action from target cluster/rg to source cluster/rg",
		Example: `
For multi-cluster config:
./repctl --rg <rg-id> failback --target cluster1
./repctl --rg <rg-id> failback --target cluster1 --discard
For single cluster config:
./repctl --rg <rg-id> failback --target <rg-id>
./repctl --rg <rg-id> failback --target <rg-id> --discard
`,
		Long: `
This command will perform a planned failback to a cluster or to an RG.
To perform failback to a cluster, use --target <clusterID> with --rg <rg-id1> and to do failback to RG, use --target <rg-id2> with --rg <rg-id1>. repctl will patch the CR at source site with action FAILBACK_LOCAL.
With --discard, this command will perform a failback but discard any writes at target. repctl will patch the CR at source site with action ACTION_FAILBACK_DISCARD_CHANGES_LOCAL`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputSourceCluster := viper.GetString("src")
			discard := viper.GetBool("discard")
			verbose := viper.GetBool(config.Verbose)
			wait := viper.GetBool("failback-wait")
			input := verifyInputForFailoverAction(inputSourceCluster)

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failback: error getting clusters folder path: %s\n", err.Error())
			}

			if input == "cluster" {
				failbackToCluster(configFolder, inputSourceCluster, rgName, discard, verbose, wait)
			} else if input == "rg" {
				failbackToRG(configFolder, inputSourceCluster, rgName, discard, verbose, wait)
			} else {
				log.Fatal("Unexpected input received")
			}
		},
	}

	failbackCmd.Flags().String("target", "", "target to which execute failback")
	_ = viper.BindPFlag("src", failbackCmd.Flags().Lookup("target"))

	failbackCmd.Flags().Bool("discard", false, "flag marking failback to discard any writes at target")
	_ = viper.BindPFlag("discard", failbackCmd.Flags().Lookup("discard"))

	failbackCmd.Flags().Bool("wait", false, "wait for action to complete")
	_ = viper.BindPFlag("failback-wait", failbackCmd.Flags().Lookup("wait"))

	failbackCmd.Flags().Bool("no-pvcremap", false, "disable PVC remapping")
	_ = viper.BindPFlag("no-pvcremap", failbackCmd.Flags().Lookup("no-pvcremap"))

	return failbackCmd
}

func failbackToRG(configFolder, inputSourceCluster, rgName string, discard, verbose bool, wait bool) {
	if verbose {
		log.Printf("fetching RG and cluster info...\n")
	}
	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "src")
	if err != nil {
		log.Fatalf("failback to RG: error fetching source RG info: (%s)\n", err.Error())
	}
	if verbose {
		log.Printf("found RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
	}

	if rg.Status.Action == config.ActionFailbackLocal && !discard {
		log.Print("Failback action is already set to planned failback.")
		return
	} else if rg.Status.Action == config.ActionFailbackLocalDiscard && discard {
		log.Print("Failback action is already set to discard failback.")
		return
	}
	if len(rg.Spec.RemoteClusters) > 1 {
		log.Fatalf("failback: PVC remapping is not allowed for replication groups associated with multiple clusters.")
	}

    // failbackToRG function
    if !viper.GetBool("no-pvcremap") {
        if err := pvcremap.RemapPVCs(context.Background(), cluster.Clientset(), rgName); err != nil {
            log.Fatalf("failback: error remapping PVCs: %s\n", err.Error())
        }
    } else {
        log.Print("PVC remapping is disabled.")
    }

	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil {
		log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
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
	if wait {
		success := waitForStateToUpdate(rgName, cluster, rLinkState)
		if success {
			log.Printf("Successfully executed action on RG (%s)\n", rg.Name)
			return
		}
		log.Printf("RG (%s), timed out with action: failback\n", rg.Name)
		return
	}
	log.Printf("RG (%s), successfully updated with action: failback\n", rg.Name)
}

func failbackToCluster(configFolder, inputSourceCluster, rgName string, discard, verbose bool, wait bool) {
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
		log.Fatalf("failback: error fetching RG info: %s\n", err.Error())
	}
	if verbose {
		log.Printf("found RG (%s) on cluster (%s)...\n", rg.Name, sourceCluster.GetID())
	}

	if rg.Status.Action == config.ActionFailbackLocal && !discard {
		log.Print("Failback action is already set to planned failback.")
		return
	} else if rg.Status.Action == config.ActionFailbackLocalDiscard && discard {
		log.Print("Failback action is already set to discard failback.")
		return
	}
	if len(rg.Spec.RemoteClusters) > 1 {
		log.Fatalf("failback: PVC remapping is not allowed for replication groups associated with multiple clusters.")
	}

    // failbackToCluster function
    if !viper.GetBool("no-pvcremap") {
        if err := pvcremap.RemapPVCs(context.Background(), clusters.Clusters[0].Clientset(), rgName); err != nil {
            log.Fatalf("failback: error remapping PVCs: %s\n", err.Error())
        }
    } else {
        log.Print("PVC remapping is disabled.")
    }

	if !rg.Status.ReplicationLinkState.IsSource {
		log.Fatalf("failback: error executing failback to target site.")
	}

	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil {
		log.Fatal("Aborted. One of your RGs is in error state. Please verify RGs logs/events and try again.")
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
	if wait {
		success := waitForStateToUpdate(rgName, sourceCluster, rLinkState)
		if success {
			log.Printf("Successfully executed action on RG (%s)\n", rg.Name)
			return
		}
		log.Printf("RG (%s), timed out with action: failback\n", rg.Name)
		return
	}
	log.Printf("RG (%s), successfully updated with action: failback\n", rg.Name)
}
