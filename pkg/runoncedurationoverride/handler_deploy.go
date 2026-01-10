package runoncedurationoverride

import (
	"context"
	"fmt"

	k8sappsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/run-once-duration-override-operator/pkg/apis/reference"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
	"github.com/openshift/run-once-duration-override-operator/pkg/secondarywatch"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewDaemonSetHandler(o *HandlerOptions) *daemonSetHandler {
	return &daemonSetHandler{
		client:   o.Client.Kubernetes,
		recorder: o.Recorder,
		asset:    o.Asset,
		lister:   o.SecondaryLister,
		deploy:   o.Deploy,
	}
}

type daemonSetHandler struct {
	client   kubernetes.Interface
	recorder events.Recorder
	lister   *secondarywatch.Lister
	asset    *asset.Asset

	deploy deploy.Interface
}

type Deployer interface {
	Exists(namespace, name string) (object metav1.Object, err error)
}

func (c *daemonSetHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original
	ensure := false

	object, accessor, getErr := c.deploy.Get()
	if getErr != nil && !k8serrors.IsNotFound(getErr) {
		handleErr = condition.NewInstallReadinessError(appsv1.InternalError, getErr)
		return
	}

	values := c.asset.Values()
	switch {
	case k8serrors.IsNotFound(getErr):
		ensure = true
	case accessor.GetAnnotations()[values.ConfigurationHashAnnotationKey] != current.Status.Hash.Configuration:
		klog.V(2).Infof("key=%s resource=%T/%s configuration hash mismatch", original.Name, object, accessor.GetName())
		ensure = true
	case accessor.GetAnnotations()[values.ServingCertHashAnnotationKey] != current.Status.Hash.ServingCert:
		klog.V(2).Infof("key=%s resource=%T/%s serving cert hash mismatch", original.Name, object, accessor.GetName())
		ensure = true
	case values.OperandImage != object.(*k8sappsv1.DaemonSet).Spec.Template.Spec.Containers[0].Image:
		klog.V(2).Infof("key=%s resource=%T/%s container image mismatch", original.Name, object, accessor.GetName())
		ensure = true
	}

	if ensure {
		object, accessor, handleErr = c.Ensure(context, original)
		if handleErr != nil {
			return
		}

		resourcemerge.SetDaemonSetGeneration(&current.Status.Generations, object.(*k8sappsv1.DaemonSet))
		klog.V(2).Infof("key=%s resource=%T/%s successfully ensured", original.Name, object, accessor.GetName())
	}

	if ref := current.Status.Resources.DeploymentRef; ref != nil && ref.ResourceVersion == accessor.GetResourceVersion() {
		klog.V(2).Infof("key=%s resource=%T/%s is in sync", original.Name, object, accessor.GetName())
		return
	}

	newRef, err := reference.GetReference(object)
	if err != nil {
		handleErr = condition.NewInstallReadinessError(appsv1.CertNotAvailable, err)
		return
	}

	klog.V(2).Infof("key=%s resource=%T/%s resource-version=%s setting object reference", original.Name, object, accessor.GetName(), newRef.ResourceVersion)
	current.Status.Resources.DeploymentRef = newRef

	return
}

func (c *daemonSetHandler) Ensure(ctx *ReconcileRequestContext, cro *appsv1.RunOnceDurationOverride) (current runtime.Object, accessor metav1.Object, err error) {
	name := c.asset.NewMutatingWebhookConfiguration().Name()
	if deleteErr := c.client.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(context.TODO(), name, metav1.DeleteOptions{}); deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
		err = fmt.Errorf("failed to delete MutatingWebhookConfiguration - %s", deleteErr.Error())
		return
	}

	if err = c.EnsureRBAC(ctx, cro); err != nil {
		return
	}

	parent := c.ApplyToDeploymentObject(ctx, cro)
	child := c.ApplyToToPodTemplate(ctx, cro)
	current, accessor, err = c.deploy.Ensure(parent, child, cro.Status.Generations)
	return
}

func (c *daemonSetHandler) ApplyToDeploymentObject(context *ReconcileRequestContext, cro *appsv1.RunOnceDurationOverride) deploy.Applier {
	values := c.asset.Values()

	return func(object metav1.Object) {
		if len(object.GetAnnotations()) == 0 {
			object.SetAnnotations(map[string]string{})
		}

		object.GetAnnotations()[values.ConfigurationHashAnnotationKey] = cro.Status.Hash.Configuration
		object.GetAnnotations()[values.ServingCertHashAnnotationKey] = cro.Status.Hash.ServingCert

		context.ControllerSetter().Set(object, cro)
	}
}

func (c *daemonSetHandler) ApplyToToPodTemplate(context *ReconcileRequestContext, cro *appsv1.RunOnceDurationOverride) deploy.Applier {
	values := c.asset.Values()

	return func(object metav1.Object) {
		if len(object.GetAnnotations()) == 0 {
			object.SetAnnotations(map[string]string{})
		}

		object.GetAnnotations()[values.OwnerAnnotationKey] = cro.Name
		object.GetAnnotations()[values.ConfigurationHashAnnotationKey] = cro.Status.Hash.Configuration
		object.GetAnnotations()[values.ServingCertHashAnnotationKey] = cro.Status.Hash.ServingCert
	}
}

func (c *daemonSetHandler) EnsureRBAC(reqContext *ReconcileRequestContext, in *appsv1.RunOnceDurationOverride) error {
	list := c.asset.RBAC().New()
	for _, item := range list {
		reqContext.ControllerSetter()(item.Object, in)

		var name string
		var err error

		ctx := context.TODO()
		switch obj := item.Object.(type) {
		case *corev1.ServiceAccount:
			_, _, err = resourceapply.ApplyServiceAccount(ctx, c.client.CoreV1(), c.recorder, obj)
			name = obj.Name
		case *rbacv1.Role:
			_, _, err = resourceapply.ApplyRole(ctx, c.client.RbacV1(), c.recorder, obj)
			name = obj.Name
		case *rbacv1.RoleBinding:
			_, _, err = resourceapply.ApplyRoleBinding(ctx, c.client.RbacV1(), c.recorder, obj)
			name = obj.Name
		case *rbacv1.ClusterRole:
			_, _, err = resourceapply.ApplyClusterRole(ctx, c.client.RbacV1(), c.recorder, obj)
			name = obj.Name
		case *rbacv1.ClusterRoleBinding:
			_, _, err = resourceapply.ApplyClusterRoleBinding(ctx, c.client.RbacV1(), c.recorder, obj)
			name = obj.Name
		default:
			return fmt.Errorf("unsupported RBAC resource type: %T", item.Object)
		}

		if err != nil {
			return fmt.Errorf("resource=%s failed to ensure RBAC - %s %v", item.Resource, err, item.Object)
		}

		klog.V(2).Infof("key=%s ensured RBAC resource %s", in.Name, name)
	}

	return nil
}
