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
	"os"

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetListCommand returns 'list' cobra command
/* #nosec G104 */
func GetListCommand() *cobra.Command {
	listCmd := &cobra.Command{
		Use:     "get",
		Short:   "lists different resources in clusters with configured replication",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				_ = cmd.Help()
				os.Exit(0)
			}
		},
	}

	listCmd.PersistentFlags().BoolP("all", "A",false, "show all objects (overrides other filters)")
	_ = viper.BindPFlag("all", listCmd.PersistentFlags().Lookup("all"))
	_ = viper.BindPFlag("all", listCmd.PersistentFlags().ShorthandLookup("A"))

	listCmd.PersistentFlags().String("rn", "", "remote namespace")
	_ = viper.BindPFlag("rn", listCmd.PersistentFlags().Lookup("rn"))

	listCmd.PersistentFlags().StringSlice("rc", []string{""}, "remote cluster id")
	_ = viper.BindPFlag("rc", listCmd.PersistentFlags().Lookup("rc"))

	listCmd.AddCommand(getListStorageClassesCommand())
	listCmd.AddCommand(getListPersistentVolumesCommand())
	listCmd.AddCommand(getListPersistentVolumeClaimsCommand())
	listCmd.AddCommand(getListReplicationGroupsCommand())

	return listCmd
}

func getListStorageClassesCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "sc",
		Aliases: []string{"storageclass", "storageclasses"},
		Short:   "list storage classes",
		Example: `./repctl get sc`,
		Long: `
Filter out storage classes which have replication enabled.
You can also list all storage classes by passing --all (-A) flag`,

		Run: func(cmd *cobra.Command, args []string) {
			log.Print("listing storage classes")

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("list sc: error getting clusters folder path: %s", err.Error())
			}

			clusterIDs := viper.GetStringSlice(config.Clusters)

			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
			if err != nil {
				log.Fatalf("list sc: error in initializing cluster info: %s", err.Error())
			}

			driverName := viper.GetString(config.Driver)
			noFilter := viper.GetBool("all")

			for _, cluster := range clusters.Clusters {
				log.Printf("Cluster: %s", cluster.GetID())

				scList, err := cluster.FilterStorageClass(context.Background(), driverName, noFilter)
				if err != nil {
					log.Printf("Encountered error during filtering storage classes. Error: %s",
						err.Error())
					continue
				}
				scList.Print()
				log.Print()
			}
		},
	}
}

func getListPersistentVolumesCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "pv",
		Aliases: []string{"persistentvolumes", "persistentvolume"},
		Short:   "list Persistent Volumes",
		Example: `./repctl get pv --all (-A)`,
		Long: `
List Persistent Volumes in the specified clusters.
You can also filter PersistentVolumes based on filters like
Remote Namespace, Remote ClusterId`,
		Run: func(cmd *cobra.Command, args []string) {
			log.Print("listing persistent volumes")

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("list pv: error getting clusters folder path: %s", err.Error())
			}

			clusterIDs := viper.GetStringSlice(config.Clusters)

			log.Print(clusterIDs)

			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
			if err != nil {
				log.Fatalf("list pv: error in initializing cluster info: %s", err.Error())
			}

			rNamespace := viper.GetString("rn")
			remoteClusterID := viper.GetString("rc")
			driverName := viper.GetString(config.Driver)
			rgName := viper.GetString(config.ReplicationGroup)
			noFilter := viper.GetBool("all")

			for _, cluster := range clusters.Clusters {
				log.Printf("Cluster: %s", cluster.GetID())

				var pvList []types.PersistentVolume
				var err error

				if noFilter {
					pvList, err = cluster.FilterPersistentVolumes(context.Background(), "", "", "", "")
				} else {
					pvList, err = cluster.FilterPersistentVolumes(context.Background(), driverName, remoteClusterID, rNamespace, rgName)
				}
				if err != nil {
					log.Printf("Encountered error during filtering persistent volumes. Error: %s",
						err.Error())
					continue
				}

				printableList := &types.PersistentVolumeList{PVList: pvList}
				printableList.Print()

				log.Print()
			}
		},
	}
}

/* #nosec G104 */
func getListPersistentVolumeClaimsCommand() *cobra.Command {
	listPVC := &cobra.Command{
		Use:     "pvc",
		Aliases: []string{"persistentvolumeclaims", "persistentvolumeclaim"},
		Short:   "list PersistentVolumeClaims",
		Example: `./repctl get pvc --all (-A)`,
		Long: `
List PersistentVolumeClaim objects which are replicated.
You can apply filters like remoteClusterId, remoteNamespace.`,
		Run: func(cmd *cobra.Command, args []string) {
			log.Print("listing persistent volume claims")

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("list pvc: error getting clusters folder path: %s", err.Error())
			}

			clusterIDs := viper.GetStringSlice(config.Clusters)

			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
			if err != nil {
				log.Fatalf("list pvc: error in initializing cluster info: %s", err.Error())
			}

			namespace := viper.GetString("namespace")
			rNamespace := viper.GetString("rn")
			rclusterID := viper.GetString("rc")
			rgName := viper.GetString(config.ReplicationGroup)
			noFilter := viper.GetBool("all")

			for _, cluster := range clusters.Clusters {
				log.Printf("Cluster: %s", cluster.GetID())

				var pvcList *types.PersistentVolumeClaimList
				var err error

				if noFilter {
					pvcList, err = cluster.FilterPersistentVolumeClaims(context.Background(),
						"", "", "", "")
				} else {
					pvcList, err = cluster.FilterPersistentVolumeClaims(context.Background(),
						namespace, rclusterID, rNamespace, rgName)
				}
				if err != nil {
					log.Printf("Encountered error during filtering persistent volume claims. Error: %s",
						err.Error())
					continue
				}

				pvcList.Print()
				log.Print()
			}
		},
	}

	listPVC.Flags().StringP("namespace", "n", "", "namespace for PVC")
	_ = viper.BindPFlag("namespace", listPVC.Flags().Lookup("namespace"))

	return listPVC
}

func getListReplicationGroupsCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "rg",
		Aliases: []string{"replicationgroups", "replicationgroup"},
		Short:   "get ReplicationGroup",
		Long: `List DellCSIReplicationGroup Custom Resource (CR)
instances on the set of provided cluster ids. You can also provide filters like
remote cluster id (rc) & driver name`,
		Run: func(cmd *cobra.Command, args []string) {
			log.Print("listing replication groups")

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("list pvc: error getting clusters folder path: %s", err.Error())
			}

			clusterIDs := viper.GetStringSlice(config.Clusters)

			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
			if err != nil {
				log.Fatalf("list pvc: error in initializing cluster info: %s", err.Error())
			}

			remoteClusterID := viper.GetString("rc")
			driverName := viper.GetString(config.Driver)

			for _, cluster := range clusters.Clusters {
				log.Printf("Cluster: %s", cluster.GetID())

				rgList, err := cluster.FilterReplicationGroups(context.Background(), driverName, remoteClusterID)
				if err != nil {
					log.Printf("Encountered error during filtering persistent volume claims. Error: %s",
						err.Error())
					continue
				}
				rgList.Print()
				log.Print()
			}
		},
	}
}
