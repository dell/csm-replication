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
	"io"
	"os"
	"path/filepath"
	"testing"

	fake_client "github.com/dell/csm-replication/test/e2e-framework/fake-client"
	"github.com/dell/repctl/pkg/cmd/mocks"
	"github.com/dell/repctl/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
		targetNS               string
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
