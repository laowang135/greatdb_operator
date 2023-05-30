//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2022.

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
	"k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GreatDBPaxos) DeepCopyInto(out *GreatDBPaxos) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GreatDBPaxos.
func (in *GreatDBPaxos) DeepCopy() *GreatDBPaxos {
	if in == nil {
		return nil
	}
	out := new(GreatDBPaxos)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GreatDBPaxos) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GreatDBPaxosConditions) DeepCopyInto(out *GreatDBPaxosConditions) {
	*out = *in
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GreatDBPaxosConditions.
func (in *GreatDBPaxosConditions) DeepCopy() *GreatDBPaxosConditions {
	if in == nil {
		return nil
	}
	out := new(GreatDBPaxosConditions)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GreatDBPaxosList) DeepCopyInto(out *GreatDBPaxosList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GreatDBPaxos, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GreatDBPaxosList.
func (in *GreatDBPaxosList) DeepCopy() *GreatDBPaxosList {
	if in == nil {
		return nil
	}
	out := new(GreatDBPaxosList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GreatDBPaxosList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GreatDBPaxosSpec) DeepCopyInto(out *GreatDBPaxosSpec) {
	*out = *in
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	in.PodSecurityContext.DeepCopyInto(&out.PodSecurityContext)
	if in.ImagePullSecrets != nil {
		in, out := &in.ImagePullSecrets, &out.ImagePullSecrets
		*out = make([]v1.LocalObjectReference, len(*in))
		copy(*out, *in)
	}
	in.Resources.DeepCopyInto(&out.Resources)
	in.VolumeClaimTemplates.DeepCopyInto(&out.VolumeClaimTemplates)
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Users != nil {
		in, out := &in.Users, &out.Users
		*out = make([]User, len(*in))
		copy(*out, *in)
	}
	out.Service = in.Service
	if in.Pause != nil {
		in, out := &in.Pause, &out.Pause
		*out = new(PauseGreatDB)
		(*in).DeepCopyInto(*out)
	}
	if in.Restart != nil {
		in, out := &in.Restart, &out.Restart
		*out = new(RestartGreatDB)
		(*in).DeepCopyInto(*out)
	}
	if in.Delete != nil {
		in, out := &in.Delete, &out.Delete
		*out = new(DeleteInstance)
		(*in).DeepCopyInto(*out)
	}
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GreatDBPaxosSpec.
func (in *GreatDBPaxosSpec) DeepCopy() *GreatDBPaxosSpec {
	if in == nil {
		return nil
	}
	out := new(GreatDBPaxosSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GreatDBPaxosStatus) DeepCopyInto(out *GreatDBPaxosStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]GreatDBPaxosConditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Member != nil {
		in, out := &in.Member, &out.Member
		*out = make([]MemberCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.UpgradeMember.DeepCopyInto(&out.UpgradeMember)
	in.RestartMember.DeepCopyInto(&out.RestartMember)
	if in.Users != nil {
		in, out := &in.Users, &out.Users
		*out = make([]User, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GreatDBPaxosStatus.
func (in *GreatDBPaxosStatus) DeepCopy() *GreatDBPaxosStatus {
	if in == nil {
		return nil
	}
	out := new(GreatDBPaxosStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MemberCondition) DeepCopyInto(out *MemberCondition) {
	*out = *in
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MemberCondition.
func (in *MemberCondition) DeepCopy() *MemberCondition {
	if in == nil {
		return nil
	}
	out := new(MemberCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PauseGreatDB) DeepCopyInto(out *PauseGreatDB) {
	*out = *in
	if in.Instances != nil {
		in, out := &in.Instances, &out.Instances
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PauseGreatDB.
func (in *PauseGreatDB) DeepCopy() *PauseGreatDB {
	if in == nil {
		return nil
	}
	out := new(PauseGreatDB)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RestartGreatDB) DeepCopyInto(out *RestartGreatDB) {
	*out = *in
	if in.Instances != nil {
		in, out := &in.Instances, &out.Instances
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RestartGreatDB.
func (in *RestartGreatDB) DeepCopy() *RestartGreatDB {
	if in == nil {
		return nil
	}
	out := new(RestartGreatDB)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RestartMember) DeepCopyInto(out *RestartMember) {
	*out = *in
	if in.Restarting != nil {
		in, out := &in.Restarting, &out.Restarting
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Restarted != nil {
		in, out := &in.Restarted, &out.Restarted
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RestartMember.
func (in *RestartMember) DeepCopy() *RestartMember {
	if in == nil {
		return nil
	}
	out := new(RestartMember)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServiceType) DeepCopyInto(out *ServiceType) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServiceType.
func (in *ServiceType) DeepCopy() *ServiceType {
	if in == nil {
		return nil
	}
	out := new(ServiceType)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpgradeMember) DeepCopyInto(out *UpgradeMember) {
	*out = *in
	if in.Upgrading != nil {
		in, out := &in.Upgrading, &out.Upgrading
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Upgraded != nil {
		in, out := &in.Upgraded, &out.Upgraded
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpgradeMember.
func (in *UpgradeMember) DeepCopy() *UpgradeMember {
	if in == nil {
		return nil
	}
	out := new(UpgradeMember)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *User) DeepCopyInto(out *User) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new User.
func (in *User) DeepCopy() *User {
	if in == nil {
		return nil
	}
	out := new(User)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeleteInstance) DeepCopyInto(out *DeleteInstance) {
	*out = *in
	if in.Instances != nil {
		in, out := &in.Instances, &out.Instances
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeleteInstance.
func (in *DeleteInstance) DeepCopy() *DeleteInstance {
	if in == nil {
		return nil
	}
	out := new(DeleteInstance)
	in.DeepCopyInto(out)
	return out
}
