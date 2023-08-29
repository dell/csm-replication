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
	"bufio"
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//go:embed templates/*
//go:embed scripts/gen_kubeconfig.sh scripts/config-placeholder
var f embed.FS

// Mirrored is a struct that contains parameters for both src and dst systems
type Mirrored struct {
	Source string
	Target string
}

// GlobalParameters is a struct that contains different replication configuration parameters for all supported drivers
type GlobalParameters struct {
	// PowerStore
	ArrayID           Mirrored
	RemoteSystem      Mirrored
	Rpo               string
	IgnoreNamespaces  bool
	VolumeGroupPrefix string

	// PowerMax
	Srp          Mirrored
	RdfMode      string
	SymID        Mirrored
	ServiceLevel Mirrored
	RdfGroup     Mirrored

	// PowerScale
	ClusterName       Mirrored
	AccessZone        Mirrored
	AzServiceIP       Mirrored
	IsiPath           string
	RootClientEnabled Mirrored

	// PowerFlex
	ConsistencyGroupName string
	ProtectionDomain     Mirrored
	StoragePool          Mirrored
}

// RemoteRetentionPolicy structure that contains values for remoteRetentionPolicy for both PV and RGs
type RemoteRetentionPolicy struct {
	RG string
	PV string
}

// ScConfig is a struct that represents config used for storage class creation
type ScConfig struct {
	Name                  string
	Driver                string
	ReclaimPolicy         string
	TargetClusterID       string
	SourceClusterID       string
	ReplicationPrefix     string
	RemoteRetentionPolicy RemoteRetentionPolicy
	Parameters            GlobalParameters
}

// GetCreateCommand returns 'create' cobra command
/* #nosec G104 */
func GetCreateCommand() *cobra.Command {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "create object in specified clusters managed by repctl",
		Example: `
./repctl create -f <path-to-file>
cat <path-to-file> | ./repctl create -f -
		`,
		Run: func(cmd *cobra.Command, args []string) {
			var data [][]byte
			var err error
			fileName := viper.GetString("create-file")

			if fileName == "-" {
				// input is from STDIN
				data, err = processCreateCmd(os.Stdin)
			} else {
				// input from an actual file
				file, fileErr := os.Open(filepath.Clean(fileName))
				if fileErr != nil {
					log.Fatalf("create: error opening file: %s", fileErr.Error())
				}

				data, err = processCreateCmd(file)
			}

			if err != nil {
				log.Fatalf("create: error while parsing input file: %s", err.Error())
			}

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("cluster list: error getting clusters folder path: %s", err.Error())
			}

			clusterIDs := viper.GetStringSlice(config.Clusters)

			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
			if err != nil {
				log.Fatalf("list pv: error in initializing cluster info: %s", err.Error())
			}

			for _, cluster := range clusters.Clusters {
				log.Printf("Creating objects in %s", cluster.GetID())
				for _, resource := range data {
					_, err := cluster.CreateObject(context.Background(), resource)
					if err != nil {
						if strings.Contains(err.Error(), "already exists") {
							log.Warnf("Object already exists: %s", err.Error())
						} else {
							log.Errorf("Encountered error during creating object. Error: %s",
								err.Error())
						}
						continue
					}
				}
			}
		},
	}

	createCmd.Flags().StringP("file", "f", "", "filename")
	_ = createCmd.MarkFlagRequired("file")
	_ = viper.BindPFlag("create-file", createCmd.Flags().Lookup("file"))

	createCmd.AddCommand(getCreatePersistentVolumeClaimsCommand())
	createCmd.AddCommand(getCreateStorageClassCommand())

	return createCmd
}

/* CLI COMMANDS */

/* #nosec G104 */
func getCreatePersistentVolumeClaimsCommand() *cobra.Command {
	createPVCCmd := &cobra.Command{
		Use:   "pvc",
		Short: "allows to create replicated PVCs from existing PVs on target cluster",
		Example: `
./repctl create pvc --rg <rg-name> -t <target-namespace>
./repctl create pvc --rg <rg-name> -t <target-namespace> --dry-run=true
./repctl create pvc --pv <pv-name> -t <target-namespace> --dry-run=true`,
		Long: `
Create PersistentVolumeClaim objects from PersistentVolumes which were created
as part of volume replication.

You can create PersistentVolumeClaim objects for either all PersistentVolumes
associated with a ReplicationGroup or a single PersistentVolume

If you don't encounter any issues in the dry-run, then you can
re-run the command by removing the dry run flag.`,
		Run: func(cmd *cobra.Command, args []string) {
			tgtNamespace := viper.GetString("target-namespace")
			dryRun := viper.GetBool("dry-run")
			rgName := viper.GetString(config.ReplicationGroup)
			prefix := viper.GetString(config.ReplicationPrefix)
			clusterIDs := viper.GetStringSlice(config.Clusters)
			providedPVList := viper.GetStringSlice("pvs")

			if len(clusterIDs) > 1 || len(clusterIDs) == 0 {
				log.Fatalf("create pvc: error please provide single cluster")
			}

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("create pvc: error getting clusters folder path: %s", err.Error())
			}

			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
			if err != nil {
				log.Fatalf("create pvc: error in initializing cluster info: %s", err.Error())
			}

			// Get the first cluster object
			cluster := clusters.Clusters[0]

			err = createPVCs(providedPVList, cluster, rgName, tgtNamespace, prefix, dryRun)
			if err != nil {
				log.Fatalf("create pvc: %s", err.Error())
			}
		},
	}

	createPVCCmd.Flags().StringP("target-namespace", "t", "", "target namespace where PV will be created")
	_ = createPVCCmd.MarkFlagRequired("target-namespace")
	_ = viper.BindPFlag("target-namespace", createPVCCmd.Flags().Lookup("target-namespace"))

	createPVCCmd.Flags().Bool("dry-run", false, "dry run")
	_ = viper.BindPFlag("dry-run", createPVCCmd.Flags().Lookup("dry-run"))

	createPVCCmd.Flags().StringSlice("pv", nil, "pvs to create pvc from")
	_ = viper.BindPFlag("pvs", createPVCCmd.Flags().Lookup("pv"))

	return createPVCCmd
}

/* #nosec G104 */
func getCreateStorageClassCommand() *cobra.Command {
	createSCCmd := &cobra.Command{
		Use:   "sc",
		Short: "Create storage class from config",
		Long:  `Create storage class from config`,
		Example: `
./repctl create sc --from-config <config-file>`,
		Run: func(cmd *cobra.Command, args []string) {
			log.Print("Started generating storage classes")

			clusterIDs := viper.GetStringSlice(config.Clusters)
			dryRun := viper.GetBool("create-sc-dry-run")

			var scConfig ScConfig
			localViper := viper.New()
			localViper.SetConfigType("yaml")
			localViper.SetConfigFile(viper.GetString("from-config"))

			err := localViper.ReadInConfig() // Find and read the config file
			if err != nil {                  // Handle errors reading the config file
				log.Fatalf("create sc: can't find config file: %s", err.Error())
			}

			err = localViper.Unmarshal(&scConfig)
			if err != nil {
				log.Fatalf("create sc: unable to decode sc config: %s", err.Error())
			}

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("create sc: error getting clusters folder path: %s", err.Error())
			}

			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
			if err != nil {
				log.Fatalf("create sc: error in initializing cluster info: %s", err.Error())
			}

			err = createSCs(scConfig, clusters, dryRun)
			if err != nil {
				log.Fatalf("create sc: %s", err.Error())
			}
		},
	}

	createSCCmd.Flags().String("from-config", "", "path to storage class config file")
	_ = viper.BindPFlag("from-config", createSCCmd.Flags().Lookup("from-config"))
	createSCCmd.MarkFlagRequired("from-config") // TODO: required because we don't have interactive option rn

	createSCCmd.Flags().Bool("dry-run", false, "generate storage classes but don't create them")
	_ = viper.BindPFlag("create-sc-dry-run", createSCCmd.Flags().Lookup("dry-run"))

	return createSCCmd
}

/* METHODS */

func createPVCs(providedPVList []string, cluster k8s.ClusterInterface, rgName, tgtNamespace, prefix string, dryRun bool) error {
	var pvList []types.PersistentVolume
	var err error

	if len(providedPVList) != 0 {
		// Get all volumes from cluster
		list, err := cluster.FilterPersistentVolumes(context.Background(), "", "", "", "")
		if err != nil {
			return fmt.Errorf("error while filtering Persistent Volumes: %s", err.Error())
		}
		for _, vol := range list {
			for _, pv := range providedPVList {
				if vol.Name == pv {
					pvList = append(pvList, vol)
				}
			}
		}
	} else {
		pvList, err = cluster.FilterPersistentVolumes(context.Background(), "", "", "", rgName)
		if err != nil {
			return fmt.Errorf("error while filtering Persistent Volumes: %s", err.Error())
		}
	}

	log.Printf("\nCluster: %s\n", cluster.GetID())

	printableList := &types.PersistentVolumeList{PVList: pvList}
	printableList.Print()

	log.Print()
	create := true
	if !dryRun {
		create, err = askForConfirmation("Proceed with creation of PVCs", os.Stdin, 3)
		if err != nil {
			return fmt.Errorf("error encountered while processing user input: %s", err.Error())
		}
	}

	if create {
		log.Print("Creating persistent volume claims")
		err = cluster.CreatePersistentVolumeClaimsFromPVs(context.Background(), tgtNamespace, pvList, prefix, dryRun)
		if err != nil {
			return fmt.Errorf("error encountered while creating PVCs: %s", err.Error())
		}
	}

	return nil
}

func createSCs(scConfig ScConfig, clusters *k8s.Clusters, dryRun bool) error {
	switch scConfig.Driver {
	case "powerstore", "powermax", "isilon", "vxflexos":
		break
	default:
		return fmt.Errorf("driver not supported")
	}

	srcSC, err := getStorageClassObject(fmt.Sprintf("templates/%s_source.yaml", scConfig.Driver), scConfig)
	if err != nil {
		return fmt.Errorf("unable to generate storage class: %s", err.Error())
	}

	tgtSC, err := getStorageClassObject(fmt.Sprintf("templates/%s_target.yaml", scConfig.Driver), scConfig)
	if err != nil {
		return fmt.Errorf("unable to generate storage class: %s", err.Error())
	}

	fmt.Print(string(srcSC))
	fmt.Print(string(tgtSC))

	if dryRun {
		return nil
	}

	log.Print("Creating generated storage classes in clusters")

	for _, cluster := range clusters.Clusters {
		if cluster.GetID() == scConfig.SourceClusterID {
			log.Print("Creating storage class in source cluster")
			_, err := cluster.CreateObject(context.Background(), srcSC)
			if err != nil {
				log.Errorf("Encountered error during creating object. Error: %s\n",
					err.Error())
				continue
			}
		}

		if cluster.GetID() == scConfig.TargetClusterID {
			log.Print("Creating storage class in target cluster")
			_, err := cluster.CreateObject(context.Background(), tgtSC)
			if err != nil {
				log.Errorf("Encountered error during creating object. Error: %s\n",
					err.Error())
				continue
			}
		}
	}

	return nil
}

/* UTILITY */

func processCreateCmd(r io.Reader) ([][]byte, error) {
	var res [][]byte

	// Create scanner and split YAML document using "---" separator
	scanner := bufio.NewScanner(r)
	scanner.Split(splitFuncAt(yamlSeparator))

	for scanner.Scan() {
		res = append(res, []byte(scanner.Text()))
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

func askForConfirmation(s string, in io.Reader, tries int) (bool, error) {
	r := bufio.NewReader(in)
	for ; tries > 0; tries-- {
		log.Printf("%s [y/n]: ", s)
		res, err := r.ReadString('\n')
		if err != nil {
			return false, err
		}
		// Empty input (i.e. "\n")
		if len(res) < 2 {
			continue
		}
		return strings.ToLower(strings.TrimSpace(res))[0] == 'y', nil
	}
	return false, nil
}

func getStorageClassObject(path string, scConfig interface{}) ([]byte, error) {
	srcData, err := f.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't find template file: %s", err.Error())
	}

	srcTmpl, err := template.New("sc").Parse(string(srcData))
	if err != nil {
		return nil, fmt.Errorf("can't parse template: %s", err.Error())
	}

	object := bytes.NewBuffer([]byte{})
	err = srcTmpl.Execute(object, scConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to execute template: %s", err.Error())
	}
	return object.Bytes(), nil
}

const yamlSeparator = "\n---"

// splitFuncAt returns implementation of bufio.SplitFunc that allows to split stream at any custom substring
func splitFuncAt(substring string) func(data []byte, atEOF bool) (advance int, token []byte, err error) {
	searchBytes := []byte(substring)
	searchLen := len(searchBytes)
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		dataLen := len(data)

		// Return nothing if we at the EOF and data is empty
		if atEOF && dataLen == 0 {
			return 0, nil, nil
		}

		// Find next separator and return token
		if i := bytes.Index(data, searchBytes); i >= 0 {
			i += searchLen
			if j := bytes.IndexByte(data[i:], '\n'); j >= 0 {
				return i + j + 1, data[0 : i-searchLen], nil
			}
		}

		// If we're at EOF, we have a final, non-terminated line. Return it.
		if atEOF {
			return dataLen, data, nil
		}

		// Request more data.
		return 0, nil, nil
	}
}
