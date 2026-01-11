package runoncedurationoverride

import (
	"context"
	"fmt"
	"reflect"

	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	"github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	RunOnceDurationOverrideGVK = schema.GroupVersionKind{
		Group:   runoncedurationoverridev1.GroupName,
		Version: runoncedurationoverridev1.GroupVersion,
		Kind:    runoncedurationoverridev1.RunOnceDurationOverrideKind,
	}
)

func NewReconciler(options *HandlerOptions) *reconciler {
	options.Deploy = deploy.NewDaemonSetInstall(
		options.SecondaryLister.AppsV1DaemonSetLister(),
		options.OperandContext,
		options.Asset,
		options.Client.Kubernetes,
		options.Recorder,
	)

	return &reconciler{
		client: options.Client.Operator,
		lister: options.PrimaryLister,
		handlers: []Handler{
			NewAvailabilityHandler(options),
			NewValidationHandler(options),
			NewConfigurationHandler(options),
			NewCertGenerationHandler(options),
			NewCertReadyHandler(options),
			NewDaemonSetHandler(options),
			NewDeploymentReadyHandler(options),
			NewWebhookConfigurationHandlerHandler(options),
			NewAvailabilityHandler(options),
		},
		operandContext: options.OperandContext,
	}
}

type reconciler struct {
	client         versioned.Interface
	lister         runoncedurationoverridev1listers.RunOnceDurationOverrideLister
	handlers       []Handler
	operandContext operatorruntime.OperandContext
}

func (r *reconciler) Reconcile(ctx context.Context, request controllerreconciler.Request) (result controllerreconciler.Result, err error) {
	klog.V(4).Infof("key=%s new request for reconcile", request.Name)

	result = controllerreconciler.Result{}

	// The operand is a singleton, so we are only interested in the CR specified in cluster
	if request.Name != r.operandContext.ResourceName() {
		klog.V(2).Infof("key=%s skipping reconcile", request.Name)
		return
	}

	original, getErr := r.lister.Get(request.Name)
	if getErr != nil {
		if k8serrors.IsNotFound(getErr) {
			klog.Errorf("[reconciler] key=%s object has been deleted - %s", request.Name, getErr.Error())
			return
		}

		// Otherwise, we will requeue.
		klog.Errorf("[reconciler] key=%s unexpected error - %s", request.Name, getErr.Error())
		err = getErr
		return
	}

	copy := original.DeepCopy()
	copy.SetGroupVersionKind(RunOnceDurationOverrideGVK)

	reconcileContext := NewReconcileRequestContext(r.operandContext)
	modified := copy
	var current *runoncedurationoverridev1.RunOnceDurationOverride
	for _, handler := range r.handlers {
		current, result, err = handler.Handle(reconcileContext, modified)
		if err != nil {
			condition.NewBuilderWithStatus(&current.Status).WithError(err)
			break
		}
		if result.Requeue || result.RequeueAfter > 0 {
			break
		}
		modified = current
	}

	updateErr := r.updateStatus(original, current)
	if updateErr != nil {
		klog.Errorf("[reconciler] key=%s failed to update status - %s", request.Name, updateErr.Error())

		if err != nil {
			err = fmt.Errorf("[reconciler] reconciliation error - %s -- update status error - %s", err.Error(), updateErr.Error())
			return
		}

		err = updateErr
	}

	return
}

// updateStatus updates the status of a RunOnceDurationOverride resource.
// If the status inside of the desired object is equal to that of the observed then
// the function does not make an update call.
func (r *reconciler) updateStatus(observed, desired *runoncedurationoverridev1.RunOnceDurationOverride) error {
	if reflect.DeepEqual(&observed.Status, &desired.Status) {
		return nil
	}

	_, err := r.client.RunOnceDurationOverrideV1().RunOnceDurationOverrides().UpdateStatus(context.TODO(), desired, metav1.UpdateOptions{})
	return err
}
