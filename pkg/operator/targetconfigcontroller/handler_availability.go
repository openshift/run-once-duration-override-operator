package targetconfigcontroller

import (
	"fmt"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
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

	available, err := a.deploy.IsAvailable()

	switch {
	case available:
		return
	case err == nil:
		v1helpers.SetOperatorCondition(&current.Status.Conditions, operatorv1.OperatorCondition{
			Type:    "Available",
			Status:  operatorv1.ConditionFalse,
			Reason:  appsv1.AdmissionWebhookNotAvailable,
			Message: fmt.Sprintf("name=%s deployment not complete", a.deploy.Name()),
		})
	case k8serrors.IsNotFound(err):
		v1helpers.SetOperatorCondition(&current.Status.Conditions, operatorv1.OperatorCondition{
			Type:    "Available",
			Status:  operatorv1.ConditionFalse,
			Reason:  appsv1.AdmissionWebhookNotAvailable,
			Message: err.Error(),
		})
	default:
		v1helpers.SetOperatorCondition(&current.Status.Conditions, operatorv1.OperatorCondition{
			Type:    "Available",
			Status:  operatorv1.ConditionFalse,
			Reason:  appsv1.InternalError,
			Message: err.Error(),
		})
	}

	return
}
