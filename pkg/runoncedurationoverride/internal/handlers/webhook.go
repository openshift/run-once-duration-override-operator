package handlers

import (
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/apps/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/apis/reference"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/ensurer"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
	"github.com/openshift/run-once-duration-override-operator/pkg/secondarywatch"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewWebhookConfigurationHandlerHandler(o *Options) *webhookConfigurationHandler {
	return &webhookConfigurationHandler{
		dynamic: ensurer.NewMutatingWebhookConfigurationEnsurer(o.Client.Dynamic),
		lister:  o.SecondaryLister,
		asset:   o.Asset,
	}
}

type webhookConfigurationHandler struct {
	dynamic *ensurer.MutatingWebhookConfigurationEnsurer
	lister  *secondarywatch.Lister
	asset   *asset.Asset
}

func (w *webhookConfigurationHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original
	ensure := false

	name := w.asset.NewMutatingWebhookConfiguration().Name()
	object, err := w.lister.AdmissionRegistrationV1MutatingWebhookConfigurationLister().Get(name)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		ensure = true
	}

	if ensure {
		desired := w.asset.NewMutatingWebhookConfiguration().New()
		context.ControllerSetter().Set(desired, original)

		servingCertCA := context.GetBundle().ServingCertCA
		for i := range desired.Webhooks {
			desired.Webhooks[i].ClientConfig.CABundle = servingCertCA
		}

		webhook, err := w.dynamic.Ensure(desired)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		object = webhook
		klog.V(2).Infof("key=%s resource=%T/%s successfully created", original.Name, object, object.Name)
	}

	if ref := original.Status.Resources.MutatingWebhookConfigurationRef; ref != nil && ref.ResourceVersion == object.ResourceVersion {
		klog.V(2).Infof("key=%s resource=%T/%s is in sync", original.Name, object, object.Name)
		return
	}

	newRef, err := reference.GetReference(object)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
		return
	}

	klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, object, object.Name, newRef.ResourceVersion)

	current.Status.Resources.MutatingWebhookConfigurationRef = newRef
	return
}
