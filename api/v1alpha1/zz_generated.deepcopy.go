//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*


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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DellCSIReplicationGroup) DeepCopyInto(out *DellCSIReplicationGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DellCSIReplicationGroup.
func (in *DellCSIReplicationGroup) DeepCopy() *DellCSIReplicationGroup {
	if in == nil {
		return nil
	}
	out := new(DellCSIReplicationGroup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DellCSIReplicationGroup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DellCSIReplicationGroupList) DeepCopyInto(out *DellCSIReplicationGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DellCSIReplicationGroup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DellCSIReplicationGroupList.
func (in *DellCSIReplicationGroupList) DeepCopy() *DellCSIReplicationGroupList {
	if in == nil {
		return nil
	}
	out := new(DellCSIReplicationGroupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DellCSIReplicationGroupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DellCSIReplicationGroupSpec) DeepCopyInto(out *DellCSIReplicationGroupSpec) {
	*out = *in
	if in.ProtectionGroupAttributes != nil {
		in, out := &in.ProtectionGroupAttributes, &out.ProtectionGroupAttributes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.RemoteProtectionGroupAttributes != nil {
		in, out := &in.RemoteProtectionGroupAttributes, &out.RemoteProtectionGroupAttributes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DellCSIReplicationGroupSpec.
func (in *DellCSIReplicationGroupSpec) DeepCopy() *DellCSIReplicationGroupSpec {
	if in == nil {
		return nil
	}
	out := new(DellCSIReplicationGroupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DellCSIReplicationGroupStatus) DeepCopyInto(out *DellCSIReplicationGroupStatus) {
	*out = *in
	in.ReplicationLinkState.DeepCopyInto(&out.ReplicationLinkState)
	in.LastAction.DeepCopyInto(&out.LastAction)
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]LastAction, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DellCSIReplicationGroupStatus.
func (in *DellCSIReplicationGroupStatus) DeepCopy() *DellCSIReplicationGroupStatus {
	if in == nil {
		return nil
	}
	out := new(DellCSIReplicationGroupStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LastAction) DeepCopyInto(out *LastAction) {
	*out = *in
	if in.FirstFailure != nil {
		in, out := &in.FirstFailure, &out.FirstFailure
		*out = (*in).DeepCopy()
	}
	if in.Time != nil {
		in, out := &in.Time, &out.Time
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LastAction.
func (in *LastAction) DeepCopy() *LastAction {
	if in == nil {
		return nil
	}
	out := new(LastAction)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ReplicationLinkState) DeepCopyInto(out *ReplicationLinkState) {
	*out = *in
	if in.LastSuccessfulUpdate != nil {
		in, out := &in.LastSuccessfulUpdate, &out.LastSuccessfulUpdate
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ReplicationLinkState.
func (in *ReplicationLinkState) DeepCopy() *ReplicationLinkState {
	if in == nil {
		return nil
	}
	out := new(ReplicationLinkState)
	in.DeepCopyInto(out)
	return out
}

//Migration Functionality
// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DellCSIMigrationGroup) DeepCopyInto(out *DellCSIMigrationGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DellCSIMigrationGroup.
func (in *DellCSIMigrationGroup) DeepCopy() *DellCSIMigrationGroup {
	if in == nil {
		return nil
	}
	out := new(DellCSIMigrationGroup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DellCSIMigrationGroup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DellCSIMigrationGroupList) DeepCopyInto(out *DellCSIMigrationGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DellCSIMigrationGroup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DellCSIMigrationGroupList.
func (in *DellCSIMigrationGroupList) DeepCopy() *DellCSIMigrationGroupList {
	if in == nil {
		return nil
	}
	out := new(DellCSIMigrationGroupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DellCSIMigrationGroupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DellCSIMigrationGroupSpec.
func (in *DellCSIMigrationGroupSpec) DeepCopy() *DellCSIMigrationGroupSpec {
	if in == nil {
		return nil
	}
	out := new(DellCSIMigrationGroupSpec)
	in.DeepCopyInto(out)
	return out
}

