/*
 Copyright © 2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"time"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	repv1 "github.com/dell/csm-replication/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"

)

// GetSnapshotCommand returns 'snapshot' cobra command
/* #nosec G104 */
func GetSnapshotCommand() *cobra.Command {
	snapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "allows to execute snapshot action at the specified cluster or rg",
		Example: `
For single cluster config:
./repctl --rg <rg-id> --sn-namespace <namespace> --sn-class <snapshot class> snapshot`,
		Long: `
This command will create a snapshot for the specified RG on the target cluster.\n`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			inputCluster := viper.GetString("target")
			prefix := viper.GetString(config.ReplicationPrefix)
			snNamespace := viper.GetString("sn-namespace")
			createPVCtrue := viper.GetBool("create-pvcs")
			snClass := viper.GetString("sn-class")
			verbose := viper.GetBool(config.Verbose)
			wait := viper.GetBool("snapshot-wait")
			input, res := verifyInputForSnapshotAction(inputCluster, rgName)

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("snapshot: error getting clusters folder path: %s\n", err.Error())
			}

			if input == "rg" {
				createSnapshot(configFolder, res, prefix, snNamespace, snClass, verbose, wait, createPVCtrue)
			} else {
				log.Fatal("Unexpected input received")
			}
		},
	}

	snapshotCmd.Flags().String("at", "", "target to execute the snapshot")
	_ = viper.BindPFlag("target", snapshotCmd.Flags().Lookup("at"))
	snapshotCmd.Flags().String("sn-namespace", "", "namespace to create the snapshots in")
	_ = viper.BindPFlag("sn-namespace", snapshotCmd.Flags().Lookup("sn-namespace"))
	snapshotCmd.Flags().String("sn-class", "", "target snapshot class to use")
	_ = viper.BindPFlag("sn-class", snapshotCmd.Flags().Lookup("sn-class"))

	snapshotCmd.Flags().Bool("wait", false, "wait for action to complete")
	_ = viper.BindPFlag("snapshot-wait", snapshotCmd.Flags().Lookup("wait"))
	snapshotCmd.Flags().Bool("create-pvcs", false, "create PVCs from snapshots")
	_ = viper.BindPFlag("create-pvcs", snapshotCmd.Flags().Lookup("create-pvcs"))
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
			log.Printf("Encountered error during filtering preplication groups. Error: %s",
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

func createSnapshot(configFolder, rgName, prefix, snNamespace, snClass string, verbose, wait, createPVCtrue bool) {
	if verbose {
		log.Printf("fetching RG and cluster info...\n")
	}

	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "src")
	if err != nil {
		log.Fatalf("snapshot to RG: error fetching RG info: (%s)\n", err.Error())
		return
	}

	if verbose {
		log.Printf("found specified RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
		log.Print("updating spec...", rg.Name)

	}



	testing := rg.Annotations[prefix+"/snapshotNamespace"]
	if testing != "testing-rn" {

	}

	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil {
		log.Fatal("Aborted. One of your RGs is in an error state. Please verify RGs logs/events and try again.")
		return
	}

	if snClass == "" {
		log.Fatal("Aborted. Snapshot class not provided.")
		return
	}

	rg.Spec.Action = config.ActionCreateSnapshot

	namespace := "default"
	if snNamespace != "" {
		namespace = snNamespace
	}

	log.Printf("Executing CreateSnapshot on Namespace: %s, Snapshot Class: %s", namespace, snClass)

	rg.Annotations[prefix+"/snapshotNamespace"] = namespace
	rg.Annotations[prefix+"/snapshotClass"] = snClass

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

		log.Printf("RG (%s), timed out with action: snapshot\n", rg.Name)
		return
	}

	log.Printf("RG (%s), successfully updated with action: snapshot\n", rg.Name)

	//todo: wait for it to finish -> confirm that snapshots are created
	time.Sleep(5 * time.Second)


	if createPVCtrue {
		if err := createPVCsFromSnapshots(cluster, rg, snNamespace, snClass); err != nil {
			log.Fatalf("Error creating PVCs from snapshots: %v", err)
		}
		log.Printf("Successfully created PVCs from snapshots in namespace test-pg1")
	}
}



func createPVCsFromSnapshots(cluster k8s.ClusterInterface, rg *repv1.DellCSIReplicationGroup, snNamespace, snClass string) error {
	ctx := context.Background()
	newNamespace := "test-pg1" // hard coded for now

	// retrieve snapshots under the user defined namespace
	snList, err := cluster.ListVolumeSnapshots(ctx, client.InNamespace(snNamespace))
	if err != nil {
		return fmt.Errorf("error getting Snapshots: %v", err)
	}

	log.Printf("Found %d snapshots", len(snList.Items))
	for _, sn := range snList.Items {
		volumeName := *sn.Status.BoundVolumeSnapshotContentName
		log.Printf(volumeName)

		// step 1: create cloned snapshot content -> done
		clonedSnapshotContentName := fmt.Sprintf("cloned-%s", volumeName)
		snContent, _ := cluster.GetVolumeSnapshotContent(ctx, volumeName)

		clonedSnapshotContent := &s1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{
				Name: clonedSnapshotContentName,
			},
			Spec: s1.VolumeSnapshotContentSpec{
				DeletionPolicy: snContent.Spec.DeletionPolicy,
				Driver:         snContent.Spec.Driver,
				Source: s1.VolumeSnapshotContentSource{
					SnapshotHandle: snContent.Spec.Source.SnapshotHandle,
				},
				VolumeSnapshotRef: v1.ObjectReference{
					Namespace: newNamespace,
					Name:      sn.Name,
				},
				VolumeSnapshotClassName: &snClass,
			},
		}

		err = cluster.GetClient().Create(ctx, clonedSnapshotContent)
		if err != nil {
			return fmt.Errorf("error creating cloned SnapshotContent %s: %v", clonedSnapshotContentName, err)
		}

		// step 2: create new snapshot -> done
		newSnapshot := &s1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sn.Name,
				Namespace: newNamespace,
			},
			Spec: s1.VolumeSnapshotSpec{
				Source: s1.VolumeSnapshotSource{
					VolumeSnapshotContentName: &clonedSnapshotContentName,
				},
				VolumeSnapshotClassName: &snClass,
			},
		}

		err = cluster.GetClient().Create(ctx, newSnapshot)
		if err != nil {
			return fmt.Errorf("error creating new Snapshot %s in namespace %s: %v", sn.Name, newNamespace, err)
		}

		// step 3: create pvc from snapshot -> done
		pvcName := *sn.Spec.Source.PersistentVolumeClaimName //panic
		newPVC := &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: newNamespace,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				StorageClassName: &snClass,
				AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}, // hard coded
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("32Gi"), // hard coded
					},
				},
				DataSource: &v1.TypedLocalObjectReference{
					APIGroup: pointer.String("snapshot.storage.k8s.io"),
					Kind:     "VolumeSnapshot",
					Name:     sn.Name,
				},
			},
		}

		err = cluster.GetClient().Create(ctx, newPVC)
		if err != nil {
			return fmt.Errorf("error creating PVC %s in namespace %s: %v", newPVC.Name, newNamespace, err)
		}

		log.Printf("Created PVC %s in namespace %s from snapshot", newPVC.Name, newNamespace)
	}

	return nil
}
