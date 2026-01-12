package targetconfigcontroller

import (
	gocontext "context"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/run-once-duration-override-operator/pkg/apis/reference"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/cert"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/targetconfigcontroller/internal/condition"
)

var (
	// DefaultCertValidFor is the default duration a cert will be valid for.
	DefaultCertValidFor = time.Hour * 24 * 365

	// DefaultCertRotateThreshold is the default threshold preceding the expiration date
	// the operator will make attempt(s) to rotate the certs.
	DefaultCertRotateThreshold = time.Hour * 48

	Organization = "Red Hat, Inc."
)

func NewCertGenerationHandler(client kubernetes.Interface, recorder events.Recorder, secretLister listerscorev1.SecretLister, configMapLister listerscorev1.ConfigMapLister, asset *asset.Asset) *certGenerationHandler {
	return &certGenerationHandler{
		client:          client,
		recorder:        recorder,
		secretLister:    secretLister,
		configMapLister: configMapLister,
		asset:           asset,
	}
}

type certGenerationHandler struct {
	client          kubernetes.Interface
	recorder        events.Recorder
	secretLister    listerscorev1.SecretLister
	configMapLister listerscorev1.ConfigMapLister
	asset           *asset.Asset
}

func (c *certGenerationHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original
	ensure := false

	secretName := c.asset.ServiceServingSecret().Name()
	currentSecret, secretGetErr := c.secretLister.Secrets(context.WebhookNamespace()).Get(secretName)
	if secretGetErr != nil && !k8serrors.IsNotFound(secretGetErr) {
		handleErr = condition.NewInstallReadinessError(appsv1.InternalError, secretGetErr)
		return
	}

	configMapName := c.asset.CABundleConfigMap().Name()
	currentConfigMap, configMapGetErr := c.configMapLister.ConfigMaps(context.WebhookNamespace()).Get(configMapName)
	if configMapGetErr != nil && !k8serrors.IsNotFound(configMapGetErr) {
		handleErr = condition.NewInstallReadinessError(appsv1.InternalError, configMapGetErr)
		return
	}

	switch {
	case k8serrors.IsNotFound(secretGetErr) || k8serrors.IsNotFound(configMapGetErr):
		ensure = true
	case original.IsTimeToRotateCert() || !cert.IsPopulated(currentSecret):
		ensure = true
	case original.Status.CertsRotateAt.IsZero():
		ensure = true
	}

	if ensure {
		// generate cert.
		expiresAt := time.Now().Add(DefaultCertValidFor)
		bundle, err := cert.GenerateWithLocalhostServing(expiresAt, Organization)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CannotGenerateCert, err)
			return
		}

		// ensure that we have a serving Secret
		desiredSecret := c.asset.ServiceServingSecret().New()
		context.ControllerSetter().Set(desiredSecret, original)

		if len(desiredSecret.Data) == 0 {
			desiredSecret.Data = map[string][]byte{}
		}
		desiredSecret.Data["tls.key"] = bundle.Serving.ServiceKey
		desiredSecret.Data["tls.crt"] = bundle.Serving.ServiceCert

		secret, _, err := resourceapply.ApplySecret(gocontext.TODO(), c.client.CoreV1(), c.recorder, desiredSecret)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CannotGenerateCert, err)
			return
		}

		// ensure that we have a configmap with the serving CA bundle
		desiredConfigMap := c.asset.CABundleConfigMap().New()

		context.ControllerSetter().Set(desiredConfigMap, original)
		if len(desiredConfigMap.Data) == 0 {
			desiredConfigMap.Data = map[string]string{}
		}
		desiredConfigMap.Data["service-ca.crt"] = string(bundle.ServingCertCA)

		configmap, _, err := resourceapply.ApplyConfigMap(gocontext.TODO(), c.client.CoreV1(), c.recorder, desiredConfigMap)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CannotGenerateCert, err)
			return
		}

		context.SetBundle(bundle)
		current.Status.CertsRotateAt = metav1.NewTime(expiresAt.Add(-1 * DefaultCertRotateThreshold))

		currentSecret = secret
		klog.V(2).Infof("key=%s resource=%T/%s successfully ensured", original.Name, currentSecret, currentSecret.Name)

		currentConfigMap = configmap
		klog.V(2).Infof("key=%s resource=%T/%s successfully ensured", original.Name, currentConfigMap, currentConfigMap.Name)
	}

	if ref := current.Status.Resources.ServiceCertSecretRef; ref == nil || ref.ResourceVersion != currentSecret.ResourceVersion {
		newRef, err := reference.GetReference(currentSecret)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CannotSetReference, err)
			return
		}

		klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, currentSecret, currentSecret.Name, newRef.ResourceVersion)
		current.Status.Resources.ServiceCertSecretRef = newRef
	}
	klog.V(2).Infof("key=%s resource=%T/%s is original sync", original.Name, currentSecret, currentSecret.Name)

	if ref := current.Status.Resources.ServiceCAConfigMapRef; ref == nil || ref.ResourceVersion != currentConfigMap.ResourceVersion {
		newRef, err := reference.GetReference(currentConfigMap)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CannotSetReference, err)
			return
		}

		klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, currentConfigMap, currentConfigMap.Name, newRef.ResourceVersion)
		current.Status.Resources.ServiceCAConfigMapRef = newRef
	}
	klog.V(2).Infof("key=%s resource=%T/%s is original sync", original.Name, currentConfigMap, currentConfigMap.Name)

	return
}
