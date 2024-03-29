package handlers

import (
	"github.com/openshift/run-once-duration-override-operator/pkg/apis/reference"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/ensurer"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
	"github.com/openshift/run-once-duration-override-operator/pkg/secondarywatch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewServiceHandler(o *Options) *serviceHandler {
	return &serviceHandler{
		dynamic: ensurer.NewServiceEnsurer(o.Client.Dynamic),
		lister:  o.SecondaryLister,
		asset:   o.Asset,
	}
}

type serviceHandler struct {
	dynamic *ensurer.ServiceEnsurer
	lister  *secondarywatch.Lister
	asset   *asset.Asset
}

func (s *serviceHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original

	desired := s.asset.Service().New()
	context.ControllerSetter().Set(desired, original)

	name := s.asset.Service().Name()
	object, err := s.lister.CoreV1ServiceLister().Services(context.WebhookNamespace()).Get(name)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		service, err := s.dynamic.Ensure(desired)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		object = service
		klog.V(2).Infof("key=%s resource=%T/%s successfully created", original.Name, object, object.Name)
	}

	if ref := current.Status.Resources.ServiceRef; ref != nil && ref.ResourceVersion == object.ResourceVersion {
		klog.V(2).Infof("key=%s resource=%T/%s is in sync", original.Name, object, object.Name)
		return
	}

	newRef, err := reference.GetReference(object)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.CannotSetReference, err)
		return
	}

	klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, object, object.Name, newRef.ResourceVersion)

	current.Status.Resources.ServiceRef = newRef
	return
}

func (s *serviceHandler) Equal(this, that *corev1.Service) bool {
	return equality.Semantic.DeepDerivative(&this.Spec, &that.Spec) &&
		equality.Semantic.DeepDerivative(this.GetObjectMeta(), that.GetObjectMeta())
}
