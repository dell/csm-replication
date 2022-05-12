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

package csimigrator

import (
	"context"
	"github.com/dell/csm-replication/controllers"
	constants "github.com/dell/csm-replication/pkg/common"
	csimigration "github.com/dell/csm-replication/pkg/csi-clients/migration"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"path"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

type PersistentVolumeControllerTestSuite struct {
	suite.Suite
	driver          utils.Driver
	client          client.Client
	migrationClient *csimigration.MockMigration
	reconciler      *PersistentVolumeReconciler
}

func (suite *PersistentVolumeControllerTestSuite) SetupTest() {
	suite.Init()
	suite.initReconciler()
}

func (suite *PersistentVolumeControllerTestSuite) Init() {
	suite.driver = utils.GetDefaultDriver()
	suite.client = utils.GetFakeClient()
	//suite.client = utils.GetFakeClientWithObjects(suite.getFakeStorageClass())

	mockMigrationClient := csimigration.NewFakeMigrationClient(utils.ContextPrefix)
	suite.migrationClient = &mockMigrationClient
	//var sc storagev1.StorageClass
	//err := suite.client.Get(context.Background(), types.NamespacedName{Name: suite.driver.StorageClass}, &sc)
	//suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) initReconciler() {
	fakeRecorder := record.NewFakeRecorder(100)
	// Initialize the annotations & labels
	controllers.InitLabelsAndAnnotations(constants.DefaultMigrationDomain)

	suite.reconciler = &PersistentVolumeReconciler{
		Client:          suite.client,
		Log:             ctrl.Log.WithName("controllers").WithName("PersistentVolumeClaim"),
		Scheme:          utils.Scheme,
		EventRecorder:   fakeRecorder,
		DriverName:      suite.driver.DriverName,
		MigrationClient: suite.migrationClient,
		ContextPrefix:   utils.ContextPrefix,
		Domain:          constants.DefaultMigrationDomain,
		ReplDomain:      constants.DefaultDomain,
	}
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcile() {
	//positive test
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           "provisionerName",
					VolumeHandle:     "volHandle",
					FSType:           "ext4",
					VolumeAttributes: suite.getParams(),
				},
			},
			StorageClassName: sc1.Name,
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}

	suite.client = utils.GetFakeClientWithObjects(sc1, sc2)
	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")

	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: pv.Name + "-to-" + sc2.Name}, updatedPV)
	suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileGetPV() {
	//pv not added to client
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           "provisionerName",
					VolumeHandle:     "volHandle",
					FSType:           "ext4",
					VolumeAttributes: suite.getParams(),
				},
			},
			StorageClassName: "name",
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}

	suite.reconciler.Client = suite.client

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err := suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileScNotFound() {
	//specified in pv sc doesn't exist => not found
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           "provisionerName",
					VolumeHandle:     "volHandle",
					FSType:           "ext4",
					VolumeAttributes: suite.getParams(),
				},
			},
			StorageClassName: "sc1.Name",
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}

	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileScFailedFetch() {
	//specified in pv sc doesn't fetch don't know how
	//sc1 := &storagev1.StorageClass{
	//	ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
	//	Provisioner: "provisionerName",
	//}
	//
	//pv := &corev1.PersistentVolume{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name: "pv",
	//		Annotations: map[string]string{
	//			"migration.storage.dell.com/migrate-to": "sc2",
	//		},
	//	},
	//	Spec: corev1.PersistentVolumeSpec{
	//		PersistentVolumeSource: corev1.PersistentVolumeSource{
	//			CSI: &corev1.CSIPersistentVolumeSource{
	//				Driver:           "provisionerName",
	//				VolumeHandle:     "volHandle",
	//				FSType:           "ext4",
	//				VolumeAttributes: suite.getParams(),
	//			},
	//		},
	//		StorageClassName: sc1.Name,
	//	},
	//	Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	//}
	//
	//suite.client = utils.GetFakeClientWithObjects(sc1)
	//suite.reconciler.Client = suite.client
	//
	//ctx := context.Background()
	//err := suite.client.Create(ctx, pv)
	//suite.NoError(err)
	//
	//req := suite.getTypicalReconcileRequest(pv.Name)
	//_, err = suite.reconciler.Reconcile(context.Background(), req)
	//suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileGetTargetSc() {
	//target sc2 doesn't exist => not found
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           "provisionerName",
					VolumeHandle:     "volHandle",
					FSType:           "ext4",
					VolumeAttributes: suite.getParams(),
				},
			},
			StorageClassName: sc1.Name,
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}

	suite.client = utils.GetFakeClientWithObjects(sc1)
	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileTargetScFailedFetch() {
	//target sc doesn't fetch don't know how
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileSameSc() {
	//sc1 = sc1 doesn't pass second
	//sc1 := &storagev1.StorageClass{
	//	ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
	//	Provisioner: "provisionerName",
	//}
	//
	//sc2 := &storagev1.StorageClass{
	//	ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
	//	Provisioner: "provisionerName2",
	//}
	//
	//pv := &corev1.PersistentVolume{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name: "pv",
	//		Annotations: map[string]string{
	//			"migration.storage.dell.com/migrate-to": "sc1",
	//		},
	//	},
	//	Spec: corev1.PersistentVolumeSpec{
	//		PersistentVolumeSource: corev1.PersistentVolumeSource{
	//			CSI: &corev1.CSIPersistentVolumeSource{
	//				Driver:           "provisionerName",
	//				VolumeHandle:     "volHandle",
	//				FSType:           "ext4",
	//				VolumeAttributes: suite.getParams(),
	//			},
	//		},
	//		StorageClassName: sc1.Name,
	//	},
	//	Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	//}
	//
	//suite.client = utils.GetFakeClientWithObjects(sc1, sc2)
	//suite.reconciler.Client = suite.client
	//
	//ctx := context.Background()
	//err := suite.client.Create(ctx, pv)
	//suite.NoError(err)
	//
	//req := suite.getTypicalReconcileRequest(pv.Name)
	//_, err = suite.reconciler.Reconcile(context.Background(), req)
	//suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileNonReplToRepl() {
	//migration type non repl to repl doesn't work cause domain?
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
		Parameters: map[string]string{
			"replication.storage.dell.com/isReplicationEnabled": "true",
		},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           "provisionerName",
					VolumeHandle:     "volHandle",
					FSType:           "ext4",
					VolumeAttributes: suite.getParams(),
				},
			},
			StorageClassName: sc1.Name,
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}

	suite.client = utils.GetFakeClientWithObjects(sc1, sc2)
	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.NoError(err, "No error on PV reconcile")

	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: pv.Name + "-to-" + sc2.Name}, updatedPV)
	suite.NoError(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileReplToNonRepl() {

}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileVolumeMigrate() {
	//err = envoke how to check?
	//injected error in migration client
}

func (suite *PersistentVolumeControllerTestSuite) getTypicalReconcileRequest(name string) reconcile.Request {
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "",
			Name:      name,
		},
	}
	return req
}

func (suite *PersistentVolumeControllerTestSuite) getParams() map[string]string {
	//creating fake PV to use with fake PVC
	volumeAttributes := map[string]string{
		"param1":                                 "val1",
		"param2":                                 "val2",
		path.Join(utils.ContextPrefix, "param3"): "val3",
	}
	return volumeAttributes
}

func TestPersistentVolumeControllerTestSuite(t *testing.T) {
	testSuite := new(PersistentVolumeControllerTestSuite)
	suite.Run(t, testSuite)
}

func (suite *PersistentVolumeControllerTestSuite) TearDownTest() {
	suite.T().Log("Cleaning up resources...")
}
