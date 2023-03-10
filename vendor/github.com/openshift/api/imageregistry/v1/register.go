package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	version   = "v1"
	groupName = "imageregistry.operator.openshift.io"
)

var (
	scheme        = runtime.NewScheme()
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	GroupVersion  = schema.GroupVersion{Group: groupName, Version: version}
	// Install is a function which adds this version to a scheme
	Install = SchemeBuilder.AddToScheme

	// SchemeGroupVersion generated code relies on this name
	// Deprecated
	SchemeGroupVersion = GroupVersion
	// AddToScheme exists solely to keep the old generators creating valid code
	// DEPRECATED
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	AddToScheme(scheme)
}

// addKnownTypes adds the set of types defined in this package to the supplied scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Config{},
		&ConfigList{},
		&ImagePruner{},
		&ImagePrunerList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}
