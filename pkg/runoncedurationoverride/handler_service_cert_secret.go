package runoncedurationoverride

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/run-once-duration-override-operator/pkg/apis/reference"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
)

func NewServiceCertSecretHandler(client kubernetes.Interface, secretLister listerscorev1.SecretLister, asset *asset.Asset) *serviceCertSecretHandler {
	return &serviceCertSecretHandler{
		client:       client,
		secretLister: secretLister,
		asset:        asset,
	}
}

type serviceCertSecretHandler struct {
	client       kubernetes.Interface
	secretLister listerscorev1.SecretLister
	asset        *asset.Asset
}

func (c *serviceCertSecretHandler) Handle(ctx *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original

	// Make sure that we have all certs generated
	secretName := c.asset.ServiceServingSecret().Name()

	object, err := c.secretLister.Secrets(ctx.WebhookNamespace()).Get(secretName)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)

		if k8serrors.IsNotFound(err) {
			// We are still waiting for the server serving Secret object object to be
			// created by the service-ca operator.
			// No further action in the handler chain until we have a secret object.
			klog.V(2).Infof("key=%s resource=%T/%s waiting for server serving secret to be created by service-ca operator", original.Name, object, secretName)
		}

		return
	}

	values := c.asset.Values()

	// we need to annotate this secret so that if it is deleted/updated
	// we can enqueue the CR
	if owner, ok := object.Annotations[values.OwnerAnnotationKey]; !ok || owner != original.Name {
		copy := object.DeepCopy()
		if len(copy.Annotations) == 0 {
			copy.Annotations = map[string]string{}
		}

		copy.Annotations[values.OwnerAnnotationKey] = original.Name
		updated, updateErr := c.client.CoreV1().Secrets(ctx.WebhookNamespace()).Update(context.TODO(), copy, metav1.UpdateOptions{})
		if updateErr != nil {
			handleErr = updateErr
			return
		}

		object = updated
	}

	if ref := current.Status.Resources.ServiceCertSecretRef; ref != nil && ref.ResourceVersion == object.ResourceVersion {
		klog.V(2).Infof("key=%s resource=%T/%s is in sync", original.Name, object, object.Name)
		return
	}

	newRef, err := reference.GetReference(object)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.CannotSetReference, err)
		return
	}

	klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, object, object.Name, newRef.ResourceVersion)

	current.Status.Resources.ServiceCertSecretRef = newRef
	return
}
