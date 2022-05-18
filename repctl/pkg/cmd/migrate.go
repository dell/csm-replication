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
	"github.com/spf13/viper"
	"os"
	"sync"

	"github.com/spf13/cobra"
)

var migrationPref = "migration.storage.dell.com"
var migrationAnnotation = ""
var migrationNS = ""

// GetMigrateCommand returns 'edit' cobra command
func GetMigrateCommand() *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "migrate storage resource to different SC", //
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) <= 0 {
				_ = cmd.Help()
				os.Exit(0)
			}
		},
	}
	migrateCmd.Flags().String("migration-prefix", migrationPref, "migration-prefix")
	_ = viper.BindPFlag("migration-prefix", migrateCmd.Flags().Lookup("migration-prefix"))
	migrationAnnotation = viper.GetString("migration-prefix") + "/migrate-to"
	migrationNS = viper.GetString("migration-prefix") + "/namespace"
	migrateCmd.AddCommand(migratePVCommand())
	migrateCmd.AddCommand(migratePVCCommand())
	migrateCmd.AddCommand(migrateSTSCommand())

	return migrateCmd
}

// GetMigratePVCommand returns 'migrate' cobra command
/* #nosec G104 */
func migratePVCommand() *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "pv",
		Short: "allows to execute migrate action on pv",
		Example: `
./repctl migrate pv <name> --to-sc <scName> (--target-ns=tns)`,
		Long: `
This command will perform a migrate command to target StorageClass.`,

		Run: func(cmd *cobra.Command, args []string) {
			if len(args) <= 0 {
				_ = cmd.Help()
				os.Exit(0)
			}
			pvName := args[0]
			toSc := viper.GetString("pvto-sc")
			targetNs := viper.GetString("pvtarget-ns")
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failover: error getting clusters folder path: %s\n", err.Error())
			}
			migrate(configFolder, "pv", pvName, "", toSc, targetNs)
		},
	}

	migrateCmd.Flags().String("to-sc", "", "target sc")
	_ = viper.BindPFlag("pvto-sc", migrateCmd.Flags().Lookup("to-sc"))
	migrateCmd.Flags().String("target-ns", "", "target namespace")
	_ = viper.BindPFlag("pvtarget-ns", migrateCmd.Flags().Lookup("target-ns"))
	migrateCmd.MarkFlagRequired("to-sc")
	return migrateCmd
}

// GetMigratePVCCommand returns 'migrate' cobra command
/* #nosec G104 */
func migratePVCCommand() *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "pvc",
		Short: "allows to execute migrate action on pvc",
		Example: `
./repctl migrate pvc <name> --to-sc <scName> (--target-ns=tns)`,
		Long: `
This command will perform a migrate command to target StorageClass.`,

		Run: func(cmd *cobra.Command, args []string) {
			if len(args) <= 0 {
				_ = cmd.Help()
				os.Exit(0)
			}
			pvcName := args[0]
			pvcNS := viper.GetString("pvcnamespace")
			toSc := viper.GetString("pvcto-sc")
			targetNs := viper.GetString("pvctarget-ns")
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failover: error getting clusters folder path: %s\n", err.Error())
			}
			migrate(configFolder, "pvc", pvcName, pvcNS, toSc, targetNs)
		},
	}

	migrateCmd.Flags().StringP("namespace", "n", "", "pvc namespace")
	_ = viper.BindPFlag("pvcnamespace", migrateCmd.Flags().Lookup("namespace"))
	migrateCmd.Flags().String("to-sc", "", "target sc")
	_ = viper.BindPFlag("pvcto-sc", migrateCmd.Flags().Lookup("to-sc"))
	migrateCmd.Flags().String("target-ns", "", "target namespace")
	_ = viper.BindPFlag("pvctarget-ns", migrateCmd.Flags().Lookup("target-ns"))
	migrateCmd.MarkFlagRequired("to-sc")
	migrateCmd.MarkFlagRequired("namespace")
	return migrateCmd
}

// GetMigratePVCCommand returns 'migrate' cobra command
/* #nosec G104 */
func migrateSTSCommand() *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "sts",
		Short: "allows to execute migrate action on sts",
		Example: `
./repctl migrate sts -n<ns> <name> --to-sc <scName> (--target-ns=tns)`,
		Long: `
This command will perform a migrate command to target StorageClass.`,

		Run: func(cmd *cobra.Command, args []string) {
			if len(args) <= 0 {
				_ = cmd.Help()
				os.Exit(0)
			}
			stsName := args[0]
			stsNS := viper.GetString("stsnamespace")
			toSc := viper.GetString("ststo-sc")
			targetNs := viper.GetString("ststarget-ns")

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failover: error getting clusters folder path: %s\n", err.Error())
			}
			migrate(configFolder, "sts", stsName, stsNS, toSc, targetNs)
		},
	}

	migrateCmd.Flags().StringP("namespace", "n", "", "pvc namespace")
	_ = viper.BindPFlag("stsnamespace", migrateCmd.Flags().Lookup("namespace"))
	migrateCmd.Flags().String("to-sc", "", "target sc")
	_ = viper.BindPFlag("ststo-sc", migrateCmd.Flags().Lookup("to-sc"))
	migrateCmd.Flags().String("target-ns", "", "target namespace")
	_ = viper.BindPFlag("ststarget-ns", migrateCmd.Flags().Lookup("target-ns"))
	migrateCmd.MarkFlagRequired("to-sc")
	migrateCmd.MarkFlagRequired("namespace")
	return migrateCmd
}

func migrate(configFolder, resource string, resName string, resNS string, toSC string, targetNS string) {
	log.Info("migrate", configFolder, resource, resName, resNS, toSC, targetNS)
	log.Info(toSC)
	clusterIDs := viper.GetStringSlice(config.Clusters)
	log.Info(clusterIDs)
	mc := &k8s.MultiClusterConfigurator{}
	clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
	if err != nil {
		log.Fatalf("edit secret: error in initializing cluster info: %s", err.Error())
	}
	switch resource {
	case "pv":
		wg := &sync.WaitGroup{}
		for _, i := range clusters.Clusters {
			wg.Add(1)
			migratePV(context.Background(), i, resName, toSC, targetNS, wg)
			wg.Wait()
		}
	case "pvc":
		wg := &sync.WaitGroup{}
		for _, i := range clusters.Clusters {
			pvc, err := i.GetPersistentVolumeClaim(context.Background(), resNS, resName)
			if err != nil {
				log.Error(err)
				os.Exit(1)
			}
			pvName := pvc.Spec.VolumeName
			wg.Add(1)
			go migratePV(context.Background(), i, pvName, toSC, targetNS, wg)
			wg.Wait()
		}
	case "sts":
		wg := &sync.WaitGroup{}
		for _, i := range clusters.Clusters {
			sts, err := i.GetStatefulSet(context.Background(), resNS, resName)
			if err != nil {
				log.Error(err)
				os.Exit(1)
			}
			pvcList := sts.Spec.VolumeClaimTemplates
			for _, pvc := range pvcList {
				log.Infof("Starting processing PVC %s", pvc.Name)
				pvName := pvc.Spec.VolumeName
				wg.Add(1)
				migratePV(context.Background(), i, pvName, toSC, targetNS, wg)
				wg.Wait()
			}

		}
	default:
		log.Error("Unknown resource")
		os.Exit(1)
	}
}

func migratePV(ctx context.Context, cluster k8s.ClusterInterface, pvName string, toSC string, targetNS string, wg *sync.WaitGroup) {
	defer wg.Done()
	pv, err := cluster.GetPersistentVolume(context.Background(), pvName)
	if err != nil {
		log.Error(err, "Unable to find backing PV")
		os.Exit(1)
	}
	log.Infof("Setting migration annotation %s on pv %s", migrationAnnotation+"/"+toSC, pv.Name)
	pv.Annotations[migrationAnnotation] = toSC
	pv.Annotations[migrationNS] = targetNS
	err = cluster.UpdatePersistentVolume(context.Background(), pv)
	if err != nil {
		log.Error(err, "unable to update persistent volume")
		os.Exit(1)
	}
	log.Infof("Successfully updated pv %s in cluster %s", pv.Name, cluster.GetID())
}
