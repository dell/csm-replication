package cmd

import (
	"context"
	"encoding/json"
	"github.com/dell/repctl/pkg/config"
	"github.com/dell/repctl/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"os/exec"
	"sigs.k8s.io/yaml"
)

// DecodedSecret is a struct that contains Data in string instead bytes
type DecodedSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Immutable         *bool             `json:"immutable,omitempty" protobuf:"varint,5,opt,name=immutable"`
	Data              map[string]string `json:"data,omitempty" protobuf:"bytes,2,rep,name=data"`
	StringData        map[string]string `json:"stringData,omitempty" protobuf:"bytes,4,rep,name=stringData"`
	Type              v1.SecretType     `json:"type,omitempty" protobuf:"bytes,3,opt,name=type,casttype=SecretType"`
}

func (s *DecodedSecret) ToSecret() *v1.Secret {
	m := make(map[string][]byte)
	for k, v := range s.Data {
		m[k] = []byte(v)
	}

	return &v1.Secret{
		TypeMeta:   s.TypeMeta,
		ObjectMeta: s.ObjectMeta,
		Immutable:  s.Immutable,
		Data:       m,
		StringData: s.StringData,
		Type:       s.Type,
	}
}

type Secret struct {
	*v1.Secret
}

func (s *Secret) ToDecodedSecret() *DecodedSecret {
	m := make(map[string]string)
	for k, v := range s.Data {
		m[k] = string(v)
	}

	meta := metav1.TypeMeta{"Secret", "v1"}
	objectMeta := metav1.ObjectMeta{
		Name:        s.ObjectMeta.Name,
		Namespace:   s.ObjectMeta.Namespace,
		Labels:      s.ObjectMeta.Labels,
		Annotations: s.ObjectMeta.Annotations,
	}

	return &DecodedSecret{
		TypeMeta:   meta,
		ObjectMeta: objectMeta,
		Immutable:  s.Immutable,
		Data:       m,
		StringData: s.StringData,
		Type:       s.Type,
	}
}

func GetEditCommand() *cobra.Command {
	editCmd := &cobra.Command{
		Use:   "edit",
		Short: "edit different resources in clusters", //
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				_ = cmd.Help()
				os.Exit(0)
			}
		},
	}

	editCmd.AddCommand(editSecretCommand())

	return editCmd
}

func editSecretCommand() *cobra.Command {
	editSecretCmd := &cobra.Command{
		Use:     "secret",
		Aliases: []string{"sercrets"},
		Short:   "edit secret by repctl",
		Example: `./repctl edit secret --name <name secret> --namespace <namespace>`,
		Run: func(cmd *cobra.Command, args []string) {
			configFolder, err := getClustersFolderPath("/.repctl/clusters/")
			if err != nil {
				log.Fatalf("ed:it secret: error getting clusters folder path: %s", err.Error())
			}

			clusterIDs := viper.GetStringSlice(config.Clusters)

			mc := &k8s.MultiClusterConfigurator{}
			clusters, err := mc.GetAllClusters(clusterIDs, configFolder)
			if err != nil {
				log.Fatalf("edit secret: error in initializing cluster info: %s", err.Error())
			}

			// Get the first cluster object
			cluster := clusters.Clusters[0]

			secretName := args[0]
			secretNamespace := viper.GetString("namespace")
			s, err := cluster.GetSecret(context.Background(), secretNamespace, secretName)
			if err != nil {
				log.Fatalf("edit secret: error in getting secret: %s", err.Error())
			}

			secret := &Secret{s}
			data := objectYAML(secret.ToDecodedSecret())

			tmpFile, err := ioutil.TempFile("", "temp")
			if err != nil {
				log.Fatalf("edit secret: error in creating temp file with secret data: %s", err.Error())
			}

			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write([]byte(data)); err != nil {
				log.Fatalf("edit secret: error in writing temp file with secret data: %s", err.Error())
			}
			if err := tmpFile.Close(); err != nil {
				log.Fatalf("edit secret: error in closing temp file with secret data: %s", err.Error())
			}

			command := exec.Command("vim", tmpFile.Name())
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			command.Stdin = os.Stdin
			err = command.Run()
			if err != nil {
				log.Fatalf("edit secret: error in running text editor: %s", err.Error())
			}

			// extract data from temp file and converting back to secret
			newSecret, err := parseSecret(tmpFile.Name())
			if err != nil {
				log.Fatalf("edit secret: error in parsing data to yaml: %s", err.Error())
			}

			for _, cluster := range clusters.Clusters {
				copiedSecret := newSecret.DeepCopy()
				err = cluster.UpdateSecret(context.Background(), copiedSecret)
				if err != nil {
					log.Fatalf("edit secret: error in updating secret in clusters: %s", err.Error())
				}
				log.Println("updated secret in cluster ", cluster.GetID())
			}
		},
	}

	editSecretCmd.Flags().StringP("namespace", "n", "", "secret namespace")
	_ = editSecretCmd.MarkFlagRequired("namespace")
	_ = viper.BindPFlag("namespace", editSecretCmd.Flags().Lookup("namespace"))

	return editSecretCmd
}

// objectYAML converts object into string
func objectYAML(obj interface{}) string {
	objString := ""
	j, err := json.Marshal(obj)
	if err != nil {
		objString = err.Error()
	} else {
		y, err := yaml.JSONToYAML(j)
		if err != nil {
			objString = err.Error()
		} else {
			objString = string(y)
		}
	}
	return objString
}

// parseSecret parses *one* secret out of a YAML file and returns it
func parseSecret(path string) (*v1.Secret, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var mySecret DecodedSecret
	err = yaml.Unmarshal(content, &mySecret)
	if err != nil {
		return nil, err
	}

	return mySecret.ToSecret(), nil
}