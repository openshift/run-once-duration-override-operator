package targetconfigcontroller

import (
	"context"
	"fmt"

	operatorv1 "github.com/openshift/api/operator/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/operatorclient"
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
	operatorClient *operatorclient.RunOnceDurationOverrideClient,
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
		operatorClient: operatorClient,
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
	operatorClient *operatorclient.RunOnceDurationOverrideClient
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
		var handlerErr error
		current, result, handlerErr = handler.Handle(reconcileContext, modified)

		if handlerErr != nil {
			err = handlerErr
			break
		}

		if result.Requeue || result.RequeueAfter > 0 {
			requeueRequested = true
			break
		}
		modified = current
	}

	// Capture the complete status with all custom fields that handlers have set
	statusToApply := current.Status.DeepCopy()

	// Add/update conditions based on reconciliation result
	if err != nil {
		reason := GetReason(err)
		if reason == "" {
			reason = "ReconciliationError"
		}

		v1helpers.SetOperatorCondition(&statusToApply.OperatorStatus.Conditions, operatorv1.OperatorCondition{
			Type:    GetConditionType(err),
			Status:  GetStatus(err),
			Reason:  reason,
			Message: err.Error(),
		})
	} else {
		v1helpers.SetOperatorCondition(&statusToApply.OperatorStatus.Conditions, operatorv1.OperatorCondition{
			Type:   runoncedurationoverridev1.InstallReadinessFailure,
			Status: operatorv1.ConditionFalse,
		})
		v1helpers.SetOperatorCondition(&statusToApply.OperatorStatus.Conditions, operatorv1.OperatorCondition{
			Type:   "Available",
			Status: operatorv1.ConditionTrue,
		})
	}

	// Build status update function that applies the complete status including custom fields
	statusUpdateFuncs := []operatorclient.UpdateRunOnceDurationOverrideStatusFunc{
		func(status *runoncedurationoverridev1.RunOnceDurationOverrideStatus) error {
			*status = *statusToApply
			return nil
		},
	}

	// Update status using custom UpdateStatus which handles retries and conflicts
	_, _, updateErr := operatorclient.UpdateStatus(ctx, c.operatorClient, statusUpdateFuncs...)
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
