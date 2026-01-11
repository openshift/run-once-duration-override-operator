package runoncedurationoverride

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

const (
	ControllerName = "runoncedurationoverride"
)

type Options struct {
	ResyncPeriod            time.Duration
	Workers                 int
	Client                  *operatorruntime.Client
	RuntimeContext          operatorruntime.OperandContext
	InformerFactory         informers.SharedInformerFactory
	OperatorInformerFactory operatorinformers.SharedInformerFactory
	Recorder                events.Recorder
}

func New(options *Options) (c Interface, err error) {
	if options == nil || options.Client == nil || options.RuntimeContext == nil {
		err = errors.New("invalid input to New")
		return
	}

	// setup operand asset
	operandAsset := asset.New(options.RuntimeContext)

	// create lister(s) for secondary resources
	deployment := options.InformerFactory.Apps().V1().Deployments()
	daemonset := options.InformerFactory.Apps().V1().DaemonSets()
	pod := options.InformerFactory.Core().V1().Pods()
	configmap := options.InformerFactory.Core().V1().ConfigMaps()
	service := options.InformerFactory.Core().V1().Services()
	secret := options.InformerFactory.Core().V1().Secrets()
	serviceaccount := options.InformerFactory.Core().V1().ServiceAccounts()
	webhook := options.InformerFactory.Admissionregistration().V1().MutatingWebhookConfigurations()
	// Create RunOnceDurationOverride informer using the standard informer factory
	rodooInformer := options.OperatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides()

	secondaryLister := &SecondaryLister{
		deployment:     deployment.Lister(),
		daemonset:      daemonset.Lister(),
		pod:            pod.Lister(),
		configmap:      configmap.Lister(),
		service:        service.Lister(),
		secret:         secret.Lister(),
		serviceaccount: serviceaccount.Lister(),
		webhook:        webhook.Lister(),
	}

	lister := rodooInformer.Lister()

	// We need a queue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Add event handler to the informer
	_, err = rodooInformer.Informer().AddEventHandler(newEventHandler(queue))
	if err != nil {
		return
	}

	informers := []cache.SharedIndexInformer{
		deployment.Informer(),
		daemonset.Informer(),
		pod.Informer(),
		configmap.Informer(),
		service.Informer(),
		secret.Informer(),
		serviceaccount.Informer(),
		webhook.Informer(),
	}

	for _, informer := range informers {
		// setup watches for secondary resources
		_, err = informer.AddEventHandler(newResourceEventHandler(queue, lister, operandAsset.Values().OwnerAnnotationKey))
		if err != nil {
			return
		}
	}

	r := NewReconciler(&HandlerOptions{
		OperandContext:  options.RuntimeContext,
		Client:          options.Client,
		PrimaryLister:   lister,
		SecondaryLister: secondaryLister,
		Asset:           operandAsset,
		Recorder:        options.Recorder,
	})

	c = &runOnceDurationOverrideController{
		workers:    options.Workers,
		queue:      queue,
		informer:   rodooInformer.Informer(),
		reconciler: r,
		lister:     lister,
		done:       make(chan struct{}, 0),
	}

	return
}

type runOnceDurationOverrideController struct {
	workers    int
	queue      workqueue.RateLimitingInterface
	informer   cache.SharedIndexInformer
	reconciler controllerreconciler.Reconciler
	lister     runoncedurationoverridev1listers.RunOnceDurationOverrideLister
	done       chan struct{}
}

func (c *runOnceDurationOverrideController) Name() string {
	return ControllerName
}

func (c *runOnceDurationOverrideController) WorkerCount() int {
	return c.workers
}

func (c *runOnceDurationOverrideController) Queue() workqueue.RateLimitingInterface {
	return c.queue
}

func (c *runOnceDurationOverrideController) Informer() cache.SharedIndexInformer {
	return c.informer
}

func (c *runOnceDurationOverrideController) Reconciler() controllerreconciler.Reconciler {
	return c.reconciler
}

func (c *runOnceDurationOverrideController) Run(parent context.Context, errorCh chan<- error) {
	defer func() {
		close(c.done)
	}()

	if parent == nil {
		errorCh <- errors.New("invalid input to Run")
		return
	}

	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	klog.V(1).Infof("[controller] name=%s starting informer", c.Name())
	go c.informer.Run(parent.Done())

	klog.V(1).Infof("[controller] name=%s waiting for informer cache to sync", c.Name())
	if ok := cache.WaitForCacheSync(parent.Done(), c.informer.HasSynced); !ok {
		errorCh <- fmt.Errorf("controller=%s failed to wait for caches to sync", c.Name())
		return
	}

	for i := 0; i < c.workers; i++ {
		go c.work(parent)
	}

	klog.V(1).Infof("[controller] name=%s started %d worker(s)", c.Name(), c.workers)
	errorCh <- nil
	klog.V(1).Infof("[controller] name=%s waiting ", c.Name())

	// Not waiting for any child to finish, waiting for the parent to signal done.
	<-parent.Done()

	klog.V(1).Infof("[controller] name=%s shutting down queue", c.Name())
}

func (c *runOnceDurationOverrideController) Done() <-chan struct{} {
	return c.done
}

// work represents a worker function that pulls item(s) off of the underlying
// work queue and invokes the reconciler function associated with the controller.
func (c *runOnceDurationOverrideController) work(shutdown context.Context) {
	klog.V(1).Infof("[controller] name=%s starting to process work item(s)", c.Name())

	for c.processNextWorkItem(shutdown) {
	}

	klog.V(1).Infof("[controller] name=%s shutting down", c.Name())
}

func (c *runOnceDurationOverrideController) processNextWorkItem(shutdownCtx context.Context) bool {
	if shutdownCtx == nil {
		return false
	}

	obj, shutdown := c.queue.Get()

	if shutdown {
		return false
	}

	// We call Done here so the workqueue knows we have finished
	// processing this item. We also must remember to call Forget if we
	// do not want this work item being re-queued. For example, we do
	// not call Forget if a transient error occurs, instead the item is
	// put back on the workqueue and attempted again after a back-off
	// period.
	defer c.queue.Done(obj)

	request, ok := obj.(controllerreconciler.Request)
	if !ok {
		// As the item in the workqueue is actually invalid, we call
		// Forget here else we'd go into a loop of attempting to
		// process a work item that is invalid.
		c.queue.Forget(obj)

		runtime.HandleError(fmt.Errorf("expected reconcile.Request in workqueue but got %#v", obj))
		return true
	}

	// Run the syncHandler, passing it the namespace/name string of the
	// Foo resource to be synced.
	result, err := c.reconciler.Reconcile(shutdownCtx, request)
	if err != nil {
		// Put the item back on the workqueue to handle any transient errors.
		c.queue.AddRateLimited(request)

		runtime.HandleError(fmt.Errorf("error syncing '%s': %s, requeuing", request, err.Error()))
		return true
	}

	if result.RequeueAfter > 0 {
		// The result.RequeueAfter request will be lost, if it is returned
		// along with a non-nil error. But this is intended as
		// We need to drive to stable reconcile loops before queuing due
		// to result.RequestAfter
		c.queue.Forget(obj)
		c.queue.AddAfter(request, result.RequeueAfter)

		return true
	}

	if result.Requeue {
		c.queue.AddRateLimited(request)
		return true
	}

	// Finally, if no error occurs we Forget this item so it does not
	// get queued again until another change happens.
	c.queue.Forget(obj)
	return true
}
