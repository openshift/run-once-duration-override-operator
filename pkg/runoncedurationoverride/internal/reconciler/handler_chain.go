package reconciler

import (
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/apps/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/handlers"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Handler is an interface that wraps the Handle method
//
// Handle handles a reconciliation request based on the original RunOnceDurationOverride object.
//
// Handle may change the status block based on the operations it performs, it returns the modified object.
// It relies on the caller to update the status. Handle will not update the status block of the original object.
//
// If an error happens while handling the the request, handleErr will be set.
// On an error the request key is expected to be requeued for further retries.
//
// If result is set, the caller is expected to to requeue the request key for further retries. It indicates
// that no further processing of the request should be done, In this case the caller will abort and no
// other handler in the chain if any should be invoked.
type Handler interface {
	Handle(context *handlers.ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error)
}

// HandlerChain defines a chain of Handler(s).
// A set of Handler(s) constitute a complete reconciliation process of  RunOnceDurationOverride object.
type HandlerChain []Handler

var _ Handler = HandlerChain{}

func (h HandlerChain) Handle(context *handlers.ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, err error) {
	for _, handler := range h {
		// Invoke the handler.
		current, result, err = handler.Handle(context, original)
		if err != nil {
			// The Handler threw an error, we should reflect it in status.conditions.
			condition.NewBuilderWithStatus(&current.Status).WithError(err)

			// if there was an error, we stop further processing.
			// and requeuethe object for further retry.
			return
		}

		if result.Requeue || result.RequeueAfter > 0 {
			// the handler has asked to requeue the object.
			return
		}

		original = current
	}

	return
}
