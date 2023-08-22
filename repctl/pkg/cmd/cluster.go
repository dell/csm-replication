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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

// GetClusterCommand returns 'cluster' cobra command
func GetClusterCommand() *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:     "cluster",
		Aliases: []string{"clusters"},
		Short:   "allows to manipulate cluster configs",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				_ = cmd.Help()
				os.Exit(0)
			}
		},
	}

	clusterCmd.AddCommand(getAddClusterCommand())
	clusterCmd.AddCommand(getRemoveClusterCommand())
	clusterCmd.AddCommand(getListClusterCommand())
	clusterCmd.AddCommand(getInjectClustersCommand())

	return clusterCmd
}

/* CLI COMMANDS */

/* #nosec G104 */
func getAddClusterCommand() *cobra.Command {
	addClusterCmd := &cobra.Command{
		Use:   "add",
		Short: "adds a cluster to be managed by repctl",
		Example: `
			./repctl cluster add -f <file> -n <name>
			./repctl cluster add -f <file1> -n <name1> -f <file2> -n <name2> ...
			./repctl cluster add <...> --auto-inject
		`,
		Run: func(cmd *cobra.Command, args []string) {
			configs := viper.GetStringSlice("files")
			clusterNames := viper.GetStringSlice("add-name")
			force := viper.GetBool("force")
			autoInject := viper.GetBool("auto-inject")

			err := addCluster(configs, clusterNames, "/.repctl/clusters/", force)
			if err != nil {
				log.Fatalf("cluster add: %s", err.Error())
			}

			if autoInject {
				mc := &k8s.MultiClusterConfigurator{}
				clusterIDs := viper.GetStringSlice(config.Clusters)

				err := injectCluster(mc, clusterIDs, "/.repctl/clusters/")
				if err != nil {
					log.Fatalf("cluster add: auto-inject: %s", err.Error())
				}
			}
		},
	}

	addClusterCmd.Flags().StringSliceP("files", "f", []string{}, "paths for kube config file")
	_ = addClusterCmd.MarkFlagRequired("file")
	_ = viper.BindPFlag("files", addClusterCmd.Flags().Lookup("files"))

	addClusterCmd.Flags().StringSliceP("names", "n", []string{}, "cluster names")
	_ = addClusterCmd.MarkFlagRequired("names")
	_ = viper.BindPFlag("add-name", addClusterCmd.Flags().Lookup("names"))

	addClusterCmd.Flags().Bool("force", false, "force flag")
	_ = viper.BindPFlag("force", addClusterCmd.Flags().Lookup("force"))

	addClusterCmd.Flags().Bool("auto-inject", false, "inject added configs to k8s clusters as config maps")
	_ = viper.BindPFlag("auto-inject", addClusterCmd.Flags().Lookup("auto-inject"))

	return addClusterCmd
}

/* #nosec G104 */
func getRemoveClusterCommand() *cobra.Command {
	removeClusterCmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "removes cluster by name from list of clusters being managed by repctl",
		Run: func(cmd *cobra.Command, args []string) {
			clusterName := viper.GetString("remove-name")

			err := removeCluster(clusterName, "/.repctl/clusters/")
			if err != nil {
				log.Fatalf("cluster remove: %s", err.Error())
			}
			log.Printf("removed cluster %s", clusterName)
		},
	}

	removeClusterCmd.Flags().StringP("name", "n", "", "cluster name")
	_ = removeClusterCmd.MarkFlagRequired("name")
	_ = viper.BindPFlag("remove-name", removeClusterCmd.Flags().Lookup("name"))

	return removeClusterCmd
}

func getListClusterCommand() *cobra.Command {
	listClusterCmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"ls"},
		Short:   "list all clusters currently being managed by repctl",
		Run: func(cmd *cobra.Command, args []string) {
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("cluster list: error getting clusters folder path: %s", err.Error())
			}

			mc := &k8s.MultiClusterConfigurator{}

			clusters, err := mc.GetAllClusters([]string{}, configFolder)
			if err != nil {
				log.Fatalf("cluster list: error in initializing cluster info: %s", err.Error())
			}
			clusters.Print()
		},
	}

	return listClusterCmd
}

/* #nosec G104 */
func getInjectClustersCommand() *cobra.Command {
	injectCmd := &cobra.Command{
		Use:   "inject",
		Short: "inject current config as config-map in clusters",
		Example: `
			./repctl cluster inject 
			./repctl cluster inject --custom-configs <path-to-config-1>,<path-to-config-2>...
		`,
		Run: func(cmd *cobra.Command, args []string) {
			clusterIDs := viper.GetStringSlice(config.Clusters)
			customConfigs := viper.GetStringSlice("custom-configs")
			useSA := viper.GetBool("use-sa")

			mc := &k8s.MultiClusterConfigurator{}
			if useSA {
				generatedConfigs, err := generateConfigsFromSA(mc, clusterIDs)
				defer os.RemoveAll("/tmp/repctl")
				if err != nil {
					log.Fatalf("cluster inject: generate cfg: %s", err.Error())
				}
				customConfigs = append(customConfigs, generatedConfigs...)
			}

			err := injectCluster(mc, clusterIDs, "/.repctl/clusters/", customConfigs...)
			if err != nil {
				log.Fatalf("cluster inject: %s", err.Error())
			}
		},
	}

	injectCmd.Flags().StringSlice("custom-configs", []string{}, "paths to custom kubeconfigs")
	_ = viper.BindPFlag("custom-configs", injectCmd.Flags().Lookup("custom-configs"))

	injectCmd.Flags().Bool("use-sa", false, "create kubeconfigs from replicator service account and inject them as config maps")
	_ = viper.BindPFlag("use-sa", injectCmd.Flags().Lookup("use-sa"))

	return injectCmd
}

/* METHODS */

func addCluster(configs, clusterNames []string, folderPath string, force bool) error {
	log.Print("Adding clusters")

	if len(configs) != len(clusterNames) {
		return fmt.Errorf("number of config files != number of cluster names")
	}

	for i, cfg := range configs {
		clusterName := clusterNames[i]
		err := updateClusters(cfg, clusterName, folderPath, force)
		if err != nil {
			return fmt.Errorf("error when adding config %s: %s", cfg, err.Error())
		}
	}

	return nil
}

func injectCluster(mc k8s.MultiClusterConfiguratorInterface, clusterIDs []string, path string, customConfigs ...string) error {
	configFolder, err := getClustersFolderPath(path)
	if err != nil {
		return fmt.Errorf("error getting clusters folder path: %s", err.Error())
	}

	clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
	if err != nil {
		return fmt.Errorf("error in initializing cluster info: %s", err.Error())
	}

	configs := &k8s.Clusters{}
	if len(customConfigs) != 0 {
		log.Print("Custom configs provided, injecting them into clusters")
		for _, cfg := range customConfigs {
			info, err := os.Stat(cfg)
			if err != nil {
				return fmt.Errorf("error checking cfg %s: %s", cfg, err.Error())
			}

			cluster, err := k8s.CreateCluster(info.Name(), cfg)
			if err != nil {
				return fmt.Errorf("error creating cluster: %s", err.Error())
			}

			configs.Clusters = append(configs.Clusters, cluster)
		}
	} else {
		log.Print("Injecting current cluster configuration to all clusters")
		configs = clusters
	}

	for _, srcCluster := range clusters.Clusters {
		err := injectConfigIntoCluster(srcCluster, configs)
		if err != nil {
			log.Printf("error during injecting configs: %s", err.Error())
			continue
		}
	}

	return nil
}

func removeCluster(clusterName string, path string) error {
	log.Printf("Removing cluster %s", clusterName)
	folderPath, err := getClustersFolderPath(path)
	if err != nil {
		return err
	}

	destFileName := filepath.Join(folderPath, clusterName)
	err = os.Remove(destFileName)
	if err != nil {
		return fmt.Errorf("failed to remove file: %s, error: %s", destFileName, err.Error())
	}

	return nil
}

/* UTILITY */

func injectConfigIntoCluster(srcCluster k8s.ClusterInterface, clusters *k8s.Clusters) error {
	type Target struct {
		ClusterID string `yaml:"clusterId"`
		Address   string `yaml:"address"`
		SecretRef string `yaml:"secretRef"`
	}

	type ConnectionConfig struct {
		ClusterID string   `yaml:"clusterId"`
		Targets   []Target `yaml:"targets"`
		LogLevel  string   `yaml:"CSI_LOG_LEVEL"`
	}

	namespace := "dell-replication-controller"
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       corev1.SchemeGroupVersion.String(),
			APIVersion: "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	_, err := srcCluster.GetNamespace(context.Background(), namespace)
	if err != nil {
		log.Printf("Creating %s namespace ", namespace)
		cErr := srcCluster.CreateNamespace(context.Background(), ns)
		if cErr != nil {
			return fmt.Errorf("error creating namespace: %s", err.Error())
		}
	}

	connectionConfig := ConnectionConfig{
		ClusterID: srcCluster.GetID(),
		LogLevel:  "INFO",
	}

	for _, tgtCluster := range clusters.Clusters {
		if srcCluster.GetID() == tgtCluster.GetID() {
			continue
		}

		secret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      tgtCluster.GetID(),
				Namespace: namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{},
		}

		data, err := os.ReadFile(tgtCluster.GetKubeConfigFile())
		if err != nil {
			return fmt.Errorf("error while trying to read config file: %s", err.Error())
		}

		err = addKeyFromLiteralToSecret(secret, "data", data)
		if err != nil {
			return fmt.Errorf("error while adding file to secret: %s", err.Error())
		}

		log.Printf("Creating/Updating %s secret in %s cluster ", secret.Name, srcCluster.GetID())
		err = srcCluster.GetClient().Create(context.Background(), secret)
		if err != nil {
			uErr := srcCluster.GetClient().Update(context.Background(), secret)
			if uErr != nil {
				return fmt.Errorf("error while creating secret in cluster: %s", err.Error())
			}
		}
		log.Printf("secret %s created ", secret.Name)

		connectionConfig.Targets = append(connectionConfig.Targets, Target{
			ClusterID: tgtCluster.GetID(),
			Address:   tgtCluster.GetHost(),
			SecretRef: tgtCluster.GetID(),
		})
	}

	bytes, err := yaml.Marshal(connectionConfig)
	if err != nil {
		return fmt.Errorf("error while marshaling connection config: %s", err.Error())
	}

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dell-replication-controller-config",
			Namespace: namespace,
		},
		Data: map[string]string{},
	}
	configMap.Data["config.yaml"] = string(bytes)

	log.Printf("Creating/Updating replication config map in %s cluster ", srcCluster.GetID())
	err = srcCluster.GetClient().Create(context.Background(), configMap)
	if err != nil {
		cmap := &corev1.ConfigMap{}
		err := srcCluster.GetClient().Get(context.Background(), types.NamespacedName{Name: "dell-replication-controller-config", Namespace: namespace}, cmap)
		if err != nil {
			return fmt.Errorf("error while trying to get existing config: %s", err.Error())
		}
		gotData := &ConnectionConfig{}
		err = yaml.Unmarshal([]byte(cmap.Data["config.yaml"]), gotData)
		if err != nil {
			return fmt.Errorf("error while unmarshaling connection config: %s", err.Error())
		}
		connectionConfig.LogLevel = gotData.LogLevel
		bytes, err := yaml.Marshal(connectionConfig)
		if err != nil {
			return fmt.Errorf("error while marshaling connection config: %s", err.Error())
		}
		configMap.Data["config.yaml"] = string(bytes)

		uErr := srcCluster.GetClient().Update(context.Background(), configMap)
		if uErr != nil {
			return fmt.Errorf("error while creating config map in cluster: %s", uErr.Error())
		}
	}
	log.Printf("config map %s created ", configMap.Name)
	return nil
}

func updateClusters(kubeConfig, clusterName, path string, force bool) error {
	log.Print(clusterName)

	folderPath, err := getClustersFolderPath(path)
	if err != nil {
		return err
	}

	srcFile, err := os.Open(filepath.Clean(kubeConfig))
	if err != nil {
		return fmt.Errorf("failed to open file: %s, error: %s", kubeConfig, err.Error())
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			log.Errorf("error encountered in closing src file. Error: %s", err.Error())
		}
	}()

	fileStats, err := srcFile.Stat()
	if err != nil {
		return err
	}
	// Don't allow file sizes more than 5 MB
	if fileStats.Size() > 5*1048576 {
		return fmt.Errorf("file too large (> 5mb)")
	}

	destFileName := filepath.Join(folderPath, clusterName)
	destFile, err := os.Create(filepath.Clean(destFileName)) // creates if file doesn't exist
	if err != nil {
		return fmt.Errorf("failed to create file: %s, error: %s", destFileName, err.Error())
	}
	defer func() {
		if err := destFile.Close(); err != nil {
			log.Printf("error encountered in closing destination file. Error: %s", err.Error())
		}
	}()

	bytes, err := io.Copy(destFile, srcFile) // check first var for number of bytes copied
	if err != nil {
		return err
	}
	err = destFile.Sync()
	if err != nil {
		return err
	}

	log.Printf("Successfully created %s in %s folder. %d bytes copied",
		clusterName, folderPath, bytes)

	return nil
}

func getClustersFolderPath(path string) (string, error) {
	curUser, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("can't get User home dir: %s", err.Error())
	}
	curUser = filepath.Join(curUser, path)

	curUserPath, err := filepath.Abs(curUser)
	if err != nil {
		return "", fmt.Errorf("can't get User home group: %s", err.Error())
	}

	err = os.MkdirAll(curUserPath, 0750)
	if err != nil {
		return "", fmt.Errorf("can't generate clusters folder: %s", err.Error())
	}
	return curUserPath, nil
}

func generateConfigsFromSA(mc *k8s.MultiClusterConfigurator, clusterIDs []string) ([]string, error) {
	var res []string

	log.Print("Generating config maps from existing service accounts")
	namespace := "dell-replication-controller"

	err := os.MkdirAll("/tmp/repctl", 0750)
	if err != nil {
		return nil, fmt.Errorf("failed to create tmp dir: %s", err.Error())
	}

	// Read and store script
	fileData, err := f.ReadFile("scripts/gen_kubeconfig.sh")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded script: %s", err.Error())
	}

	err = os.WriteFile("/tmp/repctl/gen_kubeconfig.sh", fileData, 0600) // #nosec G303
	if err != nil {
		return nil, fmt.Errorf("failed to write embedded script: %s", err.Error())
	}

	// Read and store config template
	fileData, err = f.ReadFile("scripts/config-placeholder")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded config template: %s", err.Error())
	}

	err = os.WriteFile("/tmp/repctl/config-placeholder", fileData, 0600) // #nosec G303
	if err != nil {
		return nil, fmt.Errorf("failed to write embedded config template: %s", err.Error())
	}

	configFolder, err := getClustersFolderPath("/.repctl/clusters/")
	if err != nil {
		return nil, fmt.Errorf("error getting clusters folder path: %s", err.Error())
	}

	clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
	if err != nil {
		return nil, fmt.Errorf("error in initializing cluster info: %s", err.Error())
	}

	for _, cluster := range clusters.Clusters {
		log.Printf("Generating config for service account in %s cluster", cluster.GetID())
		_, err := cluster.GetNamespace(context.Background(), namespace)
		if err != nil {
			log.Errorf("cluster inject: %s",
				fmt.Errorf("can't find namespace %s in cluster %s, skipping", namespace, cluster.GetID()))
			continue
		}
		cfgPath := fmt.Sprintf("/tmp/repctl/%s", cluster.GetID())

		// #nosec G204
		c := exec.Command("/bin/bash", "/tmp/repctl/gen_kubeconfig.sh", "-s", "dell-replication-controller-sa", "-n", namespace, "-o", cfgPath)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		err = os.Setenv("KUBECONFIG", cluster.GetKubeConfigFile())
		if err != nil {
			return nil, err
		}

		err = c.Run()
		if err != nil {
			log.Errorf("cluster inject: %s",
				fmt.Errorf("failed to get kubeconfig from service account %s", err.Error()))
			continue
		}

		res = append(res, cfgPath)
		log.Print()
	}

	return res, nil
}

// addKeyFromLiteralToSecret adds the given key and data to the given secret,
// returning an error if the key is not valid or if the key already exists.
func addKeyFromLiteralToSecret(secret *corev1.Secret, keyName string, data []byte) error {
	if errs := validation.IsConfigMapKey(keyName); len(errs) != 0 {
		return fmt.Errorf("%q is not valid key name for a Secret %s", keyName, strings.Join(errs, ";"))
	}
	if _, entryExists := secret.Data[keyName]; entryExists {
		return fmt.Errorf("cannot add key %s, another key by that name already exists", keyName)
	}
	secret.Data[keyName] = data

	return nil
}
