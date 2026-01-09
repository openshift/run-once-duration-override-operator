package runoncedurationoverride

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// NewRunner returns a new instance of runnerImpl.
func NewRunner() *runnerImpl {
	return &runnerImpl{
		done: make(chan struct{}, 0),
	}
}

type runnerImpl struct {
	done chan struct{}
}

func (r *runnerImpl) Run(parent context.Context, controller Interface, errorCh chan<- error) {
	defer func() {
		close(r.done)
	}()

	if parent == nil || controller == nil {
		errorCh <- errors.New("invalid input to runnerImpl.Run")
		return
	}

	defer utilruntime.HandleCrash()
	defer controller.Queue().ShutDown()

	klog.V(1).Infof("[controller] name=%s starting informer", controller.Name())
	go controller.Informer().Run(parent.Done())

	klog.V(1).Infof("[controller] name=%s waiting for informer cache to sync", controller.Name())
	if ok := cache.WaitForCacheSync(parent.Done(), controller.Informer().HasSynced); !ok {
		errorCh <- fmt.Errorf("controller=%s failed to wait for caches to sync", controller.Name())
		return
	}

	for i := 0; i < controller.WorkerCount(); i++ {
		go r.work(parent, controller)
	}

	klog.V(1).Infof("[controller] name=%s started %d worker(s)", controller.Name(), controller.WorkerCount())
	errorCh <- nil
	klog.V(1).Infof("[controller] name=%s waiting ", controller.Name())

	// Not waiting for any child to finish, waiting for the parent to signal done.
	<-parent.Done()

	klog.V(1).Infof("[controller] name=%s shutting down queue", controller.Name())
}

func (r *runnerImpl) Done() <-chan struct{} {
	return r.done
}

// work represents a worker function that pulls item(s) off of the underlying
// work queue and invokes the reconciler function associated with the controller.
func (r *runnerImpl) work(shutdown context.Context, controller Interface) {
	klog.V(1).Infof("[controller] name=%s starting to process work item(s)", controller.Name())

	for r.processNextWorkItem(shutdown, controller) {
	}

	klog.V(1).Infof("[controller] name=%s shutting down", controller.Name())
}

func (r *runnerImpl) processNextWorkItem(shutdownCtx context.Context, controller Interface) bool {
	if shutdownCtx == nil || controller == nil {
		return false
	}

	obj, shutdown := controller.Queue().Get()

	if shutdown {
		return false
	}

	// We call Done here so the workqueue knows we have finished
	// processing this item. We also must remember to call Forget if we
	// do not want this work item being re-queued. For example, we do
	// not call Forget if a transient error occurs, instead the item is
	// put back on the workqueue and attempted again after a back-off
	// period.
	defer controller.Queue().Done(obj)

	request, ok := obj.(reconcile.Request)
	if !ok {
		// As the item in the workqueue is actually invalid, we call
		// Forget here else we'd go into a loop of attempting to
		// process a work item that is invalid.
		controller.Queue().Forget(obj)

		utilruntime.HandleError(fmt.Errorf("expected reconcile.Request in workqueue but got %#v", obj))
		return true
	}

	// Run the syncHandler, passing it the namespace/name string of the
	// Foo resource to be synced.
	result, err := controller.Reconciler().Reconcile(shutdownCtx, request)
	if err != nil {
		// Put the item back on the workqueue to handle any transient errors.
		controller.Queue().AddRateLimited(request)

		utilruntime.HandleError(fmt.Errorf("error syncing '%s': %s, requeuing", request, err.Error()))
		return true
	}

	if result.RequeueAfter > 0 {
		// The result.RequeueAfter request will be lost, if it is returned
		// along with a non-nil error. But this is intended as
		// We need to drive to stable reconcile loops before queuing due
		// to result.RequestAfter
		controller.Queue().Forget(obj)
		controller.Queue().AddAfter(request, result.RequeueAfter)

		return true
	}

	if result.Requeue {
		controller.Queue().AddRateLimited(request)
		return true
	}

	// Finally, if no error occurs we Forget this item so it does not
	// get queued again until another change happens.
	controller.Queue().Forget(obj)
	return true
}
