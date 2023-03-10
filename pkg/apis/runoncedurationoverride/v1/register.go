// Package v1 contains API Schema definitions for the runoncedurationoverride v1 API group
// +k8s:deepcopy-gen=package,register
// +groupName=operator.openshift.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// GroupName
	GroupName = "operator.openshift.io"
	// GroupVersion is the group version used in this package.
	GroupVersion = "v1"
)

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: GroupVersion}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder initializes a scheme builder
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme is a global function that registers this API group & version to a scheme
	AddToScheme = SchemeBuilder.AddToScheme

	// localSchemeBuilder is expected by generated conversion functions
	localSchemeBuilder = &SchemeBuilder
)

// addKnownTypes adds the list of known types to Scheme
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&RunOnceDurationOverride{},
		&RunOnceDurationOverrideList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
