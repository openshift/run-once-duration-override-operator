package handlers

import (
	"github.com/openshift/run-once-duration-override-operator/pkg/apis/reference"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/ensurer"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
	"github.com/openshift/run-once-duration-override-operator/pkg/secondarywatch"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewServiceMonitorHandler(o *Options) *serviceMonitorHandler {
	return &serviceMonitorHandler{
		dynamic: ensurer.NewServiceMonitorEnsurer(o.Client.Dynamic),
		lister:  o.SecondaryLister,
		asset:   o.Asset,
	}
}

type serviceMonitorHandler struct {
	dynamic *ensurer.ServiceMonitorEnsurer
	lister  *secondarywatch.Lister
	asset   *asset.Asset
}

func (w *serviceMonitorHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original
	ensure := false

	name := w.asset.ServiceMonitor().Name()
	object, err := w.lister.ServiceMonitorV1Lister().ServiceMonitors(context.WebhookNamespace()).Get(name)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}
		ensure = true
	}

	if ensure {
		desired := w.asset.ServiceMonitor().New()
		context.ControllerSetter().Set(desired, original)

		servicemonitor, err := w.dynamic.Ensure(desired)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		object = servicemonitor
		klog.V(2).Infof("key=%s resource=%T/%s successfully created", original.Name, object, object.Name)
	}

	if ref := original.Status.Resources.ServiceMonitorRef; ref != nil && ref.ResourceVersion == object.ResourceVersion {
		klog.V(2).Infof("key=%s resource=%T/%s is in sync", original.Name, object, object.Name)
		return
	}

	newRef, err := reference.GetReference(object)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
		return
	}

	klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, object, object.Name, newRef.ResourceVersion)

	current.Status.Resources.ServiceMonitorRef = newRef
	return
}
