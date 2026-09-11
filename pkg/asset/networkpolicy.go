package asset

import (
	_ "embed"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

//go:embed networkpolicy/default-deny.yaml
var defaultDenyNetworkPolicyYAML []byte

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

func init() {
	_ = networkingv1.AddToScheme(scheme)
}

// NetworkPolicyDefaultDeny returns the default-deny NetworkPolicy with dynamic values substituted.
func (a *Asset) NetworkPolicyDefaultDeny() *networkingv1.NetworkPolicy {
	obj, err := runtime.Decode(codecs.UniversalDeserializer(), defaultDenyNetworkPolicyYAML)
	if err != nil {
		panic(err)
	}
	policy := obj.(*networkingv1.NetworkPolicy)

	// Apply dynamic namespace
	policy.Namespace = a.values.Namespace

	return policy
}
