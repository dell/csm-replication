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

package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/repctl/pkg/display"
	"github.com/dell/repctl/pkg/metadata"
	"github.com/dell/repctl/pkg/types"
	"github.com/dell/repctl/pkg/utils"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiExtensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	apiTypes "k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Clusters represents all clusters currently managed by `repctl`
type Clusters struct {
	Clusters []ClusterInterface
}

var (
	displayNewTableWriter         = display.NewTableWriter
	clientcmdBuildConfigFromFlags = clientcmd.BuildConfigFromFlags
	kubernetesNewForConfig        = kubernetes.NewForConfig
	GetCtrlRuntimeClient          = func(kubeconfig string) (client.Client, error) {
		return getControllerRuntimeClient(kubeconfig)
	}
	newClntSet = func(kubeconfig string) (*kubernetes.Clientset, *rest.Config, error) {
		return newClientSet(kubeconfig)
	}
	getServiceVersion = func(clientset *kubernetes.Clientset) (*version.Info, error) {
		return clientset.Discovery().ServerVersion()
	}
)

// Print prints all currently managed clusters to stdout in form a table
func (c *Clusters) Print() {
	// Form an empty object and create a new table writer
	t, err := displayNewTableWriter(Cluster{}, os.Stdout)
	if err != nil {
		return
	}
	t.PrintHeader()
	for _, obj := range c.Clusters {
		o := obj.(*Cluster)
		t.PrintRow(*o)
	}
	t.Done()
}

// ClientInterface is an interface that wraps around k8s client structure
type ClientInterface interface {
	client.Client
}

// ClusterInterface contains methods for interacting with k8s cluster via k8s client
type ClusterInterface interface {
	GetClient() ClientInterface
	SetClient(ClientInterface)
	GetID() string
	GetKubeVersion() string
	GetHost() string
	GetKubeConfigFile() string

	GetPersistentVolume(context.Context, string) (*v1.PersistentVolume, error)
	ListPersistentVolumes(context.Context, ...client.ListOption) (*v1.PersistentVolumeList, error)
	FilterPersistentVolumes(context.Context, string, string, string, string) ([]types.PersistentVolume, error)
	UpdatePersistentVolume(ctx context.Context, pv *v1.PersistentVolume) error

	GetNamespace(context.Context, string) (*v1.Namespace, error)
	CreateNamespace(context.Context, *v1.Namespace) error

	GetSecret(context.Context, string, string) (*v1.Secret, error)
	UpdateSecret(ctx context.Context, secret *v1.Secret) error

	ListStorageClass(context.Context, ...client.ListOption) (*storagev1.StorageClassList, error)
	FilterStorageClass(context.Context, string, bool) (*types.SCList, error)

	ListPersistentVolumeClaims(context.Context, ...client.ListOption) (*v1.PersistentVolumeClaimList, error)
	FilterPersistentVolumeClaims(context.Context, string, string, string, string) (*types.PersistentVolumeClaimList, error)
	GetPersistentVolumeClaim(context.Context, string, string) (*v1.PersistentVolumeClaim, error)
	DeletePersistentVolumeClaim(ctx context.Context, pvc *v1.PersistentVolumeClaim, opts ...client.DeleteOption) error

	GetReplicationGroups(context.Context, string) (*repv1.DellCSIReplicationGroup, error)
	ListReplicationGroups(context.Context, ...client.ListOption) (*repv1.DellCSIReplicationGroupList, error)
	FilterReplicationGroups(context.Context, string, string) (*types.RGList, error)
	PatchReplicationGroup(context.Context, *repv1.DellCSIReplicationGroup, client.Patch) error
	UpdateReplicationGroup(context.Context, *repv1.DellCSIReplicationGroup) error

	CreatePersistentVolumeClaimsFromPVs(context.Context, string, []types.PersistentVolume, string, bool) error
	CreateObject(context.Context, []byte) (runtime.Object, error)

	GetStatefulSet(context.Context, string, string) (*appsv1.StatefulSet, error)

	FilterPods(ctx context.Context, namespace string, stsName string) (*v1.PodList, error)
	ListPods(ctx context.Context, opts ...client.ListOption) (*v1.PodList, error)
	GetPod(ctx context.Context, name string, namespace string) (*v1.Pod, error)
	DeletePod(ctx context.Context, pod *v1.Pod, opts ...client.DeleteOption) error

	DeleteStsOrphan(ctx context.Context, sts *appsv1.StatefulSet) error
	CreateStatefulSet(ctx context.Context, sts *appsv1.StatefulSet) error

	GetMigrationGroup(context.Context, string) (*repv1.DellCSIMigrationGroup, error)
	UpdateMigrationGroup(context.Context, *repv1.DellCSIMigrationGroup) error
}

// Cluster is implementation of ClusterInterface that represents some cluster
type Cluster struct {
	ClusterID      string `display:"ClusterId"`
	KubeVersion    string `display:"Version"`
	Host           string `display:"URL"`
	kubeConfigFile string
	client         ClientInterface
	restClient     ClientInterface
}

// GetID returns id of the cluster
func (c *Cluster) GetID() string {
	return c.ClusterID
}

// GetKubeVersion returns k8s version of cluster
func (c *Cluster) GetKubeVersion() string {
	return c.KubeVersion
}

// GetHost returns URL of primary node
func (c *Cluster) GetHost() string {
	return c.Host
}

// GetKubeConfigFile returns path to kube config file
func (c *Cluster) GetKubeConfigFile() string {
	return c.kubeConfigFile
}

// GetClient returns ClientInterface that could be used for interaction with current cluster
func (c *Cluster) GetClient() ClientInterface {
	return c.client
}

// SetClient forces cluster to use provided client
func (c *Cluster) SetClient(client ClientInterface) {
	c.client = client
}

// PatchReplicationGroup patches replication group
func (c *Cluster) PatchReplicationGroup(ctx context.Context, rg *repv1.DellCSIReplicationGroup, patch client.Patch) error {
	return c.client.Patch(ctx, rg, patch)
}

// UpdateReplicationGroup updates replication group
func (c *Cluster) UpdateReplicationGroup(ctx context.Context, rg *repv1.DellCSIReplicationGroup) error {
	return c.client.Update(ctx, rg)
}

// GetPersistentVolume returns persistent volume object by querying cluster using persistent volume name
func (c *Cluster) GetPersistentVolume(ctx context.Context, pvName string) (*v1.PersistentVolume, error) {
	found := &v1.PersistentVolume{}
	err := c.client.Get(ctx, apiTypes.NamespacedName{Name: pvName}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// ListPersistentVolumes returns list of all persistent volume objects that are currently in cluster
func (c *Cluster) ListPersistentVolumes(ctx context.Context, opts ...client.ListOption) (*v1.PersistentVolumeList, error) {
	found := &v1.PersistentVolumeList{}
	err := c.client.List(ctx, found, opts...)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// FilterPersistentVolumes returns filtered list of all persistent volume objects that are currently in cluster
func (c *Cluster) FilterPersistentVolumes(ctx context.Context, driver, remoteClusterID, remoteNamespace, rgName string) ([]types.PersistentVolume, error) {
	matchingLabels := make(map[string]string)
	addLabelToLabelMap(matchingLabels, metadata.Driver, driver)
	addLabelToLabelMap(matchingLabels, metadata.RemoteClusterID, remoteClusterID)
	addLabelToLabelMap(matchingLabels, metadata.RemotePVCNamespace, remoteNamespace)
	addLabelToLabelMap(matchingLabels, metadata.ReplicationGroup, rgName)
	persistentVolumes, err := c.ListPersistentVolumes(ctx, client.MatchingLabels(matchingLabels))
	if err != nil {
		return nil, err
	}
	myPVList := make([]types.PersistentVolume, 0)
	for _, persistentVolume := range persistentVolumes.Items {
		persistentVolume := persistentVolume
		pv, err := types.GetPV(&persistentVolume)
		if err != nil {
			continue
		}
		myPVList = append(myPVList, pv)
	}
	return myPVList, nil
}

// GetNamespace returns namespace object by querying cluster using namespace name
func (c *Cluster) GetNamespace(ctx context.Context, nsName string) (*v1.Namespace, error) {
	found := &v1.Namespace{}
	err := c.client.Get(ctx, apiTypes.NamespacedName{Name: nsName}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// CreateNamespace creates new namespace object in cluster
func (c *Cluster) CreateNamespace(ctx context.Context, ns *v1.Namespace) error {
	return c.GetClient().Create(ctx, ns)
}

// GetSecret returns namespace object by querying cluster using namespace name
func (c *Cluster) GetSecret(ctx context.Context, nsName string, sName string) (*v1.Secret, error) {
	found := &v1.Secret{}
	err := c.client.Get(ctx, apiTypes.NamespacedName{Namespace: nsName, Name: sName}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// UpdateSecret updates secret
func (c *Cluster) UpdateSecret(ctx context.Context, secret *v1.Secret) error {
	return c.client.Update(ctx, secret)
}

// UpdatePersistentVolume updates PV
func (c *Cluster) UpdatePersistentVolume(ctx context.Context, pv *v1.PersistentVolume) error {
	return c.client.Update(ctx, pv)
}

// ListStorageClass returns list of all storage class objects that are currently in cluster
func (c *Cluster) ListStorageClass(ctx context.Context, opts ...client.ListOption) (*storagev1.StorageClassList, error) {
	found := &storagev1.StorageClassList{}
	err := c.client.List(ctx, found, opts...)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// FilterStorageClass returns filtered list of all storage class objects that are currently in cluster
func (c *Cluster) FilterStorageClass(ctx context.Context, driverName string, noFilter bool) (*types.SCList, error) {
	mySCList := make([]types.StorageClass, 0)
	scList, err := c.ListStorageClass(ctx)
	if err != nil {
		return nil, err
	}
	for _, sc := range scList.Items {
		params := sc.Parameters
		repEnabled := false
		if enabled, ok := params[metadata.ReplicationEnabled]; ok && enabled == "true" {
			repEnabled = true
		}
		if !noFilter {
			if !repEnabled || (driverName != "" && driverName != sc.Provisioner) {
				continue
			}
		}
		mySCList = append(mySCList, types.GetSC(sc, c.GetID(), repEnabled))
	}
	return &types.SCList{
		SCList: mySCList,
	}, nil
}

// ListPersistentVolumeClaims returns list of all persistent volume claim objects that are currently in cluster
func (c *Cluster) ListPersistentVolumeClaims(ctx context.Context, opts ...client.ListOption) (*v1.PersistentVolumeClaimList, error) {
	found := &v1.PersistentVolumeClaimList{}
	err := c.client.List(ctx, found, opts...)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// GetPersistentVolumeClaim returns pvc by name
func (c *Cluster) GetPersistentVolumeClaim(ctx context.Context, nsName string, pvcName string) (*v1.PersistentVolumeClaim, error) {
	found := &v1.PersistentVolumeClaim{}
	err := c.client.Get(ctx, apiTypes.NamespacedName{Namespace: nsName, Name: pvcName}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// FilterPersistentVolumeClaims returns filtered list of all persistent volume claim objects that are currently in cluster
func (c *Cluster) FilterPersistentVolumeClaims(ctx context.Context, namespace, remoteClusterID,
	remoteNamespace, rgName string,
) (*types.PersistentVolumeClaimList, error) {
	matchingLabels := make(map[string]string)
	if remoteClusterID != "" {
		matchingLabels[metadata.RemoteClusterID] = remoteClusterID
	}
	if remoteNamespace != "" {
		matchingLabels[metadata.RemotePVCNamespace] = remoteNamespace
	}
	if rgName != "" {
		matchingLabels[metadata.ReplicationGroup] = rgName
	}
	pvcList, err := c.ListPersistentVolumeClaims(ctx, client.MatchingLabels(matchingLabels), client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}
	myPVCList := make([]types.PersistentVolumeClaim, 0)
	for _, persistentVolumeClaim := range pvcList.Items {
		pvc := types.GetPVC(persistentVolumeClaim)
		myPVCList = append(myPVCList, pvc)
	}
	return &types.PersistentVolumeClaimList{
		PVCList: myPVCList,
	}, nil
}

// CreatePersistentVolumeClaimsFromPVs uses list of persistent volume to create remote PVCs
func (c *Cluster) CreatePersistentVolumeClaimsFromPVs(ctx context.Context, namespace string,
	pvList []types.PersistentVolume, prefix string, dryRun bool,
) error {
	// go through the PV list and create PVC objects
	for _, pv := range pvList {
		// First check if we have an existing PVC
		// If PVC already exists, we need to skip
		if pv.PVCName != "" {
			continue
		}
		pvcLabels := make(map[string]string, 0)
		pvcAnnotations := make(map[string]string, 0)
		// Iterate through PV labels and apply all replication specific labels
		for key, value := range pv.Labels {
			if strings.Contains(key, prefix) {
				pvcLabels[key] = value
			}
		}
		// Iterate through PV annotations and apply all replication specific annotations
		for key, value := range pv.Annotations {
			if strings.Contains(key, prefix) {
				pvcAnnotations[key] = value
			}
		}
		pvcObj := v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   namespace,
				Name:        pv.RemotePVCName,
				Labels:      pvcLabels,
				Annotations: pvcAnnotations,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				AccessModes: pv.AccessMode,
				Resources: v1.VolumeResourceRequirements{
					Requests: pv.Requests,
				},
				VolumeName: pv.Name,
				//#nosec G601 -- PVs in a RG will all have same SCName
				StorageClassName: &pv.SCName,
				VolumeMode:       pv.VolumeMode,
			},
		}
		var err error
		if dryRun {
			err = c.client.Create(ctx, &pvcObj, client.DryRunAll)
		} else {
			err = c.client.Create(ctx, &pvcObj)
		}
		if err != nil {
			log.Printf("Dry-run: %v. Failed to create PVC for PV: %s. Error: %s\n", dryRun, pv.Name, err.Error())
			return err
		}
		log.Printf("Dry-Run: %v. Successfully created PVC with name: %s using PV: %s in the namespace: %s\n",
			dryRun, pv.RemotePVCName, pv.Name, namespace)
	}
	return nil
}

// CreateObject creates k8s object from yaml file
// Supported objects:
// StorageClass, Namespace, CustomResourceDefinition, ClusterRole, ClusterRoleBinding, Role, RoleBinding, Service, Deployment and ConfigMap
func (c *Cluster) CreateObject(ctx context.Context, data []byte) (runtime.Object, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(repv1.AddToScheme(scheme))
	utilruntime.Must(apiExtensionsv1.AddToScheme(scheme))

	runtimeObj, _, err := serializer.NewCodecFactory(scheme).UniversalDeserializer().Decode(data, nil, nil)
	if err != nil {
		return nil, err
	}

	switch obj := runtimeObj.(type) {
	case *storagev1.StorageClass:
		err := c.client.Create(ctx, obj)
		if err != nil {
			return nil, err
		}
		log.Print("Successfully created storage class: ", obj.Name)
	case *v1.Namespace:
		err := c.client.Create(ctx, obj)
		if err != nil {
			return nil, err
		}
	case *apiExtensionsv1.CustomResourceDefinition:
		err := c.client.Create(ctx, obj)
		if err != nil {
			return nil, err
		}
		log.Print("Successfully created crds: ", obj.Name)
	case *rbacv1.ClusterRole:
		err := c.client.Create(ctx, obj)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				err := c.client.Update(ctx, obj)
				if err != nil {
					return nil, err
				}
				log.Print("Successfully updated existing cluster role: ", obj.Name)
			} else {
				return nil, err
			}
		} else {
			log.Print("Successfully created cluster role: ", obj.Name)
		}
	case *rbacv1.Role:
		crObj, ok := runtimeObj.(*rbacv1.Role)
		if !ok {
			return nil, fmt.Errorf("unsupported object type")
		}
		err := c.client.Create(ctx, crObj)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				err := c.client.Update(ctx, crObj)
				if err != nil {
					return nil, err
				}
				log.Print("Successfully updated existing role: ", crObj.Name)
			} else {
				return nil, err
			}
		} else {
			log.Print("Successfully created role: ", crObj.Name)
		}
	case *rbacv1.ClusterRoleBinding:
		err := c.client.Create(ctx, obj)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				err := c.client.Update(ctx, obj)
				if err != nil {
					return nil, err
				}
				log.Print("Successfully updated existing cluster role binding: ", obj.Name)
			} else {
				return nil, err
			}
		} else {
			log.Print("Successfully created cluster role binding: ", obj.Name)
		}
	case *rbacv1.RoleBinding:
		crbObj, ok := runtimeObj.(*rbacv1.RoleBinding)
		if !ok {
			return nil, fmt.Errorf("unsupported object type")
		}
		err := c.client.Create(ctx, crbObj)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				err := c.client.Update(ctx, crbObj)
				if err != nil {
					return nil, err
				}
				log.Print("Successfully updated existing role binding: ", crbObj.Name)
			} else {
				return nil, err
			}
		} else {
			log.Print("Successfully created role binding: ", crbObj.Name)
		}
	case *v1.Service:
		err := c.client.Create(ctx, obj)
		if err != nil {
			return nil, err
		}
	case *appsv1.Deployment:
		err := c.client.Create(ctx, obj)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				err := c.client.Update(ctx, obj)
				if err != nil {
					return nil, err
				}
				log.Print("Successfully updated existing deployment: ", obj.Name)
			} else {
				return nil, err
			}
		} else {
			log.Print("Successfully created deployment: ", obj.Name)
		}
	case *v1.ConfigMap:
		err := c.client.Create(ctx, obj)
		if err != nil {
			return nil, err
		}
		log.Print("Successfully created config map: ", obj.Name)
	case *v1.ServiceAccount:
		err := c.client.Create(ctx, obj)
		if err != nil {
			return nil, err
		}
		log.Print("Successfully created ServiceAccount: ", obj.Name)
	case *v1.Secret:
		err := c.client.Create(ctx, obj)
		if err != nil {
			return nil, err
		}
		log.Print("Successfully created Secret: ", obj.Name)
	default:
		return nil, fmt.Errorf("unsupported object type %+v", obj.GetObjectKind())
	}

	return runtimeObj, nil
}

// ListReplicationGroups returns list of all replication group objects that are currently in cluster
func (c *Cluster) ListReplicationGroups(ctx context.Context, opts ...client.ListOption) (*repv1.DellCSIReplicationGroupList, error) {
	found := &repv1.DellCSIReplicationGroupList{}
	err := c.client.List(ctx, found, opts...)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// FilterReplicationGroups returns filtered list of all replication group objects that are currently in cluster
func (c *Cluster) FilterReplicationGroups(ctx context.Context, driverName, remoteClusterID string) (*types.RGList, error) {
	matchingLabels := make(map[string]string)
	if remoteClusterID != "" {
		matchingLabels[metadata.RemoteClusterID] = remoteClusterID
	}
	if driverName != "" {
		matchingLabels[metadata.Driver] = driverName
	}
	rgList, err := c.ListReplicationGroups(ctx, client.MatchingLabels(matchingLabels))
	if err != nil {
		return nil, err
	}
	myRGList := make([]types.RG, 0)
	for _, rg := range rgList.Items {
		myRGList = append(myRGList, types.GetRG(rg))
	}
	return &types.RGList{
		RGList: myRGList,
	}, nil
}

func addLabelToLabelMap(labels map[string]string, key, value string) {
	if value != "" {
		labels[key] = value
	}
}

// MultiClusterConfiguratorInterface wraps GetAllClusters method
type MultiClusterConfiguratorInterface interface {
	GetAllClusters([]string, string) (*Clusters, error)
}

// MultiClusterConfigurator is implementation of MultiClusterConfiguratorInterface
type MultiClusterConfigurator struct{}

// GetAllClusters initializes and returns clusters found in configDir
func (*MultiClusterConfigurator) GetAllClusters(clusterIDs []string, configDir string) (*Clusters, error) {
	strictCheck := len(clusterIDs) > 0

	clusters := make([]ClusterInterface, 0)
	items, err := os.ReadDir(configDir)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if !item.IsDir() {
			fileName := item.Name()
			var clusterID, kubeConfigFile string
			clusterID = fileName
			kubeConfigFile = filepath.Join(configDir, fileName)

			if strictCheck {
				if !utils.IsStringInSlice(clusterID, clusterIDs) {
					// skip this cluster id as it was not provided in list of clusterids
					continue
				}
			}
			c, err := CreateCluster(clusterID, kubeConfigFile)
			if err != nil {
				log.Printf("Error encountered in creating kube client for ClusterId: %s. Error: %s\n",
					clusterID, err.Error())
				log.Printf("Output will not include results from ClusterId: %s\n", clusterID)
				continue
			} else {
				clusters = append(clusters, c)
			}
		}
	}
	if len(clusters) == 0 {
		return nil, fmt.Errorf("failed to find any valid config files in %s", configDir)
	}
	return &Clusters{
		Clusters: clusters,
	}, nil
}

func newClientSet(kubeconfig string) (*kubernetes.Clientset, *rest.Config, error) {
	// Create a Config (k8s.io/client-go/rest)
	restConfig, err := clientcmdBuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	clientset, err := kubernetesNewForConfig(restConfig)
	if err != nil {
		return nil, nil, err
	}
	return clientset, restConfig, nil
}

// CreateCluster creates new k8s client for cluster
func CreateCluster(clusterID, kubeconfig string) (ClusterInterface, error) {
	k8sClient, err := GetCtrlRuntimeClient(kubeconfig)
	if err != nil {
		return nil, err
	}
	versionString := ""
	host := ""

	// Create a temporary clientset to get the server version
	// controller runtime client doesnt provide the discovery interface
	clientset, restConfig, err := newClntSet(kubeconfig)
	if err != nil {
		// We can silently ignore this error
	} else {
		host = restConfig.Host
		version, err := getServiceVersion(clientset)
		if err != nil {
			// We can silently ignore this error
		} else {
			versionString = fmt.Sprintf("v%s.%s", version.Major, version.Minor)
		}
	}
	cluster := Cluster{
		ClusterID:      clusterID,
		client:         k8sClient,
		KubeVersion:    versionString,
		kubeConfigFile: kubeconfig,
		Host:           host,
	}
	return &cluster, nil
}

// GetReplicationGroups returns replication group object by querying cluster using replication group name
func (c *Cluster) GetReplicationGroups(ctx context.Context, rgID string) (*repv1.DellCSIReplicationGroup, error) {
	found := &repv1.DellCSIReplicationGroup{}
	err := c.client.Get(ctx, apiTypes.NamespacedName{Name: rgID}, found)
	if err != nil {
		return nil, err
	}
	return found, err
}

// GetStatefulSet returns sts by name
func (c *Cluster) GetStatefulSet(ctx context.Context, nsName string, stsName string) (*appsv1.StatefulSet, error) {
	found := &appsv1.StatefulSet{}
	err := c.client.Get(ctx, apiTypes.NamespacedName{Namespace: nsName, Name: stsName}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// FilterPods returns filtered list of pod, managed by given sts
func (c *Cluster) FilterPods(ctx context.Context, namespace string, stsName string) (*v1.PodList, error) {
	podList, err := c.ListPods(ctx, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}

	myPodList := &v1.PodList{}

	for _, item := range podList.Items {
		for _, reference := range item.OwnerReferences {
			if reference.Name == stsName && reference.Kind == "StatefulSet" {
				myPodList.Items = append(myPodList.Items, item)
			}
		}
	}

	return myPodList, nil
}

// ListPods returns list of all pods in ns
func (c *Cluster) ListPods(ctx context.Context, opts ...client.ListOption) (*v1.PodList, error) {
	found := &v1.PodList{}
	err := c.client.List(ctx, found, opts...)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// DeleteStsOrphan deletes sts with orphan option
func (c *Cluster) DeleteStsOrphan(ctx context.Context, sts *appsv1.StatefulSet) error {
	deletionOrphan := metav1.DeletePropagationOrphan
	err := c.client.Delete(ctx, sts, &client.DeleteOptions{
		PropagationPolicy: &deletionOrphan,
	})
	if err != nil {
		return err
	}
	return nil
}

// DeletePod deletes pod from cluster
func (c *Cluster) DeletePod(ctx context.Context, pod *v1.Pod, opts ...client.DeleteOption) error {
	err := c.client.Delete(ctx, pod, opts...)
	if err != nil {
		return err
	}
	return nil
}

// GetPod returns Pod from the cluster
func (c *Cluster) GetPod(ctx context.Context, name string, namespace string) (*v1.Pod, error) {
	found := &v1.Pod{}
	err := c.client.Get(ctx, apiTypes.NamespacedName{Namespace: namespace, Name: name}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// DeletePersistentVolumeClaim deletes pvc from cluster
func (c *Cluster) DeletePersistentVolumeClaim(ctx context.Context, pvc *v1.PersistentVolumeClaim, opts ...client.DeleteOption) error {
	err := c.client.Delete(ctx, pvc, opts...)
	if err != nil {
		return err
	}
	return nil
}

// CreateStatefulSet creates new sts object in cluster
func (c *Cluster) CreateStatefulSet(ctx context.Context, sts *appsv1.StatefulSet) error {
	return c.GetClient().Create(ctx, sts)
}

// GetMigrationGroup returns migration group object by querying cluster using migration group name
func (c *Cluster) GetMigrationGroup(ctx context.Context, mgName string) (*repv1.DellCSIMigrationGroup, error) {
	found := &repv1.DellCSIMigrationGroup{}
	err := c.client.Get(ctx, apiTypes.NamespacedName{Name: mgName}, found)
	if err != nil {
		return nil, err
	}
	return found, err
}

// UpdateMigrationGroup updates migration group
func (c *Cluster) UpdateMigrationGroup(ctx context.Context, mg *repv1.DellCSIMigrationGroup) error {
	return c.client.Update(ctx, mg)
}
