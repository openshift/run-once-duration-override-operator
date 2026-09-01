package targetconfigcontroller

import (
	gocontext "context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
)

func NewNetworkPolicyDefaultDenyHandler(client kubernetes.Interface, recorder events.Recorder, asset *asset.Asset) *networkPolicyDefaultDenyHandler {
	return &networkPolicyDefaultDenyHandler{
		client:   client,
		recorder: recorder,
		asset:    asset,
		cache:    resourceapply.NewResourceCache(),
	}
}

type networkPolicyDefaultDenyHandler struct {
	client   kubernetes.Interface
	recorder events.Recorder
	asset    *asset.Asset
	cache    resourceapply.ResourceCache
}

func (h *networkPolicyDefaultDenyHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original

	defaultDenyPolicy := h.asset.NetworkPolicyDefaultDeny()
	context.ControllerSetter().Set(defaultDenyPolicy, original)

	if _, _, err := resourceapply.ApplyNetworkPolicy(gocontext.TODO(), h.client.NetworkingV1(), h.recorder, defaultDenyPolicy, h.cache); err != nil {
		handleErr = NewInstallReadinessError(appsv1.InternalError, fmt.Errorf("failed to apply default-deny NetworkPolicy: %w", err))
		return
	}

	return
}
