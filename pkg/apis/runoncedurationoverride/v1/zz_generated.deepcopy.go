//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2023 Red Hat, Inc.

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

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1

import (
	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RunOnceDurationOverride) DeepCopyInto(out *RunOnceDurationOverride) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RunOnceDurationOverride.
func (in *RunOnceDurationOverride) DeepCopy() *RunOnceDurationOverride {
	if in == nil {
		return nil
	}
	out := new(RunOnceDurationOverride)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RunOnceDurationOverride) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RunOnceDurationOverrideCondition) DeepCopyInto(out *RunOnceDurationOverrideCondition) {
	*out = *in
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RunOnceDurationOverrideCondition.
func (in *RunOnceDurationOverrideCondition) DeepCopy() *RunOnceDurationOverrideCondition {
	if in == nil {
		return nil
	}
	out := new(RunOnceDurationOverrideCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RunOnceDurationOverrideList) DeepCopyInto(out *RunOnceDurationOverrideList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RunOnceDurationOverride, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RunOnceDurationOverrideList.
func (in *RunOnceDurationOverrideList) DeepCopy() *RunOnceDurationOverrideList {
	if in == nil {
		return nil
	}
	out := new(RunOnceDurationOverrideList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RunOnceDurationOverrideList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RunOnceDurationOverrideResourceHash) DeepCopyInto(out *RunOnceDurationOverrideResourceHash) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RunOnceDurationOverrideResourceHash.
func (in *RunOnceDurationOverrideResourceHash) DeepCopy() *RunOnceDurationOverrideResourceHash {
	if in == nil {
		return nil
	}
	out := new(RunOnceDurationOverrideResourceHash)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RunOnceDurationOverrideResources) DeepCopyInto(out *RunOnceDurationOverrideResources) {
	*out = *in
	if in.ConfigurationRef != nil {
		in, out := &in.ConfigurationRef, &out.ConfigurationRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
	if in.ServiceCAConfigMapRef != nil {
		in, out := &in.ServiceCAConfigMapRef, &out.ServiceCAConfigMapRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
	if in.ServiceRef != nil {
		in, out := &in.ServiceRef, &out.ServiceRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
	if in.ServiceCertSecretRef != nil {
		in, out := &in.ServiceCertSecretRef, &out.ServiceCertSecretRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
	if in.DeploymentRef != nil {
		in, out := &in.DeploymentRef, &out.DeploymentRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
	if in.APiServiceRef != nil {
		in, out := &in.APiServiceRef, &out.APiServiceRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
	if in.MutatingWebhookConfigurationRef != nil {
		in, out := &in.MutatingWebhookConfigurationRef, &out.MutatingWebhookConfigurationRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RunOnceDurationOverrideResources.
func (in *RunOnceDurationOverrideResources) DeepCopy() *RunOnceDurationOverrideResources {
	if in == nil {
		return nil
	}
	out := new(RunOnceDurationOverrideResources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RunOnceDurationOverrideSpec) DeepCopyInto(out *RunOnceDurationOverrideSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RunOnceDurationOverrideSpec.
func (in *RunOnceDurationOverrideSpec) DeepCopy() *RunOnceDurationOverrideSpec {
	if in == nil {
		return nil
	}
	out := new(RunOnceDurationOverrideSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RunOnceDurationOverrideStatus) DeepCopyInto(out *RunOnceDurationOverrideStatus) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
	out.Hash = in.Hash
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]RunOnceDurationOverrideCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.CertsRotateAt.DeepCopyInto(&out.CertsRotateAt)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RunOnceDurationOverrideStatus.
func (in *RunOnceDurationOverrideStatus) DeepCopy() *RunOnceDurationOverrideStatus {
	if in == nil {
		return nil
	}
	out := new(RunOnceDurationOverrideStatus)
	in.DeepCopyInto(out)
	return out
}
