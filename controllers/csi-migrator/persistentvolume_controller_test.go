/*
 *
 * Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

/*
 Copyright © 2022-2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/pkg/common/constants"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	csimigration "github.com/dell/csm-replication/test/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	fakeClient := errorFakeCtrlRuntimeClient{
		Client: utils.GetFakeClient(),
	}
	suite.client = fakeClient

	mockMigrationClient := csimigration.NewFakeMigrationClient(utils.ContextPrefix)
	suite.migrationClient = &mockMigrationClient
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
	// positive test
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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
	// pv not added to client
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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
	// specified in pv sc doesn't exist => not found
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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
	// specified in pv sc doesn't fetch
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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
	fakeClient := errorFakeCtrlRuntimeClient{
		Client: utils.GetFakeClientWithObjects(sc1),
		method: "get",
		key:    "sc1",
	}
	suite.client = fakeClient
	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileGetTargetSc() {
	// target sc2 doesn't exist => not found
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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
	// target sc doesn't fetch
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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
	fakeClient := errorFakeCtrlRuntimeClient{
		Client: utils.GetFakeClientWithObjects(sc1, sc2),
		method: "get",
		key:    "sc2",
	}
	suite.client = fakeClient
	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileSameSc() {
	// source sc = target sc
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc1",
				"migration.storage.dell.com/namespace":  "namespace",
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

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileNonReplToRepl() {
	// migration type non repl to repl
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
				"migration.storage.dell.com/namespace":  "namespace",
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
	// migration type repl to non repl
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters: map[string]string{
			"replication.storage.dell.com/isReplicationEnabled": "true",
		},
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileVolumeMigrate() {
	// volume migrate error
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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

	suite.migrationClient.InjectError(fmt.Errorf("error"))

	_, err = suite.reconciler.Reconcile(context.Background(), req)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileNewPvFailedFetch() {
	// new pv doesn't fetch

	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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
	fakeClient := errorFakeCtrlRuntimeClient{
		Client: utils.GetFakeClientWithObjects(sc1, sc2),
		method: "get",
		key:    "pv-to-sc2",
	}
	suite.client = fakeClient
	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileCreatePv() {
	// new pv test
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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

	fakeClient := errorFakeCtrlRuntimeClient{
		Client: utils.GetFakeClientWithObjects(sc1, sc2),
		method: "create",
		key:    "pv-to-sc2",
	}
	suite.client = fakeClient
	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err)
}

func (suite *PersistentVolumeControllerTestSuite) TestPVReconcileUpdate() {
	// update test
	sc1 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc1"},
		Provisioner: "provisionerName",
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "sc2"},
		Provisioner: "provisionerName",
		Parameters:  map[string]string{"test": "test"},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				"migration.storage.dell.com/migrate-to": "sc2",
				"migration.storage.dell.com/namespace":  "namespace",
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

	fakeClient := errorFakeCtrlRuntimeClient{
		Client: utils.GetFakeClientWithObjects(sc1, sc2),
		method: "update",
		key:    "pv",
	}
	suite.client = fakeClient
	suite.reconciler.Client = suite.client

	ctx := context.Background()
	err := suite.client.Create(ctx, pv)
	suite.NoError(err)

	req := suite.getTypicalReconcileRequest(pv.Name)
	_, err = suite.reconciler.Reconcile(context.Background(), req)
	suite.Error(err)

	updatedPV := new(corev1.PersistentVolume)
	err = suite.client.Get(ctx, types.NamespacedName{Namespace: "", Name: pv.Name + "-to-" + sc2.Name}, updatedPV)
	suite.NoError(err)
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
	// creating fake PV to use with fake PVC
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

type errorFakeCtrlRuntimeClient struct {
	ctrlruntimeclient.Client
	method string
	key    string
}

func (e errorFakeCtrlRuntimeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	if e.method == "get" && key.Name == e.key {
		return fmt.Errorf("Get method error")
	}
	return e.Client.Get(ctx, key, obj)
}

func (e errorFakeCtrlRuntimeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if e.method == "create" && obj.GetName() == e.key {
		return fmt.Errorf("Create method error")
	}
	return e.Client.Create(ctx, obj, opts...)
}

func (e errorFakeCtrlRuntimeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if e.method == "update" && obj.GetName() == e.key {
		return fmt.Errorf("Update method error")
	}
	return e.Client.Update(ctx, obj, opts...)
}

func (suite *PersistentVolumeControllerTestSuite) TestSetupWithManagerPv() {
	suite.Init()
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second)
	err := suite.reconciler.SetupWithManager(context.Background(), mgr, expRateLimiter, 1)
	suite.Error(err, "Setup should fail when there is no manager")
}

type fakeClientObject struct {
	client.Object
	annotations map[string]string
}

func TestIsMigrationRequested(t *testing.T) {
	originalNewPredicateFuncs := newPredicateFuncs
	originalGetAnnotations := getAnnotations

	defer func() {
		newPredicateFuncs = originalNewPredicateFuncs
		getAnnotations = originalGetAnnotations
	}()

	type testCase struct {
		name        string
		annotations map[string]string
		setupMocks  func()
		expected    bool
	}

	testCases := []testCase{
		{
			name: "MigrationRequested annotation exists",
			annotations: map[string]string{
				controllers.MigrationRequested: "true",
			},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{
						controllers.MigrationRequested: "true",
					}
				}
			},
			expected: true,
		},
		{
			name:        "MigrationRequested annotation does not exist",
			annotations: map[string]string{},
			setupMocks: func() {
				getAnnotations = func(_ client.Object) map[string]string {
					return map[string]string{}
				}
			},
			expected: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			meta := &fakeClientObject{
				annotations: tt.annotations,
			}
			newPredicateFuncs = func(f func(client.Object) bool) predicate.Funcs {
				return predicate.Funcs{
					GenericFunc: func(e event.GenericEvent) bool {
						return f(e.Object)
					},
				}
			}
			predicateFunc := isMigrationRequested()
			result := predicateFunc.Generic(event.GenericEvent{Object: meta})
			assert.Equal(t, tt.expected, result)
		})
	}
}
