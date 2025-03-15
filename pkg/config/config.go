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

package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/record"
	certutil "k8s.io/client-go/util/cert"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ControllerManagerOpts - Controller Manager configuration
type ControllerManagerOpts struct {
	UseConfFileFormat bool
	WatchNamespace    string
	ConfigDir         string
	ConfigFileName    string
	InCluster         bool
	Mode              string
}

var isInInvalidState bool

// GetControllerManagerOpts initializes and returns new ControllerManagerOpts object
func GetControllerManagerOpts() ControllerManagerOpts {
	defaultNameSpace := getEnv(common.EnvWatchNameSpace, common.DefaultNameSpace)
	configFile := getEnv(common.EnvConfigFileName, common.DefaultConfigFileName)
	configDir := getEnv(common.EnvConfigDirName, common.DefaultConfigDir)
	inClusterEnvVal := getEnv(common.EnvInClusterConfig, "false")
	inCluster := false
	if strings.ToLower(inClusterEnvVal) == "true" {
		inCluster = true
	}
	useConfFileFormatEnvVal := getEnv(common.EnvUseConfFileFormat, "true")
	useConfFileFormat := true
	if strings.ToLower(useConfFileFormatEnvVal) == "false" {
		useConfFileFormat = false
	}
	return ControllerManagerOpts{
		WatchNamespace:    defaultNameSpace,
		ConfigFileName:    configFile,
		ConfigDir:         configDir,
		InCluster:         inCluster,
		UseConfFileFormat: useConfFileFormat,
	}
}

// target - target cluster information
type target struct {
	ClusterID string `yaml:"clusterId"`
	SecretRef string `yaml:"secretRef"`
	Address   string `yaml:"address"`
}

// replicationConfigMap - represents the configuration file
type replicationConfigMap struct {
	ClusterID string   `yaml:"clusterId"`
	Targets   []target `yaml:"targets"`
	LogLevel  string   `yaml:"CSI_LOG_LEVEL"`
}

// replicationConfig - represents the configuration of Replication (formed using replicationConfigMap)
type replicationConfig struct {
	ClusterID string   `yaml:"clusterId"`
	Targets   []target `yaml:"targets"`
	connection.ConnHandler
}

// Config structure that combines replication configuration and current log level
type Config struct {
	repConfig *replicationConfig
	LogLevel  string
	Lock      sync.Mutex
}

// UpdateConfigOnSecretEvent updates config instance if update to currently used secret was made
func (c *Config) UpdateConfigOnSecretEvent(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, secretName string, recorder record.EventRecorder, log logr.Logger) error {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	// First check if we are interested in this secret
	found := false
	for _, target := range c.repConfig.Targets {
		if secretName == target.SecretRef {
			found = true
			log.V(common.DebugLevel).Info(fmt.Sprintf("Received event for secret: %s configured for ClusterId: %s\n", secretName, target.ClusterID))
			break
		}
	}
	if found {
		// This secret is relevant to us
		// Lets update the entire config
		err := c.updateConfig(ctx, client, opts, recorder, log)
		return err
	}
	log.V(common.DebugLevel).Info("Ignoring event for secret as it is not related to us")
	return nil
}

// UpdateConfigMap updates config instance by reading mounted config
func (c *Config) UpdateConfigMap(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, recorder record.EventRecorder, log logr.Logger) error {
	c.Lock.Lock()
	defer c.Lock.Unlock()

	err := c.updateConfig(ctx, client, opts, recorder, log)
	return err
}

func (c *Config) updateConfig(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, recorder record.EventRecorder, log logr.Logger) error {
	cmap, replicationConfig, err := getReplicationConfig(ctx, client, opts, recorder, log)
	if err != nil {
		return err
	}
	c.repConfig = replicationConfig
	c.LogLevel = cmap.LogLevel
	log.V(common.InfoLevel).Info("Updated config")
	return nil
}

var (
	GetConnection = func(c *Config, clusterID string) (connection.RemoteClusterClient, error) {
		return c.repConfig.GetConnection(clusterID)
	}
)

// GetConnection returns cluster client for given cluster ID
func (c *Config) GetConnection(clusterID string) (connection.RemoteClusterClient, error) {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	return GetConnection(c, clusterID)
}

// GetClusterID returns cluster ID for config instance
func (c *Config) GetClusterID() string {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	return c.repConfig.ClusterID
}

// PrintConfig prints current config information using provided logger interface
func (c *Config) PrintConfig(log logr.Logger) {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	c.repConfig.Print(log)
}

// GetConfig returns new instance of replication config
func GetConfig(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, recorder record.EventRecorder, log logr.Logger) (*Config, error) {
	cmap, repConfig, err := getReplicationConfig(ctx, client, opts, recorder, log)
	if err != nil {
		return nil, err
	}
	return &Config{
		repConfig: repConfig,
		LogLevel:  cmap.LogLevel,
	}, nil
}

// Print prints current config information using provided logger interface
func (config *replicationConfig) Print(log logr.Logger) {
	log.Info(fmt.Sprintf("Source ClusterId: %s", config.ClusterID))
	for _, target := range config.Targets {
		log.Info(fmt.Sprintf("ClusterId: %s, Secret Ref: %s", target.ClusterID, target.SecretRef))
	}
}

var (
	Verify = func(config *replicationConfig, ctx context.Context) error {
		return config.Verify(ctx)
	}
)

// VerifyConfig verifies correctness of replication config
func (config *replicationConfig) VerifyConfig(ctx context.Context) error {
	if config.ClusterID == "" {
		return fmt.Errorf("missing source cluster id")
	}
	targetMap := make(map[string]string)
	for _, target := range config.Targets {
		if target.ClusterID == "" {
			return fmt.Errorf("ClusterId missing for target: %v", target)
		}
		if _, ok := targetMap[target.ClusterID]; ok {
			return fmt.Errorf("detected duplicate entries for ClusterId - %s", target.ClusterID)
		}
		targetMap[target.ClusterID] = ""
	}
	// err := config.Verify(ctx)
	err := Verify(config, ctx)
	return err
}

// SetConnectionHandler sets connection handler of replication config to provided handler
func (config *replicationConfig) SetConnectionHandler(handler connection.ConnHandler) {
	config.ConnHandler = handler
}

// readConfigFile - uses viper to read the config from the config map
func readConfigFile(configFile, configPath string) (*replicationConfigMap, error) {
	viper.New()
	viper.SetConfigName(configFile)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configPath)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, err
	}
	var configMap replicationConfigMap
	err = viper.Unmarshal(&configMap)
	if err != nil {
		return nil, err
	}
	configMap.LogLevel = viper.GetString("CSI_LOG_LEVEL")
	return &configMap, nil
}

func getReplicationConfig(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, recorder record.EventRecorder, log logr.Logger) (*replicationConfigMap, *replicationConfig, error) {
	configMap, err := readConfigFile(opts.ConfigFileName, opts.ConfigDir)
	if err != nil {
		return nil, nil, err
	}
	if client != nil {
		connHandler, err := getConnHandler(ctx, configMap.Targets, client, opts, log)
		if err != nil {
			return nil, nil, err
		}

		repConfig := newReplicationConfig(configMap, connHandler)
		err = repConfig.VerifyConfig(ctx)
		if err != nil && opts.Mode == "controller" {
			log.V(common.InfoLevel).Info("Wrong config, publishing event. ", err.Error(), isInInvalidState)
			err := controllers.PublishControllerEvent(ctx, client, recorder, "Warning", "Invalid", "Config update won't be applied because of invalid configmap/secrets. Please fix the invalid configuration.")
			isInInvalidState = true
			if err != nil {
				return nil, nil, err
			}
		} else {
			if isInInvalidState == true && opts.Mode == "controller" {

				log.V(common.InfoLevel).Info("Correct config, publishing event. ")
				err := controllers.PublishControllerEvent(ctx, client, recorder, "Normal", "Correct config applied", "Correct configuration has been applied to cluster.")
				isInInvalidState = false
				if err != nil {
					log.V(common.InfoLevel).Info(err.Error())
					return nil, nil, err
				}
			}
		}
		return configMap, repConfig, nil
	}
	return configMap, nil, nil
}

var (
	InClusterConfig = func() (*rest.Config, error) {
		return rest.InClusterConfig()
	}
)

// Returns a connection handler for the remote clusters
// Currently only returns the k8s conn handler
func getConnHandler(ctx context.Context, targets []target, client ctrlClient.Client, opts ControllerManagerOpts, log logr.Logger) (connection.ConnHandler, error) {
	var k8sConnHandler connection.RemoteK8sConnHandler
	var restConfig *rest.Config
	var err error
	for _, target := range targets {
		if opts.UseConfFileFormat {
			log.V(common.DebugLevel).Info("Expecting secret data to be in format of conf file")
			restConfig, err = buildRestConfigFromSecretConfFileFormat(ctx, target.SecretRef, opts.WatchNamespace, client)
			if err != nil {
				return nil, err
			}
		} else {
			log.V(common.InfoLevel).Info("Expecting secret data to be in form of a service account token/custom format")
			// restConfig, err = buildRestConfigFromCustomFormat(target.SecretRef, opts.WatchNamespace, client)
			restConfig, err = buildRestConfigFromServiceAccountToken(ctx, target.SecretRef, opts.WatchNamespace, client, target.Address)
			if err != nil {
				return nil, err
			}
		}
		k8sConnHandler.AddOrUpdateConfig(target.ClusterID, restConfig, log)
	}
	// Let's add a connection handler by default for self (single cluster scenario)
	inCluster, _ := strconv.ParseBool(getEnv(common.EnvInClusterConfig, "false"))

	if inCluster {
		restConfig, err = InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		var kubeconfig string
		if path := getKubeConfigPathFromEnv(); path != "" {
			kubeconfig = filepath.Join(path, ".kube", "config")
		}
		if kubeconfig == "" {
			return nil, fmt.Errorf("failed to get kube config path")
		}
		// use the current context in kubeconfig
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	k8sConnHandler.AddOrUpdateConfig(controllers.Self, restConfig, log)

	return &k8sConnHandler, nil
}

func getKubeConfigPathFromEnv() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("X_CSI_KUBECONFIG_PATH") // user specified path
}

// newReplicationConfig - returns a new replication config given a config map & a connection handler
func newReplicationConfig(configMap *replicationConfigMap, handler connection.ConnHandler) *replicationConfig {
	var replicationConfig replicationConfig
	replicationConfig.ClusterID = configMap.ClusterID
	targets := make([]target, len(configMap.Targets))
	copy(targets, configMap.Targets)
	replicationConfig.Targets = targets
	replicationConfig.SetConnectionHandler(handler)
	return &replicationConfig
}

func getEnv(envName, defaultValue string) string {
	envVal, found := os.LookupEnv(envName)
	if !found {
		envVal = defaultValue
	}
	return envVal
}

// buildRestConfigFromCustomFormat - Helper method to create REST config if a full conf file format is not required
// although not in use, this can be used in future based on user feedback
func buildRestConfigFromCustomFormat(ctx context.Context, secretName string, namespace string, client ctrlClient.Client) (*rest.Config, error) {
	secret := new(v1.Secret)
	err := client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
	if err != nil {
		return nil, err
	}
	server := string(secret.Data["server"])
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{
		ClusterDefaults: api.Cluster{
			CertificateAuthorityData: secret.Data["ca"],
			Server:                   server,
		},
		AuthInfo: api.AuthInfo{
			ClientKeyData:         secret.Data["clientkey"],
			ClientCertificateData: secret.Data["clientcert"],
		},
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	return kubeConfig.ClientConfig()
}

// buildRestConfigFromSecretConfFileFormat - Builds a REST config from a secret created using a kubeconfig file
func buildRestConfigFromSecretConfFileFormat(ctx context.Context, secretName string, namespace string, client ctrlClient.Client) (*rest.Config, error) {
	secret := new(v1.Secret)
	err := client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
	if err != nil {
		return nil, err
	}
	return clientcmd.RESTConfigFromKubeConfig(secret.Data["data"])
}

// buildRestConfigFromServiceAccountToken - Builds REST config from a secret created using a service account token secret
func buildRestConfigFromServiceAccountToken(ctx context.Context, secretName string, namespace string, client ctrlClient.Client, host string) (*rest.Config, error) {
	secret := new(v1.Secret)
	err := client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
	if err != nil {
		return nil, err
	}
	tlsClientConfig := rest.TLSClientConfig{}
	if _, err := certutil.NewPoolFromBytes(secret.Data["ca.crt"]); err != nil {

		return nil, err
	}
	tlsClientConfig.CAData = secret.Data["ca.crt"]
	tlsClientConfig.KeyData = secret.Data["token"]
	return &rest.Config{
		Host:            "https://" + host,
		TLSClientConfig: tlsClientConfig,
		BearerToken:     string(secret.Data["token"]),
	}, nil
}
