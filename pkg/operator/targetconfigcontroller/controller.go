package targetconfigcontroller

import (
	"context"
	"fmt"
	"reflect"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	"github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/operatorclient"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/targetconfigcontroller/internal/condition"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

const (
	ControllerName = "runoncedurationoverride"
)

var (
	RunOnceDurationOverrideGVK = schema.GroupVersionKind{
		Group:   runoncedurationoverridev1.GroupName,
		Version: runoncedurationoverridev1.GroupVersion,
		Kind:    runoncedurationoverridev1.RunOnceDurationOverrideKind,
	}
)

func NewTargetConfigController(
	operatorClient versioned.Interface,
	kubeClient kubernetes.Interface,
	runtimeContext operatorruntime.OperandContext,
	informerFactory informers.SharedInformerFactory,
	operatorInformerFactory operatorinformers.SharedInformerFactory,
	recorder events.Recorder,
) factory.Controller {
	// setup operand asset
	operandAsset := asset.New(runtimeContext)

	deployInterface := deploy.NewDaemonSetInstall(
		informerFactory.Apps().V1().DaemonSets().Lister(),
		runtimeContext,
		operandAsset,
		kubeClient,
		recorder,
	)

	c := &runOnceDurationOverrideController{
		lister:         operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides().Lister(),
		client:         operatorClient,
		operandContext: runtimeContext,
		handlers: []Handler{
			NewAvailabilityHandler(operandAsset, deployInterface),
			NewValidationHandler(),
			NewConfigurationHandler(kubeClient, recorder, informerFactory.Core().V1().ConfigMaps().Lister(), operandAsset),
			NewCertGenerationHandler(kubeClient, recorder, informerFactory.Core().V1().Secrets().Lister(), informerFactory.Core().V1().ConfigMaps().Lister(), operandAsset),
			NewCertReadyHandler(kubeClient, informerFactory.Core().V1().Secrets().Lister(), informerFactory.Core().V1().ConfigMaps().Lister()),
			NewDaemonSetHandler(kubeClient, recorder, operandAsset, deployInterface),
			NewDeploymentReadyHandler(deployInterface),
			NewWebhookConfigurationHandlerHandler(kubeClient, recorder, informerFactory.Admissionregistration().V1().MutatingWebhookConfigurations().Lister(), operandAsset),
			NewAvailabilityHandler(operandAsset, deployInterface),
		},
	}

	return factory.New().WithFilteredEventsInformers(
		isOwnedByOperator,
		operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides().Informer(),
		informerFactory.Apps().V1().Deployments().Informer(),
		informerFactory.Apps().V1().DaemonSets().Informer(),
		informerFactory.Core().V1().Pods().Informer(),
		informerFactory.Core().V1().ConfigMaps().Informer(),
		informerFactory.Core().V1().Services().Informer(),
		informerFactory.Core().V1().Secrets().Informer(),
		informerFactory.Core().V1().ServiceAccounts().Informer(),
		informerFactory.Admissionregistration().V1().MutatingWebhookConfigurations().Informer(),
	).WithSync(c.sync).ToController(ControllerName, recorder)
}

type runOnceDurationOverrideController struct {
	lister         runoncedurationoverridev1listers.RunOnceDurationOverrideLister
	client         versioned.Interface
	handlers       []Handler
	operandContext operatorruntime.OperandContext
}

func (c *runOnceDurationOverrideController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	klog.V(4).Infof("key=%s new request for reconcile", operatorclient.OperatorConfigName)

	original, getErr := c.lister.Get(operatorclient.OperatorConfigName)
	if getErr != nil {
		if k8serrors.IsNotFound(getErr) {
			klog.Errorf("[reconciler] key=%s object has been deleted - %s", operatorclient.OperatorConfigName, getErr.Error())
			return nil
		}

		// Otherwise, we will requeue.
		klog.Errorf("[reconciler] key=%s unexpected error - %s", operatorclient.OperatorConfigName, getErr.Error())
		return getErr
	}

	copy := original.DeepCopy()
	copy.SetGroupVersionKind(RunOnceDurationOverrideGVK)

	reconcileContext := NewReconcileRequestContext(c.operandContext)
	modified := copy
	var current *runoncedurationoverridev1.RunOnceDurationOverride
	var err error
	var requeueRequested bool
	for _, handler := range c.handlers {
		var result controllerreconciler.Result
		current, result, err = handler.Handle(reconcileContext, modified)
		if err != nil {
			condition.NewBuilderWithStatus(&current.Status).WithError(err)
			break
		}
		if result.Requeue || result.RequeueAfter > 0 {
			requeueRequested = true
			break
		}
		modified = current
	}

	updateErr := c.updateStatus(original, current)
	if updateErr != nil {
		klog.Errorf("[reconciler] key=%s failed to update status - %s", operatorclient.OperatorConfigName, updateErr.Error())

		if err != nil {
			return fmt.Errorf("[reconciler] reconciliation error - %s -- update status error - %s", err.Error(), updateErr.Error())
		}

		return updateErr
	}

	if err != nil {
		return err
	}

	if requeueRequested {
		return fmt.Errorf("synthetic requeue request")
	}

	return nil
}

// updateStatus updates the status of a RunOnceDurationOverride resource.
// If the status inside of the desired object is equal to that of the observed then
// the function does not make an update call.
func (c *runOnceDurationOverrideController) updateStatus(observed, desired *runoncedurationoverridev1.RunOnceDurationOverride) error {
	if reflect.DeepEqual(&observed.Status, &desired.Status) {
		return nil
	}

	_, err := c.client.RunOnceDurationOverrideV1().RunOnceDurationOverrides().UpdateStatus(context.TODO(), desired, metav1.UpdateOptions{})
	return err
}
