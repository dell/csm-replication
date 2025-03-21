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

package controllers_test

import (
	"os"
	"testing"
	"time"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/dell/csm-replication/controllers"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddAnnotation(t *testing.T) {
	obj := &metav1.ObjectMeta{}
	controllers.AddAnnotation(obj, "key", "value")
	assert.Equal(t, "value", obj.GetAnnotations()["key"])
}

func TestAddLabel(t *testing.T) {
	obj := &metav1.ObjectMeta{}
	controllers.AddLabel(obj, "labelKey", "labelValue")
	assert.Equal(t, "labelValue", obj.GetLabels()["labelKey"])
}

func TestDeleteLabel(t *testing.T) {
	obj := &metav1.ObjectMeta{}
	controllers.AddLabel(obj, "labelKey", "labelValue")
	assert.True(t, controllers.DeleteLabel(obj, "labelKey"))
	assert.Empty(t, obj.GetLabels()["labelKey"])
}

func TestAddFinalizerIfNotExist(t *testing.T) {
	obj := &metav1.ObjectMeta{}
	assert.True(t, controllers.AddFinalizerIfNotExist(obj, "finalizer"))
	assert.False(t, controllers.AddFinalizerIfNotExist(obj, "finalizer"))
}

func TestRemoveFinalizerIfExists(t *testing.T) {
	obj := &metav1.ObjectMeta{}
	controllers.AddFinalizerIfNotExist(obj, "finalizer")
	assert.True(t, controllers.RemoveFinalizerIfExists(obj, "finalizer"))
	assert.False(t, controllers.RemoveFinalizerIfExists(obj, "finalizer"))
}

func TestIsCSIFinalError(t *testing.T) {
	err := status.Error(codes.Aborted, "test error")
	assert.False(t, controllers.IsCSIFinalError(err))

	err = status.Error(codes.InvalidArgument, "final error")
	assert.True(t, controllers.IsCSIFinalError(err))
}

func TestIgnoreIfFinalError(t *testing.T) {
	err := status.Error(codes.Aborted, "test error")
	assert.NotNil(t, controllers.IgnoreIfFinalError(err))

	err = status.Error(codes.InvalidArgument, "final error")
	assert.Nil(t, controllers.IgnoreIfFinalError(err))
}

func TestGetPodNameFromEnv(t *testing.T) {
	os.Setenv(controllers.XCSIReplicationPodName, "test-pod")
	assert.Equal(t, "test-pod", controllers.GetPodNameFromEnv())

	os.Unsetenv(controllers.XCSIReplicationPodName)
	assert.Equal(t, "", controllers.GetPodNameFromEnv())
}

func TestGetPodNamespaceFromEnv(t *testing.T) {
	os.Setenv(controllers.XCSIReplicationPodNamespace, "test-namespace")
	assert.Equal(t, "test-namespace", controllers.GetPodNameSpaceFromEnv())

	os.Unsetenv(controllers.XCSIReplicationPodNamespace)
	assert.Equal(t, "", controllers.GetPodNameSpaceFromEnv())
}

func TestUpdateConditions(t *testing.T) {
	rg := &repv1.DellCSIReplicationGroup{
		Status: repv1.DellCSIReplicationGroupStatus{
			Conditions: []repv1.LastAction{},
		},
	}

	// Define timestamps
	now := metav1.Now()
	firstFailure := metav1.NewTime(time.Now().Add(-time.Hour))

	condition1 := repv1.LastAction{
		Condition:        "InProgress",
		FirstFailure:     &firstFailure,
		Time:             &now,
		ErrorMessage:     "",
		ActionAttributes: map[string]string{"key1": "value1"},
	}

	condition2 := repv1.LastAction{
		Condition:        "Failed",
		FirstFailure:     &now,
		Time:             &now,
		ErrorMessage:     "Error encountered",
		ActionAttributes: map[string]string{"key2": "value2"},
	}

	condition3 := repv1.LastAction{
		Condition:        "Success",
		FirstFailure:     nil,
		Time:             &now,
		ErrorMessage:     "",
		ActionAttributes: map[string]string{"key3": "value3"},
	}

	controllers.UpdateConditions(rg, condition1, 3)
	assert.Equal(t, 1, len(rg.Status.Conditions))
	assert.Equal(t, condition1, rg.Status.Conditions[0])

	controllers.UpdateConditions(rg, condition2, 3)
	assert.Equal(t, 2, len(rg.Status.Conditions))
	assert.Equal(t, condition2, rg.Status.Conditions[0])
	assert.Equal(t, condition1, rg.Status.Conditions[1])

	controllers.UpdateConditions(rg, condition3, 2) // maxConditions = 2
	assert.Equal(t, 2, len(rg.Status.Conditions))
	assert.Equal(t, condition3, rg.Status.Conditions[0])
	assert.Equal(t, condition2, rg.Status.Conditions[1])
}

func TestInitLabelsAndAnnotations(_ *testing.T) {
	controllers.InitLabelsAndAnnotations("test-domain")
}
