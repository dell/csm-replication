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

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/metadata"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			createPVCs := viper.GetBool("create-pvcs")
			snClass := viper.GetString("sn-class")
			verbose := viper.GetBool(config.Verbose)
			wait := viper.GetBool("snapshot-wait")
			input, res := verifyInputForSnapshotAction(inputCluster, rgName)

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("snapshot: error getting clusters folder path: %s\n", err.Error())
			}

			if input == "rg" {
				createSnapshot(configFolder, res, prefix, snNamespace, snClass, verbose, wait, createPVCs)
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

func createSnapshot(configFolder, rgName, prefix, snNamespace, snClass string, verbose, wait, createPVCs bool) {
	if verbose {
		log.Printf("fetching RG and cluster info...\n")
	}

	cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "src")
	if err != nil {
		log.Fatalf("snapshot to RG: error fetching RG info: (%s)\n", err.Error())
		return
	}

	// Check if snNamespace is specified and different from the original namespace
	if snNamespace == "" || snNamespace == rg.Namespace {
		log.Fatal("Error: --sn-namespace must be specified and different from the original application's namespace")
	}

	// Use default volumesnapshotclass if not specified
	if snClass == "" {
		snClass = getDefaultSnapshotClass(rg)
	}

	if verbose {
		log.Printf("found specified RG (%s) on cluster (%s)...\n", rg.Name, cluster.GetID())
		log.Print("updating spec...", rg.Name)
	}

	rLinkState := rg.Status.ReplicationLinkState
	if rLinkState.LastSuccessfulUpdate == nil {
		log.Fatal("Aborted. One of your RGs is in an error state. Please verify RGs logs/events and try again.")
		return
	}

	rg.Spec.Action = config.ActionCreateSnapshot
	rg.Annotations[prefix+"/snapshotNamespace"] = snNamespace
	rg.Annotations[prefix+"/snapshotClass"] = snClass

	if err := cluster.UpdateReplicationGroup(context.Background(), rg); err != nil {
		log.Fatalf("snapshot: error executing UpdateAction %s\n", err.Error())
		return
	}

	if wait {
		success := waitForStateToUpdate(rgName, cluster, rLinkState)
		if !success {
			log.Printf("RG (%s), timed out with action: snapshot\n", rg.Name)
			return
		}
	}

	log.Printf("Successfully created snapshots for RG (%s)\n", rg.Name)

	if createPVCs {
		if err := createPVCsFromSnapshots(cluster, rg, snClass); err != nil {
			log.Fatalf("Error creating PVCs from snapshots: %v", err)
		}
		log.Printf("Successfully created PVCs from snapshots in namespace test-pg1")
	}
	// if createPVCs {
	// 	if err := createClonedSnapshotsAndPVCs(cluster, rg, snNamespace, snClass); err != nil {
	// 		log.Fatalf("Error creating cloned snapshots and PVCs: %v", err)
	// 	}
	// 	log.Printf("Successfully created cloned snapshots and PVCs in namespace %s", snNamespace)
	// }
}

func getDefaultSnapshotClass(rg *repv1.DellCSIReplicationGroup) string {
	driver, ok := rg.Labels[metadata.Driver]
	if !ok {
		driver, ok = rg.Annotations[metadata.Driver]
	}
	if !ok {
		return "default-snapclass"
	}

	switch driver {
	case "csi-powerstore":
		return "powerstore-snapclass"
	case "csi-powerflex":
		return "powerflex-snapclass"
	default:
		return "default-snapclass"
	}
}

// func createPVCsFromSnapshots(cluster k8s.ClusterInterface, rg *repv1.DellCSIReplicationGroup, newNamespace, snapshotClass string) error {
// 	ctx := context.Background()

// 	// Use the namespace from the ReplicationGroup for the original PVCs
// 	originalNamespace := rg.Namespace

// 	pvcList, err := cluster.FilterPersistentVolumeClaims(ctx, originalNamespace, "", "", rg.Name)
// 	if err != nil {
// 		return fmt.Errorf("error getting PVCs: %v", err)
// 	}

// 	for _, pvc := range pvcList.PVCList {
// 		origPVC, err := cluster.GetPersistentVolumeClaim(ctx, originalNamespace, pvc.Name)
// 		if err != nil {
// 			return fmt.Errorf("error getting original PVC %s in namespace %s: %v", pvc.Name, originalNamespace, err)
// 		}

// 		snapshotName := fmt.Sprintf("%s-snapshot", pvc.Name)

// 		newPVC := &v1.PersistentVolumeClaim{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      pvc.Name,
// 				Namespace: newNamespace,
// 			},
// 			Spec: v1.PersistentVolumeClaimSpec{
// 				StorageClassName: origPVC.Spec.StorageClassName,
// 				AccessModes:      origPVC.Spec.AccessModes,
// 				Resources:        origPVC.Spec.Resources,
// 				DataSource: &v1.TypedLocalObjectReference{
// 					APIGroup: pointer.String("snapshot.storage.k8s.io"),
// 					Kind:     "VolumeSnapshot",
// 					Name:     snapshotName,
// 				},
// 			},
// 		}

// 		err = cluster.GetClient().Create(ctx, newPVC)
// 		if err != nil {
// 			return fmt.Errorf("error creating PVC %s in namespace %s: %v", newPVC.Name, newNamespace, err)
// 		}

// 		log.Printf("Created PVC %s in namespace %s from snapshot", newPVC.Name, newNamespace)
// 	}

// 	return nil
// }

func createPVCsFromSnapshots(cluster k8s.ClusterInterface, rg *repv1.DellCSIReplicationGroup, snapshotClass string) error {
	ctx := context.Background()
	newNamespace := "test-pg1" // Hard-coded namespace

	// Use the namespace from the ReplicationGroup for the original PVCs
	originalNamespace := rg.Namespace

	pvcList, err := cluster.FilterPersistentVolumeClaims(ctx, originalNamespace, "", "", rg.Name)
	if err != nil {
		return fmt.Errorf("error getting PVCs: %v", err)
	}

	for _, pvc := range pvcList.PVCList {
		origPVC, err := cluster.GetPersistentVolumeClaim(ctx, originalNamespace, pvc.Name)
		if err != nil {
			return fmt.Errorf("error getting original PVC %s in namespace %s: %v", pvc.Name, originalNamespace, err)
		}

		// Step 1: Create cloned SnapshotContent
		clonedSnapshotContentName := fmt.Sprintf("cloned-volume-%s", pvc.Name)
		clonedSnapshotContent := &snapshotv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{
				Name: clonedSnapshotContentName,
			},
			Spec: snapshotv1.VolumeSnapshotContentSpec{
				DeletionPolicy: snapshotv1.VolumeSnapshotContentDelete,
				Driver:         "csi-vxflexos.dellemc.com",
				Source: snapshotv1.VolumeSnapshotContentSource{
					SnapshotHandle: pointer.String("snapshot-handle"), // This should be retrieved from the original snapshot
				},
				VolumeSnapshotRef: v1.ObjectReference{
					Namespace: newNamespace,
					Name:      fmt.Sprintf("snapshot-%s", pvc.Name),
				},
				VolumeSnapshotClassName: &snapshotClass,
			},
		}

		err = cluster.GetClient().Create(ctx, clonedSnapshotContent)
		if err != nil {
			return fmt.Errorf("error creating cloned SnapshotContent %s: %v", clonedSnapshotContentName, err)
		}

		// Step 2: Create new Snapshot
		newSnapshotName := fmt.Sprintf("snapshot-%s", pvc.Name)
		newSnapshot := &snapshotv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      newSnapshotName,
				Namespace: newNamespace,
			},
			Spec: snapshotv1.VolumeSnapshotSpec{
				Source: snapshotv1.VolumeSnapshotSource{
					VolumeSnapshotContentName: &clonedSnapshotContentName,
				},
				VolumeSnapshotClassName: &snapshotClass,
			},
		}

		err = cluster.GetClient().Create(ctx, newSnapshot)
		if err != nil {
			return fmt.Errorf("error creating new Snapshot %s in namespace %s: %v", newSnapshotName, newNamespace, err)
		}

		// Step 3: Create PVC from Snapshot
		newPVC := &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvc.Name,
				Namespace: newNamespace,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				StorageClassName: origPVC.Spec.StorageClassName,
				AccessModes:      origPVC.Spec.AccessModes,
				Resources:        origPVC.Spec.Resources,
				DataSource: &v1.TypedLocalObjectReference{
					APIGroup: pointer.String("snapshot.storage.k8s.io"),
					Kind:     "VolumeSnapshot",
					Name:     newSnapshotName,
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
