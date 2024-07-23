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

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//"sigs.k8s.io/controller-runtime/pkg/client"
	s1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"

	//"k8s.io/apimachinery/pkg/api/resource"
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
			storageClass := viper.GetString("storage-class")
			wait := viper.GetBool("snapshot-wait")
			input, res := verifyInputForSnapshotAction(inputCluster, rgName)

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("snapshot: error getting clusters folder path: %s\n", err.Error())
			}

			if input == "rg" {
				createSnapshot(configFolder, res, prefix, snNamespace, snClass, storageClass, verbose, wait, createPVCtrue)
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

	snapshotCmd.Flags().String("storage-class", "", "storage class to use when creating PVCs from snapshots")
	_ = viper.BindPFlag("storage-class", snapshotCmd.Flags().Lookup("storage-class"))
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

func createSnapshot(configFolder, rgName, prefix, snNamespace, snClass, storageClass string, verbose, wait, createPVCtrue bool) {
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

	// todo: wait for it to finish -> confirm that snapshots are created
	time.Sleep(5 * time.Second)

	if createPVCtrue && storageClass != "" {
		if err := createPVCsFromSnapshots(cluster, rg, snNamespace, snClass, storageClass); err != nil {
			log.Fatalf("Error creating PVCs from snapshots: %v", err)
		}
		log.Printf("Successfully created test PVCs from snapshots under namespaces: test-<original pvc namespace>")
	}
}

func createPVCsFromSnapshots(cluster k8s.ClusterInterface, rg *repv1.DellCSIReplicationGroup, snNamespace, snClass, storageClass string) error {
	ctx := context.Background()
	pvcList, err := cluster.ListPersistentVolumeClaims(ctx)
	if err != nil {
		return fmt.Errorf("error getting pvcs: %v", err)
	}

	log.Printf("Found %d pvcs", len(pvcList.Items))
	for _, pvc := range pvcList.Items {
		// step 1: retrieve the latest snapshot content from pvc
		pvName := pvc.Spec.VolumeName
		pv, err := cluster.GetPersistentVolume(ctx, pvName)
		if err != nil {
			return fmt.Errorf("error getting pv: %v", err)
		}
		pvHandle := pv.Spec.CSI.VolumeHandle
		snContentList, err := cluster.FilterVolumeSnapshotContents(ctx, pvHandle)
		if err != nil {
			return fmt.Errorf("error getting snapshot contents: %v", err)
		}

		// return error if list is empty
		if len(snContentList.Items) == 0 {
			return fmt.Errorf("no snapshot contents found for volume %s", pvName)
		}

		// get the latest snapshot content by timestamp
		timeStampLatest := pvc.CreationTimestamp
		snContentLatestName := ""

		for _, snContent := range snContentList.Items {
			if snContent.CreationTimestamp.After(timeStampLatest.Time) {
				timeStampLatest = snContent.CreationTimestamp
				snContentLatestName = snContent.Name
			}
		}

		// step 2: create cloned snapshot content -> done
		newNamespace := "test-" + pvc.Namespace
		clonedSnapshotContentName := fmt.Sprintf("cloned-%s", snContentLatestName)
		snContent, _ := cluster.GetVolumeSnapshotContent(ctx, snContentLatestName)
		snName := snContent.Spec.VolumeSnapshotRef.Name

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
					Name:      snName,
				},
				VolumeSnapshotClassName: &snClass,
			},
		}

		err = cluster.GetClient().Create(ctx, clonedSnapshotContent)
		if err != nil {
			return fmt.Errorf("error creating cloned SnapshotContent %s: %v", clonedSnapshotContentName, err)
		}

		// step 3: create new snapshot -> done
		newSnapshot := &s1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snName,
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
			return fmt.Errorf("error creating new Snapshot %s in namespace %s: %v", snName, newNamespace, err)
		}

		// step 4: create pvc from snapshot -> done
		pvcName := pvc.Name
		newPVC := &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: newNamespace,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				StorageClassName: pointer.String(storageClass),
				AccessModes:      pvc.Spec.AccessModes,
				Resources:        pvc.Spec.Resources,
				DataSource: &v1.TypedLocalObjectReference{
					APIGroup: pointer.String("snapshot.storage.k8s.io"),
					Kind:     "VolumeSnapshot",
					Name:     snName,
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
