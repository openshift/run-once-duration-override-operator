package runoncedurationoverride

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
)

func NewAvailabilityHandler(asset *asset.Asset, deploy deploy.Interface) *availabilityHandler {
	return &availabilityHandler{
		asset:  asset,
		deploy: deploy,
	}
}

type availabilityHandler struct {
	asset  *asset.Asset
	deploy deploy.Interface
}

func (a *availabilityHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original
	builder := condition.NewBuilderWithStatus(&current.Status)

	available, err := a.deploy.IsAvailable()

	switch {
	case available:
		builder.WithAvailable(corev1.ConditionTrue, "")
	case err == nil:
		builder.WithError(condition.NewAvailableError(appsv1.AdmissionWebhookNotAvailable, fmt.Errorf("name=%s deployment not complete", a.deploy.Name())))
	case k8serrors.IsNotFound(err):
		builder.WithError(condition.NewAvailableError(appsv1.AdmissionWebhookNotAvailable, err))
	default:
		builder.WithError(condition.NewAvailableError(appsv1.InternalError, err))
	}

	return
}
