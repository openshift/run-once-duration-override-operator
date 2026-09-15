package asset

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkPolicyDefaultDeny returns a default-deny NetworkPolicy for the operator namespace.
//
// This policy selects all pods in the namespace and enables default-deny for both ingress
// and egress by specifying policyTypes without any allow rules.
//
// NetworkPolicies are additive (use OR logic):
// - This policy enables default-deny for all pods
// - Subsequent policies add specific allow rules
// - If any policy allows traffic, that traffic is permitted
// - Policies cannot override or block traffic allowed by other policies
//
// Note: The run-once-duration-override DaemonSet uses hostNetwork=true and automatically
// bypasses all NetworkPolicy rules. This policy only affects future pods that run on the
// pod network (if any are added, such as helper pods similar to kube-apiserver-operator's
// guard/installer/pruner pods).
func (a *Asset) NetworkPolicyDefaultDeny() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkPolicy",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny",
			Namespace: a.values.Namespace,
			Annotations: map[string]string{
				"include.release.openshift.io/self-managed-high-availability": "true",
				"include.release.openshift.io/single-node-developer":          "true",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}
}
