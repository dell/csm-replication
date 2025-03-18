/*
 Copyright © 2022-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package csinoderescanner

import (
	"context"
	"testing"
	"time"

	storagev1 "github.com/dell/csm-replication/api/v1"
	controller "github.com/dell/csm-replication/controllers"
	"github.com/dell/csm-replication/test/e2e-framework/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type testCase struct {
	name        string
	mgState     string
	expectErr   bool
	expectedRes reconcile.Result
	createPod   bool
	podLabels   map[string]string
}

func setupTestReconciler() (*NodeRescanReconciler, context.Context) {
	mockClient := utils.GetFakeClient()
	log := zap.New(zap.UseDevMode(true))
	ctx := context.TODO()

	reconciler := &NodeRescanReconciler{
		Client:                     mockClient,
		Log:                        log,
		Scheme:                     &runtime.Scheme{},
		DriverName:                 "powermax",
		NodeName:                   "test-node",
		MaxRetryDurationForActions: 1 * time.Hour,
	}
	return reconciler, ctx
}

func TestWithoutMG(t *testing.T) {
	reconciler, ctx := setupTestReconciler()
	_, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"},
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestSetupWithManager(t *testing.T) {
	reconciler, _ := setupTestReconciler()
	reconciler.MaxRetryDurationForActions = 0
	mgr := manager.Manager(nil)
	expRateLimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second)
	err := reconciler.SetupWithManager(mgr, expRateLimiter, 1)

	if err == nil {
		t.Errorf("Expected setup to fail due to nil manager, but it succeeded")
	}
}

func runTestCases(t *testing.T, tests []testCase) {
	reconciler, ctx := setupTestReconciler()
	log := zap.New(zap.UseDevMode(true))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mg := &storagev1.DellCSIMigrationGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-mg", Namespace: "default"},
				Status:     storagev1.DellCSIMigrationGroupStatus{State: tt.mgState},
			}

			if tt.createPod {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-" + tt.name,
						Namespace: "default",
						Labels:    tt.podLabels,
					},
					Spec: corev1.PodSpec{NodeName: reconciler.NodeName},
				}
				if err := reconciler.Client.Create(ctx, pod); err != nil {
					t.Fatalf("Failed to create pod: %v", err)
				}
			}

			if err := reconciler.Client.Create(ctx, mg); err != nil {
				log.Error(err, "failed to create test object")
			}

			res, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-mg", Namespace: "default"},
			})

			if (err != nil) != tt.expectErr {
				log.Error(err, "failed to create test object")
			}
			if res != tt.expectedRes {
				t.Errorf("Expected result: %v, got: %v", tt.expectedRes, res)
			}
		})
	}
}

func TestNodeRescanReconcilerDeleteState(t *testing.T) {
	tests := []testCase{
		{
			name:        "Deleting State",
			mgState:     DeletingState,
			expectErr:   false,
			expectedRes: reconcile.Result{},
			createPod:   true,
			podLabels:   map[string]string{"app": NodeLabelFilter, controller.NodeReScanned: "yes"},
		},
	}
	runTestCases(t, tests)
}

func TestNodeRescanReconcilerDeleteStateWithoutPod(t *testing.T) {
	tests := []testCase{
		{
			name:        "Deleting State",
			mgState:     DeletingState,
			expectErr:   false,
			expectedRes: reconcile.Result{},
			createPod:   false,
		},
	}
	runTestCases(t, tests)
}

func TestNodeRescanReconcilerMigratedState(t *testing.T) {
	tests := []testCase{
		{
			name:        "Migrated State",
			mgState:     MigratedState,
			expectErr:   false,
			expectedRes: reconcile.Result{},
			createPod:   true,
			podLabels:   map[string]string{"app": NodeLabelFilter},
		},
	}
	runTestCases(t, tests)
}

func TestNodeRescanReconcilerMigratedStateWithoutPod(t *testing.T) {
	tests := []testCase{
		{
			name:        "Migrated State without Pod",
			mgState:     MigratedState,
			expectErr:   false,
			expectedRes: reconcile.Result{},
			createPod:   false,
		},
	}
	runTestCases(t, tests)
}

func TestNodeRescanReconcilerUnknownState(t *testing.T) {
	tests := []testCase{
		{
			name:        "Unknown State",
			mgState:     "Unknown",
			expectErr:   false,
			createPod:   true,
			expectedRes: reconcile.Result{},
		},
	}
	runTestCases(t, tests)
}
