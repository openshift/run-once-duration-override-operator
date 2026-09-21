package targetconfigcontroller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
)

func TestManageNetworkPolicyDefaultDeny(t *testing.T) {
	kubeClient := kubefake.NewSimpleClientset()
	recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
	testAsset := asset.New(createTestOperandContext())

	handler := &networkPolicyDefaultDenyHandler{
		client:   kubeClient,
		recorder: recorder,
		asset:    testAsset,
		cache:    resourceapply.NewResourceCache(),
	}

	reconcileContext := NewReconcileRequestContext(createTestOperandContext())
	rodoo := createTestRodoo(3600, nil)
	rodoo.SetGroupVersionKind(RunOnceDurationOverrideGVK)

	_, _, err := handler.Handle(reconcileContext, rodoo)
	if err != nil {
		t.Fatalf("failed to apply network policy: %v", err)
	}

	// Verify the default-deny policy was created
	expectedName := "default-deny"
	policy, err := kubeClient.NetworkingV1().NetworkPolicies(testAsset.Values().Namespace).Get(
		context.TODO(), expectedName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected default-deny policy to exist: %v", err)
	}
	if policy.Name != expectedName {
		t.Errorf("expected policy name %q, got %q", expectedName, policy.Name)
	}
}
