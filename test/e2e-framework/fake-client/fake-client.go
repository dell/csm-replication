/*
Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package fakeclient

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	storagev1alpha1 "github.com/dell/csm-replication/api/v1alpha1"
	core_v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type errorInjector interface {
	shouldFail(method string, obj runtime.Object) error
}

type storageKey struct {
	Namespace string
	Name      string
	Kind      string
}

// Client is a fake k8s client
type Client struct {
	Objects       map[storageKey]runtime.Object
	errorInjector errorInjector
}

func getKey(obj runtime.Object) (storageKey, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return storageKey{}, err
	}
	gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
	if err != nil {
		return storageKey{}, err
	}
	return storageKey{
		Name:      accessor.GetName(),
		Namespace: accessor.GetNamespace(),
		Kind:      gvk.Kind,
	}, nil
}

// NewFakeClient initializes and returns new fake k8s client
func NewFakeClient(initialObjects []runtime.Object, errorInjector errorInjector) (*Client, error) {
	client := &Client{
		Objects:       map[storageKey]runtime.Object{},
		errorInjector: errorInjector,
	}

	for _, obj := range initialObjects {
		key, err := getKey(obj)
		if err != nil {
			return nil, err
		}
		client.Objects[key] = obj
	}
	return client, nil
}

// Get finds object and puts it in client.Object obj argument
func (f Client) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	if f.errorInjector != nil {
		if err := f.errorInjector.shouldFail("Get", obj); err != nil {
			return err
		}
	}

	gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
	if err != nil {
		return err
	}
	k := storageKey{
		Name:      key.Name,
		Namespace: key.Namespace,
		Kind:      gvk.Kind,
	}
	o, found := f.Objects[k]
	if !found {
		gvr := schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}
		return errors.NewNotFound(gvr, key.Name)
	}

	j, err := json.Marshal(o)
	if err != nil {
		return err
	}
	decoder := scheme.Codecs.UniversalDecoder()
	_, _, err = decoder.Decode(j, nil, obj)
	return err
}

// List list all requested items in fake cluster
func (f Client) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.errorInjector != nil {
		if err := f.errorInjector.shouldFail("List", list); err != nil {
			return err
		}
	}
	switch list.(type) {
	case *storagev1.StorageClassList:
		return f.listStorageClasses(list.(*storagev1.StorageClassList))
	case *core_v1.PersistentVolumeClaimList:
		return f.listPersistentVolumeClaim(list.(*core_v1.PersistentVolumeClaimList), opts...)
	case *core_v1.PersistentVolumeList:
		return f.listPersistentVolume(list.(*core_v1.PersistentVolumeList), opts...)
	case *storagev1alpha1.DellCSIReplicationGroupList:
		return f.listReplicationGroup(list.(*storagev1alpha1.DellCSIReplicationGroupList), opts...)
	default:
		return fmt.Errorf("unknown type: %s", reflect.TypeOf(list))
	}
}

func (f *Client) listStorageClasses(list *storagev1.StorageClassList) error {
	for k, v := range f.Objects {
		if k.Kind == "StorageClass" {
			list.Items = append(list.Items, *v.(*storagev1.StorageClass))
		}
	}
	return nil
}

func (f *Client) listPersistentVolumeClaim(list *core_v1.PersistentVolumeClaimList, opts ...client.ListOption) error {
	lo := &client.ListOptions{}
	for _, option := range opts {
		option.ApplyToList(lo)
	}

	for k, v := range f.Objects {
		if k.Kind == "PersistentVolumeClaim" {
			pvc := *v.(*core_v1.PersistentVolumeClaim)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(pvc.Labels)) {
				continue
			}
			list.Items = append(list.Items, *v.(*core_v1.PersistentVolumeClaim))
		}
	}
	return nil
}

func (f *Client) listPersistentVolume(list *core_v1.PersistentVolumeList, opts ...client.ListOption) error {
	lo := &client.ListOptions{}
	for _, option := range opts {
		option.ApplyToList(lo)
	}

	for k, v := range f.Objects {
		if k.Kind == "PersistentVolume" {
			pv := *v.(*core_v1.PersistentVolume)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(pv.Labels)) {
				continue
			}
			list.Items = append(list.Items, *v.(*core_v1.PersistentVolume))
		}
	}
	return nil
}

func (f *Client) listReplicationGroup(list *storagev1alpha1.DellCSIReplicationGroupList, opts ...client.ListOption) error {
	lo := &client.ListOptions{}
	for _, option := range opts {
		option.ApplyToList(lo)
	}

	for k, v := range f.Objects {
		if k.Kind == "DellCSIReplicationGroup" {
			rg := *v.(*storagev1alpha1.DellCSIReplicationGroup)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(rg.Labels)) {
				continue
			}
			list.Items = append(list.Items, *v.(*storagev1alpha1.DellCSIReplicationGroup))
		}
	}
	return nil
}

// Create creates new object in fake cluster by putting it in map
func (f Client) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if f.errorInjector != nil {
		if err := f.errorInjector.shouldFail("Create", obj); err != nil {
			return err
		}
	}
	k, err := getKey(obj)
	if err != nil {
		return err
	}
	_, found := f.Objects[k]
	if found {
		gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
		if err != nil {
			return err
		}
		gvr := schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}
		return errors.NewAlreadyExists(gvr, k.Name)
	}
	f.Objects[k] = obj
	return nil
}

// Delete deletes existing object in fake cluster by removing it from map
func (f Client) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if len(opts) > 0 {
		return fmt.Errorf("delete options are not supported")
	}
	if f.errorInjector != nil {
		if err := f.errorInjector.shouldFail("Delete", obj); err != nil {
			return err
		}
	}

	k, err := getKey(obj)
	if err != nil {
		return err
	}
	_, found := f.Objects[k]
	if !found {
		gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
		if err != nil {
			return err
		}
		gvr := schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}
		return errors.NewNotFound(gvr, k.Name)
	}
	delete(f.Objects, k)
	return nil
}

// Update updates object in fake k8s cluster
func (f Client) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if f.errorInjector != nil {
		if err := f.errorInjector.shouldFail("Update", obj); err != nil {
			return err
		}
	}
	k, err := getKey(obj)
	if err != nil {
		return err
	}
	_, found := f.Objects[k]
	if !found {
		gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
		if err != nil {
			return err
		}
		gvr := schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}
		return errors.NewNotFound(gvr, k.Name)
	}
	f.Objects[k] = obj
	return nil
}

// Patch patches the given obj in the Kubernetes cluster
func (f Client) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	panic("implement me")
}

// DeleteAllOf deletes all objects of the given type matching the given options
func (f Client) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	panic("implement me")
}

// Status knows how to create a client which can update status subresource
// for kubernetes objects
func (f Client) Status() client.StatusWriter {
	return f
}

// Scheme returns the scheme this client is using
func (f Client) Scheme() *runtime.Scheme {
	panic("implement me")
}

// RESTMapper returns the rest this client is using
func (f Client) RESTMapper() meta.RESTMapper {
	panic("implement me")
}
