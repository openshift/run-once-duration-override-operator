package targetconfigcontroller

import (
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/cert"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
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
	Handle(context *ReconcileRequestContext, original *runoncedurationoverridev1.RunOnceDurationOverride) (current *runoncedurationoverridev1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error)
}

func NewReconcileRequestContext(oc operatorruntime.OperandContext) *ReconcileRequestContext {
	return &ReconcileRequestContext{
		OperandContext: oc,
	}
}

type ReconcileRequestContext struct {
	operatorruntime.OperandContext
	bundle *cert.Bundle
}

func (r *ReconcileRequestContext) SetBundle(bundle *cert.Bundle) {
	r.bundle = bundle
}

func (r *ReconcileRequestContext) GetBundle() *cert.Bundle {
	return r.bundle
}

func (r *ReconcileRequestContext) ControllerSetter() operatorruntime.SetControllerFunc {
	return operatorruntime.SetController
}
