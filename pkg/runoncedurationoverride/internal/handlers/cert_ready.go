package handlers

import (
	"fmt"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/cert"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
	"github.com/openshift/run-once-duration-override-operator/pkg/secondarywatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewCertReadyHandler(o *Options) *certReadyHandler {
	return &certReadyHandler{
		client: o.Client.Kubernetes,
		lister: o.SecondaryLister,
	}
}

type certReadyHandler struct {
	client kubernetes.Interface
	lister *secondarywatch.Lister
}

func (c *certReadyHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original
	resources := original.Status.Resources

	if context.GetBundle() == nil {
		secret, err := c.lister.CoreV1SecretLister().Secrets(context.WebhookNamespace()).Get(resources.ServiceCertSecretRef.Name)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		configmap, err := c.lister.CoreV1ConfigMapLister().ConfigMaps(context.WebhookNamespace()).Get(resources.ServiceCAConfigMapRef.Name)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		servingCertCA := []byte(configmap.Data["service-ca.crt"])
		bundle := &cert.Bundle{
			Serving: cert.Serving{
				ServiceKey:  secret.Data["tls.key"],
				ServiceCert: secret.Data["tls.crt"],
			},
			ServingCertCA: servingCertCA,
		}

		if err := bundle.Validate(); err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, fmt.Errorf("certs not populated - %s", err.Error()))
			return
		}

		context.SetBundle(bundle)
	}

	bundle := context.GetBundle()
	current.Status.Hash.ServingCert = bundle.Hash()

	klog.V(2).Infof("key=%s cert check passed", original.Name)
	return
}
