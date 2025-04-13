/*
 Copyright © 2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"context"

	repv1 "github.com/dell/csm-replication/api/v1"
	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/cmd/mocks"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiTypes "k8s.io/apimachinery/pkg/types"
)

type MigrateTestSuite struct {
	suite.Suite
	testDataFolder string
}

func (suite *MigrateTestSuite) SetupSuite() {
	curUser, err := os.UserHomeDir()
	suite.NoError(err)

	curUser = filepath.Join(curUser, folderPath)
	curUserPath, err := filepath.Abs(curUser)
	suite.NoError(err)

	suite.testDataFolder = curUserPath
}

func (suite *MigrateTestSuite) TestGetMigrateCommand() {
	cmd := GetMigrateCommand()

	// Test the command usage
	suite.Equal("migrate", cmd.Use)
	suite.Equal("migrate storage resource to different SC", cmd.Short)

	// Test the flags
	prefixFlag := cmd.Flags().Lookup("migration-prefix")
	suite.NotNil(prefixFlag)
	suite.Equal("migration-prefix", prefixFlag.Usage)

	subCommands := cmd.Commands()
	suite.NotEmpty(subCommands)
	for _, subCmd := range subCommands {
		suite.NotNil(subCmd)
	}

}

func TestMigrateTestSuite(t *testing.T) {
	suite.Run(t, new(MigrateTestSuite))
}

func TestMigratePVCommand(t *testing.T) {
	toSC := "target-sc"
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		pvName                 string
		pvNamespace            string
		toSC                   string
		targetNS               string
		wantErr                bool
		expectedOutputContains string
	}{
		{
			name: "successful PV migration",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			pvName:                 "test-pv",
			pvNamespace:            "test-ns",
			toSC:                   "target-sc",
			targetNS:               "target-ns",
			wantErr:                false,
			expectedOutputContains: "Successfully updated pv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			persistentVolume := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv",
				},
				Status: v1.PersistentVolumeStatus{
					Phase: v1.VolumeAvailable,
				},
			}

			migratedPV := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv" + "-to-" + toSC,
				},
				Status: v1.PersistentVolumeStatus{
					Phase: v1.VolumeAvailable,
				},
			}

			fake, _ := fake_client.NewFakeClient([]runtime.Object{persistentVolume, migratedPV}, nil)

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "",
					},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			migratePVCmd := migratePVCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			migratePVCmd.Flag("to-sc").Value.Set("target-sc")

			args := []string{"test-pv"}
			migratePVCmd.Run(nil, args)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Contains(t, string(out), tt.expectedOutputContains)
		})
	}
}

func TestMigratePVCCommand(t *testing.T) {
	toSC := "target-sc"
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		pvcName                string
		pvName                 string
		pvcNamespace           string
		toSC                   string
		wantErr                bool
		expectedOutputContains string
	}{
		{
			name: "successful PVC migration",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			pvcName:                "test-pvc",
			pvName:                 "test-pv",
			pvcNamespace:           "test-ns",
			toSC:                   "target-sc",
			wantErr:                false,
			expectedOutputContains: "Successfully updated pv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			persistentVolumeClaim := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.pvcName,
					Namespace: tt.pvcNamespace,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					VolumeName: tt.pvName,
				},
			}

			persistentVolume := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv",
				},
				Status: v1.PersistentVolumeStatus{
					Phase: v1.VolumeAvailable,
				},
			}

			migratedPV := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv" + "-to-" + toSC,
				},
				Status: v1.PersistentVolumeStatus{
					Phase: v1.VolumeAvailable,
				},
			}

			fake, _ := fake_client.NewFakeClient([]runtime.Object{persistentVolumeClaim, persistentVolume, migratedPV}, nil)

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "",
					},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)
			foundPV := &v1.PersistentVolume{}
			err := fake.Get(context.TODO(), apiTypes.NamespacedName{Name: "test-pv"}, foundPV)
			if err != nil {
				fmt.Println("error:" + err.Error())
			}

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			migratePVCCmd := migratePVCCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			migratePVCCmd.Flag("to-sc").Value.Set("target-sc")
			migratePVCCmd.Flag("namespace").Value.Set("test-ns")
			args := []string{"test-pvc"}
			migratePVCCmd.Run(nil, args)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Contains(t, string(out), tt.expectedOutputContains)
		})
	}
}

func TestMigrateSTSCommand(t *testing.T) {
	toSC := "target-sc"
	fromSC := "sc1"
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		stsName                string
		podName                string
		ns                     string
		pvcName                string
		pvName                 string
		toSC                   string
		wantErr                bool
		expectedOutputContains string
	}{
		{
			name: "successful STS migration",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			stsName:                "test-sts",
			podName:                "test-pod",
			ns:                     "test-ns",
			pvcName:                "test-pvc",
			pvName:                 "test-pv",
			toSC:                   "target-sc",
			wantErr:                false,
			expectedOutputContains: "Successfully updated pv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			persistentVolumeClaim := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.pvcName,
					Namespace: tt.ns,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					VolumeName:       tt.pvName,
					StorageClassName: &fromSC,
				},
			}

			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.stsName,
					Namespace: tt.ns,
				},
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						*persistentVolumeClaim,
					},
				},
			}

			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.podName,
					Namespace: tt.ns,
					OwnerReferences: []metav1.OwnerReference{
						metav1.OwnerReference{
							Name: tt.stsName,
							Kind: "StatefulSet",
						},
					},
				},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						v1.Volume{
							Name: "test-pv",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: tt.pvcName,
								},
							},
						},
					},
				},
			}

			persistentVolume := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv",
				},
				Status: v1.PersistentVolumeStatus{
					Phase: v1.VolumeBound,
				},
			}

			migratedPV := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv" + "-to-" + toSC,
				},
				Status: v1.PersistentVolumeStatus{
					Phase: v1.VolumeBound,
				},
			}

			fake, _ := fake_client.NewFakeClient([]runtime.Object{persistentVolumeClaim, sts, pod, persistentVolume, migratedPV}, nil)

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "",
					},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			migrateSTSCmd := migrateSTSCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			migrateSTSCmd.Flag("to-sc").Value.Set("target-sc")
			migrateSTSCmd.Flag("namespace").Value.Set("test-ns")
			args := []string{"test-sts"}
			migrateSTSCmd.Run(nil, args)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Contains(t, string(out), tt.expectedOutputContains)
		})
	}
}

func TestMigrateMGCommand(t *testing.T) {
	tests := []struct {
		name                   string
		getClustersFolderPath  func(string) (string, error)
		mgName                 string
		wantErr                bool
		expectedOutputContains string
	}{
		{
			name: "successful array migration",
			getClustersFolderPath: func(path string) (string, error) {
				return clusterPath, nil
			},
			mgName:                 "test-mg",
			wantErr:                false,
			expectedOutputContains: "Successfully migrated all volumes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGetClustersFolderPathFunction := getClustersFolderPathFunction
			defer func() {
				getClustersFolderPathFunction = originalGetClustersFolderPathFunction
			}()

			getClustersFolderPathFunction = tt.getClustersFolderPath

			migrationGroup := &repv1.DellCSIMigrationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.mgName,
				},
				Status: repv1.DellCSIMigrationGroupStatus{
					State: "Committed",
				},
			}

			scheme := runtime.NewScheme()
			_ = repv1.AddToScheme(scheme)

			fake, _ := fake_client.NewFakeClient([]runtime.Object{migrationGroup}, nil)

			mockClusters := &k8s.Clusters{
				Clusters: []k8s.ClusterInterface{
					&k8s.Cluster{
						ClusterID: "",
					},
				},
			}
			mockClusters.Clusters[0].SetClient(fake)

			getClustersMock := mocks.NewMockGetClustersInterface(gomock.NewController(t))
			getClustersMock.EXPECT().GetAllClusters(gomock.Any(), gomock.Any()).Times(1).Return(mockClusters, nil)

			migrateMGCmd := migrateMGCommand(getClustersMock)

			rescueStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = rescueStdout
			}()

			args := []string{tt.mgName}
			migrateMGCmd.Run(nil, args)

			w.Close()
			out, _ := io.ReadAll(r)
			os.Stdout = rescueStdout

			assert.Contains(t, string(out), tt.expectedOutputContains)
		})
	}
}
