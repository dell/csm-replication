/*
 Copyright © 2022-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"bufio"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v12 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/spf13/cobra"
)

var (
	migrationPref       = "migration.storage.dell.com"
	migrationAnnotation = ""
	migrationNS         = ""
	yes                 = false
)

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
	migrateCmd.AddCommand(migratePVCommand(&k8s.MultiClusterConfigurator{}))
	migrateCmd.AddCommand(migratePVCCommand(&k8s.MultiClusterConfigurator{}))
	migrateCmd.AddCommand(migrateSTSCommand(&k8s.MultiClusterConfigurator{}))
	migrateCmd.AddCommand(migrateMGCommand(&k8s.MultiClusterConfigurator{}))

	return migrateCmd
}

// GetMigratePVCommand returns 'migrate' cobra command
/* #nosec G104 */
func migratePVCommand(mc GetClustersInterface) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "pv",
		Short: "allows to execute migrate action on pv",
		Example: `
./repctl migrate pv <name> --to-sc <scName> (--target-ns=tns) (--wait)`,
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
			wait := viper.GetBool("pvwait")
			configFolder, err := getClustersFolderPathFunction("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failover: error getting clusters folder path: %s\n", err.Error())
			}
			migrate(mc, configFolder, "pv", pvName, "", toSc, targetNs, wait, false)
		},
	}

	migrateCmd.Flags().String("to-sc", "", "target sc")
	_ = viper.BindPFlag("pvto-sc", migrateCmd.Flags().Lookup("to-sc"))
	migrateCmd.Flags().String("target-ns", "", "target namespace")
	_ = viper.BindPFlag("pvtarget-ns", migrateCmd.Flags().Lookup("target-ns"))
	migrateCmd.Flags().Bool("wait", true, "wait for action to complete")
	_ = viper.BindPFlag("pvwait", migrateCmd.Flags().Lookup("wait"))
	err := migrateCmd.MarkFlagRequired("to-sc")
	if err != nil {
		log.Fatalf(" error in marking flag to-sc required %s", err.Error())
	}
	return migrateCmd
}

// GetMigratePVCCommand returns 'migrate' cobra command
/* #nosec G104 */
func migratePVCCommand(mc GetClustersInterface) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "pvc",
		Short: "allows to execute migrate action on pvc",
		Example: `
./repctl migrate pvc <name> --to-sc <scName> (--target-ns=tns) (--wait)`,
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
			wait := viper.GetBool("pvcwait")
			configFolder, err := getClustersFolderPathFunction("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failover: error getting clusters folder path: %s\n", err.Error())
			}
			migrate(mc, configFolder, "pvc", pvcName, pvcNS, toSc, targetNs, wait, false)
		},
	}

	migrateCmd.Flags().StringP("namespace", "n", "", "pvc namespace")
	_ = viper.BindPFlag("pvcnamespace", migrateCmd.Flags().Lookup("namespace"))
	migrateCmd.Flags().String("to-sc", "", "target sc")
	_ = viper.BindPFlag("pvcto-sc", migrateCmd.Flags().Lookup("to-sc"))
	migrateCmd.Flags().String("target-ns", "", "target namespace")
	_ = viper.BindPFlag("pvctarget-ns", migrateCmd.Flags().Lookup("target-ns"))
	migrateCmd.Flags().Bool("wait", true, "wait for action to complete")
	_ = viper.BindPFlag("pvcwait", migrateCmd.Flags().Lookup("wait"))
	err := migrateCmd.MarkFlagRequired("to-sc")
	if err != nil {
		log.Fatalf(" error in marking flag to-sc required %s", err.Error())
	}
	err = migrateCmd.MarkFlagRequired("namespace")
	if err != nil {
		log.Fatalf(" error in marking flag namespace required %s", err.Error())
	}
	return migrateCmd
}

// GetMigratePVCCommand returns 'migrate' cobra command
/* #nosec G104 */
func migrateSTSCommand(mc GetClustersInterface) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "sts",
		Short: "allows to execute migrate action on sts",
		Example: `
./repctl migrate sts -n<ns> <name> --to-sc <scName> (--target-ns=tns) (--wait) (--yes)`,
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
			wait := viper.GetBool("stswait")
			ndu := viper.GetBool("ndu")
			yes = viper.GetBool("yes")
			configFolder, err := getClustersFolderPathFunction("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("failover: error getting clusters folder path: %s\n", err.Error())
			}
			migrate(mc, configFolder, "sts", stsName, stsNS, toSc, targetNs, wait, ndu)
		},
	}

	migrateCmd.Flags().StringP("namespace", "n", "", "pvc namespace")
	_ = viper.BindPFlag("stsnamespace", migrateCmd.Flags().Lookup("namespace"))
	migrateCmd.Flags().String("to-sc", "", "target sc")
	_ = viper.BindPFlag("ststo-sc", migrateCmd.Flags().Lookup("to-sc"))
	migrateCmd.Flags().String("target-ns", "", "target namespace")
	_ = viper.BindPFlag("ststarget-ns", migrateCmd.Flags().Lookup("target-ns"))
	migrateCmd.Flags().Bool("wait", true, "wait for action to complete")
	_ = viper.BindPFlag("stswait", migrateCmd.Flags().Lookup("wait"))
	migrateCmd.Flags().Bool("ndu", false, "recreate STS in NDU manner")
	_ = viper.BindPFlag("ndu", migrateCmd.Flags().Lookup("ndu"))
	migrateCmd.Flags().BoolP("yes", "y", false, "agree with prompts")
	_ = viper.BindPFlag("yes", migrateCmd.Flags().Lookup("yes"))
	err := migrateCmd.MarkFlagRequired("to-sc")
	if err != nil {
		log.Fatalf(" error in marking flag to-sc required %s", err.Error())
	}
	err = migrateCmd.MarkFlagRequired("namespace")
	if err != nil {
		log.Fatalf(" error in marking flag namespace required %s", err.Error())
	}
	return migrateCmd
}

// GetMigrateArrayCommand returns 'migrate' cobra command
/* #nosec G104 */
func migrateMGCommand(mc GetClustersInterface) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "mg",
		Short: "allows to execute migrate action on mg",
		Example: `
./repctl migrate mg <name>  (--wait)`,
		Long: `
This command will perform a migration of all volumes on source to target.`,

		Run: func(cmd *cobra.Command, args []string) {
			if len(args) <= 0 {
				_ = cmd.Help()
				os.Exit(0)
			}
			mgName := args[0]
			wait := viper.GetBool("mgwait")
			configFolder, err := getClustersFolderPathFunction("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("migrate: error getting clusters folder path: %s\n", err.Error())
			}
			migrateMG(mc, configFolder, "mg", mgName, wait)
		},
	}

	migrateCmd.Flags().Bool("wait", true, "wait for action to complete")
	_ = viper.BindPFlag("mgwait", migrateCmd.Flags().Lookup("wait"))
	return migrateCmd
}

func migrate(mc GetClustersInterface, configFolder, resource string, resName string, resNS string, toSC string, targetNS string, wait bool, ndu bool) {
	clusterIDs := viper.GetStringSlice(config.Clusters)
	clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
	if err != nil {
		log.Fatalf("edit secret: error in initializing cluster info: %s", err.Error())
	}
	switch resource {
	case "pv":
		wg := &sync.WaitGroup{}
		for _, i := range clusters.Clusters {
			wg.Add(1)
			go migratePV(context.Background(), i, resName, toSC, targetNS, wg, wait)
		}
		wg.Wait()

	case "pvc":
		wg := &sync.WaitGroup{}
		for _, i := range clusters.Clusters {
			pvc, err := i.GetPersistentVolumeClaim(context.Background(), resNS, resName)
			if err != nil {
				log.Error(err)
				os.Exit(1)
			}
			log.Info(pvc.OwnerReferences)
			pvName := pvc.Spec.VolumeName
			wg.Add(1)
			go migratePV(context.Background(), i, pvName, toSC, targetNS, wg, wait)
		}
		wg.Wait()

	case "sts":
		wg := &sync.WaitGroup{}
		for _, i := range clusters.Clusters {
			sts, err := i.GetStatefulSet(context.Background(), resNS, resName)
			if err != nil {
				log.Error(err)
				os.Exit(1)
			}
			list, err := i.FilterPods(context.Background(), resNS, sts.Name)
			if err != nil {
				log.Error(err)
				os.Exit(1)
			}
			scMap := make(map[string]struct{})
			for _, template := range sts.Spec.VolumeClaimTemplates {
				if template.Spec.StorageClassName != nil {
					scMap[*template.Spec.StorageClassName] = struct{}{}
				}
			}
			if len(scMap) > 1 {
				log.Warnf("Multiple source StorageClasses detected in StatefulSet. Make sure all of your volumes can be migrated to target SC.")
				for s := range scMap {
					log.Infof("SC: %s", s)
				}
				fmt.Println("Do you want to continue? [y/N]")
				if !yes {
					reader := bufio.NewReader(os.Stdin)
					fmt.Print("-> ")
					charContinue, _, err := reader.ReadRune()
					if err != nil {
						log.Error(err)
					}
					switch charContinue {
					case 'y', 'Y':
					default:
						return
					}
				}
				log.Info("Continuing")
			}
			for _, pod := range list.Items {
				for _, volume := range pod.Spec.Volumes {
					if volume.PersistentVolumeClaim != nil {
						pvc, err := i.GetPersistentVolumeClaim(context.Background(), resNS, volume.PersistentVolumeClaim.ClaimName)
						if err != nil {
							log.Error(err)
							os.Exit(1)
						}
						wg.Add(1)
						go migratePV(context.Background(), i, pvc.Spec.VolumeName, toSC, targetNS, wg, wait || ndu)
					}
				}
			}
		}
		wg.Wait()
		if ndu {
			for _, i := range clusters.Clusters {
				sts, err := i.GetStatefulSet(context.Background(), resNS, resName)
				if err != nil {
					log.Error(err)
					os.Exit(1)
				}
				err = recreateStsNdu(i, sts, toSC)
				if err != nil {
					log.Error("Failed to recreate STS: ", err)
				}
			}
		}
	default:
		log.Error("Unknown resource")
		os.Exit(1)
	}
}

func migratePV(ctx context.Context, cluster k8s.ClusterInterface, pvName string, toSC string, targetNS string, wg *sync.WaitGroup, wait bool) {
	defer wg.Done()
	log.Info(pvName)
	pv, err := cluster.GetPersistentVolume(context.Background(), pvName)
	if err != nil {
		log.Error(err, "Unable to find backing PV")
		os.Exit(1)
	}
	log.Infof("Setting migration annotation %s on pv %s", migrationAnnotation+"/"+toSC, pv.Name)
	annotations := pv.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[migrationAnnotation] = toSC
	if len(targetNS) > 0 {
		annotations[migrationNS] = targetNS
	}
	pv.Annotations = annotations
	err = cluster.UpdatePersistentVolume(context.Background(), pv)
	if err != nil {
		log.Error(err, "unable to update persistent volume")
		os.Exit(1)
	}
	if wait {
		done := waitForPVToBeBound(pvName+"-to-"+toSC, cluster)
		if done {
			fmt.Printf("Successfully updated pv") // This message is validated in UT; do not delete or change
			log.Infof("Successfully updated pv %s in cluster %s. Consider using new PV: [%s]", pv.Name, cluster.GetID(), pvName+"-to-"+toSC)
		} else {
			log.Error("time out waiting for the PV to be bound")
		}
	} else {
		fmt.Printf("Successfully updated pv") // This message is validated in UT; do not delete or change
		log.Infof("Successfully updated pv %s in cluster %s. Consider using new PV: [%s]", pv.Name, cluster.GetID(), pvName+"-to-"+toSC)
	}
}

func waitForPVToBeBound(pvName string, cluster k8s.ClusterInterface) bool {
	log.Info("migrated pv name:", pvName)
	t := time.NewTicker(5 * time.Second)
	ret := make(chan bool)
	go func() {
		log.Print("Waiting for action to complete ...")
		for {
			select {
			case <-time.After(5 * time.Minute):
				ret <- false
			case <-t.C:
				pv, err := cluster.GetPersistentVolume(context.Background(), pvName)
				if err != nil && !errors.IsNotFound(err) {
					log.Fatalf("migrate: error in fecthing pv info: %s\n", err.Error())
				}
				if pv != nil && (pv.Status.Phase == v1.VolumeBound || pv.Status.Phase == v1.VolumeAvailable) {
					ret <- true
					return
				}
			}
		}
	}()
	res := <-ret
	return res
}

func waitForPodToBeReady(podName string, podNS string, cluster k8s.ClusterInterface) bool {
	t := time.NewTicker(5 * time.Second)
	ret := make(chan bool)
	go func() {
		log.Print("Waiting for action to complete ...")
		for {
			select {
			case <-time.After(5 * time.Minute):
				ret <- false
			case <-t.C:
				pod, err := cluster.GetPod(context.Background(), podName, podNS)
				if err != nil && !errors.IsNotFound(err) {
					log.Fatalf("migrate: error in fecthing pod info: %s\n", err.Error())
				}
				if pod != nil && pod.Status.Phase == v1.PodRunning {
					for _, condition := range pod.Status.Conditions {
						if condition.Type == v1.PodReady {
							ret <- true
							return
						}
					}
				}
			}
		}
	}()
	res := <-ret
	return res
}

func recreateStsNdu(cluster k8s.ClusterInterface, sts *v12.StatefulSet, targetSC string) error {
	stsDeepCopy := sts.DeepCopy()
	stsDeepCopy.ResourceVersion = ""
	if sts.Spec.Replicas != nil {
		if *sts.Spec.Replicas <= 1 {
			return errors.NewBadRequest("Unable to perform NDU with replicas <= 1")
		}
	}
	log.Info("Trying to delete original sts with orphan option")
	err := cluster.DeleteStsOrphan(context.Background(), sts)
	if err != nil {
		return err
	}
	log.Info("Changing SC in STS manifest")
	for _, template := range stsDeepCopy.Spec.VolumeClaimTemplates {
		template.Spec.StorageClassName = &targetSC
	}
	log.Info("trying to apply modified sts")
	err = cluster.CreateStatefulSet(context.Background(), stsDeepCopy)
	if err != nil {
		return err
	}

	list, err := cluster.FilterPods(context.Background(), sts.Namespace, sts.Name)
	if err != nil {
		return err
	}
	log.Info("trying to delete old resources")
	for i := range list.Items {
		pod := list.Items[i]
		for j := range pod.Spec.Volumes {
			volume := pod.Spec.Volumes[j]
			if volume.PersistentVolumeClaim != nil {
				pvc, err := cluster.GetPersistentVolumeClaim(context.Background(), sts.Namespace, volume.PersistentVolumeClaim.ClaimName)
				if err != nil {
					return err
				}
				log.Infof("trying to delete pvc %s", pvc.Name)
				err = cluster.DeletePersistentVolumeClaim(context.Background(), pvc, &client.DeleteOptions{})
				if err != nil {
					return err
				}

			}
		}
		log.Infof("trying to delete pod %s", pod.Name)
		err = cluster.DeletePod(context.Background(), &pod, &client.DeleteOptions{})
		if err != nil {
			return err
		}
		done := waitForPodToBeReady(pod.Name, pod.Namespace, cluster)
		if !done {
			return err
		}
	}
	return nil
}

func migrateMG(mc GetClustersInterface, configFolder, resource string, resName string, wait bool) {
	clusterIDs := viper.GetStringSlice(config.Clusters)
	clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
	if err != nil {
		log.Fatalf("edit secret: error in initializing cluster info: %s", err.Error())
	}
	wg := &sync.WaitGroup{}
	for _, cluster := range clusters.Clusters {
		_, err := cluster.GetMigrationGroup(context.Background(), resName)
		if err != nil {
			// skip the cluster on which MG is not found
			continue
		}
		wg.Add(1)
		go migrateArray(context.Background(), cluster, resName, wg, wait)
	}
	wg.Wait()
}

func migrateArray(ctx context.Context, cluster k8s.ClusterInterface, mgName string, wg *sync.WaitGroup, wait bool) {
	defer wg.Done()
	log.Info(mgName)
	mg, err := cluster.GetMigrationGroup(context.Background(), mgName)
	if err != nil {
		log.Error(err, "Unable to find mg")
		os.Exit(1)
	}
	annotations := mg.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	migrationAnnotation = "ArrayMigrate"
	Action := "Ready"
	log.Infof("Setting Array migration annotation %s on mg %s", migrationAnnotation+"/"+Action, mg.Name)
	annotations[migrationAnnotation] = Action
	mg.Annotations = annotations
	err = cluster.UpdateMigrationGroup(context.Background(), mg)
	if err != nil {
		log.Error(err, "unable to update mg")
		os.Exit(1)
	}
	// Command exit criteria
	if wait {
		done := waitForArrayMigration(mgName, cluster)
		if done {
			fmt.Printf("Successfully migrated all volumes from source array to target") // This message is validated in UT; do not delete or change
			log.Infof("Successfully migrated all volumes from source array to target")
		} else {
			log.Error("time out waiting for array migration")
		}
	} else {
		fmt.Printf("Successfully migrated all volumes from source array to target") // This message is validated in UT; do not delete or change
		log.Infof("Successfully migrated all volumes from source array to target")
	}
}

func waitForArrayMigration(mgName string, cluster k8s.ClusterInterface) bool {
	t := time.NewTicker(5 * time.Second)
	ret := make(chan bool)
	go func() {
		log.Print("Waiting for action to complete ...")
		for {
			select {
			case <-time.After(5 * time.Minute):
				ret <- false
			case <-t.C:
				mg, err := cluster.GetMigrationGroup(context.Background(), mgName)
				if err != nil && !errors.IsNotFound(err) {
					log.Fatalf("migrate: error in fecthing pv info: %s\n", err.Error())
				}
				if mg != nil && (mg.Status.State == "Committed") {
					ret <- true
					return
				}
			}
		}
	}()
	res := <-ret
	return res
}
