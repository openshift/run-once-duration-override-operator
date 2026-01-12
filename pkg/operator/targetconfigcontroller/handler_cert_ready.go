package targetconfigcontroller

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/cert"
)

func NewCertReadyHandler(client kubernetes.Interface, secretLister listerscorev1.SecretLister, configMapLister listerscorev1.ConfigMapLister) *certReadyHandler {
	return &certReadyHandler{
		client:          client,
		secretLister:    secretLister,
		configMapLister: configMapLister,
	}
}

type certReadyHandler struct {
	client          kubernetes.Interface
	secretLister    listerscorev1.SecretLister
	configMapLister listerscorev1.ConfigMapLister
}

func (c *certReadyHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original
	resources := original.Status.Resources

	if context.GetBundle() == nil {
		secret, err := c.secretLister.Secrets(context.WebhookNamespace()).Get(resources.ServiceCertSecretRef.Name)
		if err != nil {
			handleErr = NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		configmap, err := c.configMapLister.ConfigMaps(context.WebhookNamespace()).Get(resources.ServiceCAConfigMapRef.Name)
		if err != nil {
			handleErr = NewInstallReadinessError(appsv1.CertNotAvailable, err)
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
			handleErr = NewInstallReadinessError(appsv1.CertNotAvailable, fmt.Errorf("certs not populated - %s", err.Error()))
			return
		}

		context.SetBundle(bundle)
	}

	bundle := context.GetBundle()
	current.Status.Hash.ServingCert = bundle.Hash()

	klog.V(2).Infof("key=%s cert check passed", original.Name)
	return
}
