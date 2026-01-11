package runoncedurationoverride

import (
	gocontext "context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/run-once-duration-override-operator/pkg/apis/reference"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
)

const (
	ServiceCAInjectBundle = "service.beta.openshift.io/inject-cabundle"
)

func NewServiceCAConfigMapHandler(client kubernetes.Interface, recorder events.Recorder, configMapLister listerscorev1.ConfigMapLister, asset *asset.Asset) *serviceCAConfigMapHandler {
	return &serviceCAConfigMapHandler{
		client:          client,
		recorder:        recorder,
		configMapLister: configMapLister,
		asset:           asset,
	}
}

type serviceCAConfigMapHandler struct {
	client          kubernetes.Interface
	recorder        events.Recorder
	configMapLister listerscorev1.ConfigMapLister
	asset           *asset.Asset
}

func (c *serviceCAConfigMapHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original

	// assume that resource is in sync.
	ensure := false

	name := c.asset.CABundleConfigMap().Name()
	object, err := c.configMapLister.ConfigMaps(context.WebhookNamespace()).Get(name)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		ensure = true
	}

	if !ensure && object.Annotations[ServiceCAInjectBundle] != "true" {
		klog.V(2).Infof("key=%s resource=%T/%s resource has drifted", original.Name, object, object.Name)
		ensure = true
	}

	if ensure {
		desired := c.asset.CABundleConfigMap().New()

		context.ControllerSetter().Set(desired, original)
		if len(desired.Annotations) == 0 {
			desired.Annotations = map[string]string{}
		}

		// ask service-ca operator to provide with the serving cert.
		desired.Annotations[ServiceCAInjectBundle] = "true"

		cm, _, err := resourceapply.ApplyConfigMap(gocontext.TODO(), c.client.CoreV1(), c.recorder, desired)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
			return
		}

		object = cm
		klog.V(2).Infof("key=%s resource=%T/%s successfully ensured", original.Name, object, object.Name)
	}

	if ref := current.Status.Resources.ServiceCAConfigMapRef; ref != nil && ref.ResourceVersion == object.ResourceVersion {
		klog.V(2).Infof("key=%s resource=%T/%s is in sync", original.Name, object, object.Name)
		return
	}

	newRef, err := reference.GetReference(object)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
		return
	}

	klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, object, object.Name, newRef.ResourceVersion)
	current.Status.Resources.ServiceCAConfigMapRef = newRef
	return
}
