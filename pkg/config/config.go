/*
 Copyright © 2021-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"github.com/dell/csm-replication/pkg/common/constants"
	"github.com/dell/csm-replication/pkg/connection"
	"github.com/dell/csmlog"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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
	defaultNameSpace := getEnv(constants.EnvWatchNameSpace, constants.DefaultNameSpace)
	configFile := getEnv(constants.EnvConfigFileName, constants.DefaultConfigFileName)
	configDir := getEnv(constants.EnvConfigDirName, constants.DefaultConfigDir)
	inClusterEnvVal := getEnv(constants.EnvInClusterConfig, "false")
	inCluster := false
	if strings.ToLower(inClusterEnvVal) == "true" {
		inCluster = true
	}
	useConfFileFormatEnvVal := getEnv(constants.EnvUseConfFileFormat, "true")
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
	LogFormat string   `yaml:"CSI_LOG_FORMAT"`
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
	LogFormat string
	Lock      sync.Mutex
}

// UpdateConfigOnSecretEvent updates config instance if update to currently used secret was made
func (c *Config) UpdateConfigOnSecretEvent(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, secretName string, recorder record.EventRecorder) error {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	// First check if we are interested in this secret
	found := false
	for _, target := range c.repConfig.Targets {
		if secretName == target.SecretRef {
			found = true
			csmlog.Infof("Received event for secret: %s configured for ClusterId: %s", secretName, target.ClusterID)
			break
		}
	}
	if found {
		// This secret is relevant to us
		// Lets update the entire config
		err := c.updateConfig(ctx, client, opts, recorder)
		return err
	}
	csmlog.Infof("Ignoring event for secret as it is not related to us")
	return nil
}

// UpdateConfigMap updates config instance by reading mounted config
func (c *Config) UpdateConfigMap(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, recorder record.EventRecorder) error {
	c.Lock.Lock()
	defer c.Lock.Unlock()

	err := c.updateConfig(ctx, client, opts, recorder)
	return err
}

func (c *Config) updateConfig(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, recorder record.EventRecorder) error {
	cmap, replicationConfig, err := getReplicationConfig(ctx, client, opts, recorder)
	if err != nil {
		return err
	}
	c.repConfig = replicationConfig
	c.LogLevel = cmap.LogLevel
	c.LogFormat = cmap.LogFormat
	csmlog.Infof("Updated config")
	return nil
}

var GetConnection = func(c *Config, clusterID string) (connection.RemoteClusterClient, error) {
	return c.repConfig.GetConnection(clusterID)
}

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

// PrintConfig prints current config information.
func (c *Config) PrintConfig() {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	c.repConfig.Print()
}

// GetConfig returns new instance of replication config
func GetConfig(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, recorder record.EventRecorder) (*Config, error) {
	cmap, repConfig, err := getReplicationConfig(ctx, client, opts, recorder)
	if err != nil {
		return nil, err
	}
	return &Config{
		repConfig: repConfig,
		LogLevel:  cmap.LogLevel,
		LogFormat: cmap.LogFormat,
	}, nil
}

// Print prints current config information.
func (config *replicationConfig) Print() {
	csmlog.Infof("Source ClusterId: %s", config.ClusterID)
	for _, target := range config.Targets {
		csmlog.Infof("ClusterId: %s, Secret Ref: %s", target.ClusterID, target.SecretRef)
	}
}

var Verify = func(config *replicationConfig, ctx context.Context) error {
	return config.Verify(ctx)
}

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
	configMap.LogFormat = viper.GetString("CSI_LOG_FORMAT")
	return &configMap, nil
}

func getReplicationConfig(ctx context.Context, client ctrlClient.Client, opts ControllerManagerOpts, recorder record.EventRecorder) (*replicationConfigMap, *replicationConfig, error) {
	configMap, err := readConfigFile(opts.ConfigFileName, opts.ConfigDir)
	if err != nil {
		return nil, nil, err
	}
	if client != nil {
		connHandler, err := getConnHandler(ctx, configMap.Targets, client, opts)
		if err != nil {
			return nil, nil, err
		}

		repConfig := newReplicationConfig(configMap, connHandler)
		err = repConfig.VerifyConfig(ctx)
		if err != nil && opts.Mode == "controller" {
			csmlog.Infof("Wrong config, publishing event. %s", err.Error())
			err := controllers.PublishControllerEvent(ctx, client, recorder, "Warning", "Invalid", "Config update won't be applied because of invalid configmap/secrets. Please fix the invalid configuration.")
			isInInvalidState = true
			if err != nil {
				return nil, nil, err
			}
		} else {
			if isInInvalidState == true && opts.Mode == "controller" {

				csmlog.Infof("Correct config, publishing event")
				err := controllers.PublishControllerEvent(ctx, client, recorder, "Normal", "Correct config applied", "Correct configuration has been applied to cluster.")
				isInInvalidState = false
				if err != nil {
					csmlog.Infof(err.Error())
					return nil, nil, err
				}
			}
		}
		return configMap, repConfig, nil
	}
	return configMap, nil, nil
}

var InClusterConfig = func() (*rest.Config, error) {
	return rest.InClusterConfig()
}

// Returns a connection handler for the remote clusters
// Currently only returns the k8s conn handler
func getConnHandler(ctx context.Context, targets []target, client ctrlClient.Client, opts ControllerManagerOpts) (connection.ConnHandler, error) {
	var k8sConnHandler connection.RemoteK8sConnHandler
	var restConfig *rest.Config
	var err error
	for _, target := range targets {
		if opts.UseConfFileFormat {
			csmlog.Infof("Expecting secret data to be in format of conf file")
			restConfig, err = buildRestConfigFromSecretConfFileFormat(ctx, target.SecretRef, opts.WatchNamespace, client)
			if err != nil {
				return nil, err
			}
		} else {
			csmlog.Infof("Expecting secret data to be in form of a service account token/custom format")
			// restConfig, err = buildRestConfigFromCustomFormat(target.SecretRef, opts.WatchNamespace, client)
			restConfig, err = buildRestConfigFromServiceAccountToken(ctx, target.SecretRef, opts.WatchNamespace, client, target.Address)
			if err != nil {
				return nil, err
			}
		}
		k8sConnHandler.AddOrUpdateConfig(target.ClusterID, restConfig)
	}
	// Let's add a connection handler by default for self (single cluster scenario)
	inCluster, _ := strconv.ParseBool(getEnv(constants.EnvInClusterConfig, "false"))

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
	k8sConnHandler.AddOrUpdateConfig(controllers.Self, restConfig)

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
	return &rest.Config{
		Host: string(secret.Data["server"]),
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   secret.Data["ca"],
			KeyData:  secret.Data["clientkey"],
			CertData: secret.Data["clientcert"],
		},
	}, nil
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
