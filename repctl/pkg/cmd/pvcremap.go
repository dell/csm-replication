package cmd

import (
	"github.com/dell/repctl/pkg/k8s"
	"github.com/dell/repctl/pkg/pvcremap" // Import the pvcremapcode package
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetPvcRemapCommand returns the 'pvcremap' cobra command
func GetPvcRemapCommand() *cobra.Command {
	pvcRemapCmd := &cobra.Command{
		Use:   "pvcremap",
		Short: "Remap PVCs to different PVs",
		Long:  "Remap PVCs to different PVs in the context of replication groups",
		Example: `
./repctl --rg <rg-id> pvcremap --namespace <namespace> --pvc <pvc-name> --target <original|replicated>
`,
		Run: func(cmd *cobra.Command, args []string) {
			rgName := viper.GetString(config.ReplicationGroup)
			namespace := viper.GetString("namespace")
			pvcName := viper.GetString("pvc")
			targetPV := viper.GetString("target")

			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("pvcremap: error getting clusters folder path: %s\n", err.Error())
			}

			cluster, rg, err := GetRGAndClusterFromRGID(configFolder, rgName, "src")
			if err != nil {
				log.Fatalf("pvcremap: error fetching source RG info: (%s)\n", err.Error())
			}

			if err := pvcremap.SwapPVC(cluster, rg, namespace, pvcName, targetPV); err != nil {
				log.Fatalf("pvcremap: error remapping PVC: %s\n", err.Error())
			}

			log.Printf("PVC %s/%s remapped to target %s successfully\n", namespace, pvcName, targetPV)
		},
	}

	pvcRemapCmd.Flags().String("namespace", "", "namespace of the PVC to be remapped")
	_ = viper.BindPFlag("namespace", pvcRemapCmd.Flags().Lookup("namespace"))

	pvcRemapCmd.Flags().String("pvc", "", "name of the PVC to be remapped")
	_ = viper.BindPFlag("pvc", pvcRemapCmd.Flags().Lookup("pvc"))

	pvcRemapCmd.Flags().String("target", "", "target PV ('original' or 'replicated')")
	_ = viper.BindPFlag("target", pvcRemapCmd.Flags().Lookup("target"))

	return pvcRemapCmd
}