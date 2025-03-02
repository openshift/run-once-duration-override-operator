/*
Copyright The Kubernetes Authors.

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

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomResourceDefinitionConditionApplyConfiguration represents a declarative configuration of the CustomResourceDefinitionCondition type for use
// with apply.
type CustomResourceDefinitionConditionApplyConfiguration struct {
	Type               *apiextensionsv1.CustomResourceDefinitionConditionType `json:"type,omitempty"`
	Status             *apiextensionsv1.ConditionStatus                       `json:"status,omitempty"`
	LastTransitionTime *metav1.Time                                           `json:"lastTransitionTime,omitempty"`
	Reason             *string                                                `json:"reason,omitempty"`
	Message            *string                                                `json:"message,omitempty"`
}

// CustomResourceDefinitionConditionApplyConfiguration constructs a declarative configuration of the CustomResourceDefinitionCondition type for use with
// apply.
func CustomResourceDefinitionCondition() *CustomResourceDefinitionConditionApplyConfiguration {
	return &CustomResourceDefinitionConditionApplyConfiguration{}
}

// WithType sets the Type field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Type field is set to the value of the last call.
func (b *CustomResourceDefinitionConditionApplyConfiguration) WithType(value apiextensionsv1.CustomResourceDefinitionConditionType) *CustomResourceDefinitionConditionApplyConfiguration {
	b.Type = &value
	return b
}

// WithStatus sets the Status field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Status field is set to the value of the last call.
func (b *CustomResourceDefinitionConditionApplyConfiguration) WithStatus(value apiextensionsv1.ConditionStatus) *CustomResourceDefinitionConditionApplyConfiguration {
	b.Status = &value
	return b
}

// WithLastTransitionTime sets the LastTransitionTime field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LastTransitionTime field is set to the value of the last call.
func (b *CustomResourceDefinitionConditionApplyConfiguration) WithLastTransitionTime(value metav1.Time) *CustomResourceDefinitionConditionApplyConfiguration {
	b.LastTransitionTime = &value
	return b
}

// WithReason sets the Reason field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Reason field is set to the value of the last call.
func (b *CustomResourceDefinitionConditionApplyConfiguration) WithReason(value string) *CustomResourceDefinitionConditionApplyConfiguration {
	b.Reason = &value
	return b
}

// WithMessage sets the Message field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Message field is set to the value of the last call.
func (b *CustomResourceDefinitionConditionApplyConfiguration) WithMessage(value string) *CustomResourceDefinitionConditionApplyConfiguration {
	b.Message = &value
	return b
}
