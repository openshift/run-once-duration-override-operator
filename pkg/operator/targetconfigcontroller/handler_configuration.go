package targetconfigcontroller

import (
	gocontext "context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/run-once-duration-override-operator/pkg/apis/reference"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/targetconfigcontroller/internal/condition"
)

func NewConfigurationHandler(client kubernetes.Interface, recorder events.Recorder, configMapLister listerscorev1.ConfigMapLister, asset *asset.Asset) *configurationHandler {
	return &configurationHandler{
		client:          client,
		recorder:        recorder,
		configMapLister: configMapLister,
		asset:           asset,
	}
}

type configurationHandler struct {
	client          kubernetes.Interface
	recorder        events.Recorder
	asset           *asset.Asset
	configMapLister listerscorev1.ConfigMapLister
}

func (c *configurationHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original

	desired, err := c.NewConfiguration(context, original)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.ConfigurationCheckFailed, err)
		return
	}

	name := c.asset.Configuration().Name()
	object, err := c.configMapLister.ConfigMaps(context.WebhookNamespace()).Get(name)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			handleErr = condition.NewInstallReadinessError(appsv1.InternalError, err)
			return
		}

		cm, _, err := resourceapply.ApplyConfigMap(gocontext.TODO(), c.client.CoreV1(), c.recorder, desired)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.InternalError, err)
			return
		}

		object = cm
		klog.V(2).Infof("key=%s resource=%T/%s successfully created", original.Name, object, object.Name)
	}

	equal := false
	hash := original.Spec.RunOnceDurationOverrideConfig.Spec.Hash()
	if hash == current.Status.Hash.Configuration {
		equal = true
	}

	if ref := current.Status.Resources.ConfigurationRef; equal && ref != nil && ref.ResourceVersion == object.ResourceVersion {
		klog.V(2).Infof("key=%s resource=%T/%s is in sync", original.Name, object, object.Name)
		return
	}

	if !equal {
		klog.V(2).Infof("key=%s resource=%T/%s configuration has drifted", original.Name, object, object.Name)

		cm, _, err := resourceapply.ApplyConfigMap(gocontext.TODO(), c.client.CoreV1(), c.recorder, desired)
		if err != nil {
			handleErr = condition.NewInstallReadinessError(appsv1.ConfigurationCheckFailed, err)
			return
		}

		object = cm
	}

	newRef, err := reference.GetReference(object)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.CannotSetReference, err)
		return
	}

	current.Status.Hash.Configuration = hash
	current.Status.Resources.ConfigurationRef = newRef

	klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, object, object.Name, newRef.ResourceVersion)
	return
}

func (c *configurationHandler) NewConfiguration(context *ReconcileRequestContext, override *appsv1.RunOnceDurationOverride) (configuration *corev1.ConfigMap, err error) {
	bytes, err := yaml.Marshal(override.Spec.RunOnceDurationOverrideConfig)
	if err != nil {
		return
	}

	configuration = c.asset.Configuration().New()

	// Set owner reference.
	context.ControllerSetter().Set(configuration, override)

	if len(configuration.Data) == 0 {
		configuration.Data = map[string]string{}
	}
	configuration.Data[c.asset.Values().ConfigurationKey] = string(bytes)
	return
}

func (c *configurationHandler) IsConfigurationEqual(current *corev1.ConfigMap, this *appsv1.RunOnceDurationOverride) (equal bool, err error) {
	observed := current.Data[c.asset.Values().ConfigurationKey]

	other := &appsv1.RunOnceDurationOverride{}
	err = yaml.Unmarshal([]byte(observed), other)
	if err != nil {
		return
	}

	equal = equality.Semantic.DeepEqual(this, other)
	return
}
