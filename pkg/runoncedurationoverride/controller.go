package runoncedurationoverride

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	"github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/condition"
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

func New(
	workers int,
	operatorClient versioned.Interface,
	kubeClient kubernetes.Interface,
	runtimeContext operatorruntime.OperandContext,
	informerFactory informers.SharedInformerFactory,
	operatorInformerFactory operatorinformers.SharedInformerFactory,
	recorder events.Recorder,
) (c *runOnceDurationOverrideController, err error) {
	if operatorClient == nil || kubeClient == nil || runtimeContext == nil {
		err = errors.New("invalid input to New")
		return
	}

	// setup operand asset
	operandAsset := asset.New(runtimeContext)

	// We need a queue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Add event handler to the informer
	_, err = operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides().Informer().AddEventHandler(newEventHandler(queue))
	if err != nil {
		return
	}

	informers := []cache.SharedIndexInformer{
		informerFactory.Apps().V1().Deployments().Informer(),
		informerFactory.Apps().V1().DaemonSets().Informer(),
		informerFactory.Core().V1().Pods().Informer(),
		informerFactory.Core().V1().ConfigMaps().Informer(),
		informerFactory.Core().V1().Services().Informer(),
		informerFactory.Core().V1().Secrets().Informer(),
		informerFactory.Core().V1().ServiceAccounts().Informer(),
		informerFactory.Admissionregistration().V1().MutatingWebhookConfigurations().Informer(),
	}

	for _, informer := range informers {
		// setup watches for secondary resources
		_, err = informer.AddEventHandler(newResourceEventHandler(queue, operandAsset.Values().OwnerAnnotationKey))
		if err != nil {
			return
		}
	}

	deployInterface := deploy.NewDaemonSetInstall(
		informerFactory.Apps().V1().DaemonSets().Lister(),
		runtimeContext,
		operandAsset,
		kubeClient,
		recorder,
	)

	c = &runOnceDurationOverrideController{
		workers:        workers,
		queue:          queue,
		informer:       operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides().Informer(),
		lister:         operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides().Lister(),
		done:           make(chan struct{}, 0),
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

	return
}

type runOnceDurationOverrideController struct {
	workers        int
	queue          workqueue.RateLimitingInterface
	informer       cache.SharedIndexInformer
	lister         runoncedurationoverridev1listers.RunOnceDurationOverrideLister
	done           chan struct{}
	client         versioned.Interface
	handlers       []Handler
	operandContext operatorruntime.OperandContext
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

	klog.V(1).Infof("[controller] name=%s starting informer", ControllerName)
	go c.informer.Run(parent.Done())

	klog.V(1).Infof("[controller] name=%s waiting for informer cache to sync", ControllerName)
	if ok := cache.WaitForCacheSync(parent.Done(), c.informer.HasSynced); !ok {
		errorCh <- fmt.Errorf("controller=%s failed to wait for caches to sync", ControllerName)
		return
	}

	for i := 0; i < c.workers; i++ {
		go c.work(parent)
	}

	klog.V(1).Infof("[controller] name=%s started %d worker(s)", ControllerName, c.workers)
	errorCh <- nil
	klog.V(1).Infof("[controller] name=%s waiting ", ControllerName)

	// Not waiting for any child to finish, waiting for the parent to signal done.
	<-parent.Done()

	klog.V(1).Infof("[controller] name=%s shutting down queue", ControllerName)
}

func (c *runOnceDurationOverrideController) Done() <-chan struct{} {
	return c.done
}

// work represents a worker function that pulls item(s) off of the underlying
// work queue and invokes the reconciler function associated with the controller.
func (c *runOnceDurationOverrideController) work(shutdown context.Context) {
	klog.V(1).Infof("[controller] name=%s starting to process work item(s)", ControllerName)

	for c.processNextWorkItem(shutdown) {
	}

	klog.V(1).Infof("[controller] name=%s shutting down", ControllerName)
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
	result, err := c.Reconcile(shutdownCtx, request)
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

func (c *runOnceDurationOverrideController) Reconcile(ctx context.Context, request controllerreconciler.Request) (result controllerreconciler.Result, err error) {
	klog.V(4).Infof("key=%s new request for reconcile", request.Name)

	result = controllerreconciler.Result{}

	// The operand is a singleton, so we are only interested in the CR specified in cluster
	if request.Name != c.operandContext.ResourceName() {
		klog.V(2).Infof("key=%s skipping reconcile", request.Name)
		return
	}

	original, getErr := c.lister.Get(request.Name)
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

	reconcileContext := NewReconcileRequestContext(c.operandContext)
	modified := copy
	var current *runoncedurationoverridev1.RunOnceDurationOverride
	for _, handler := range c.handlers {
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

	updateErr := c.updateStatus(original, current)
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
func (c *runOnceDurationOverrideController) updateStatus(observed, desired *runoncedurationoverridev1.RunOnceDurationOverride) error {
	if reflect.DeepEqual(&observed.Status, &desired.Status) {
		return nil
	}

	_, err := c.client.RunOnceDurationOverrideV1().RunOnceDurationOverrides().UpdateStatus(context.TODO(), desired, metav1.UpdateOptions{})
	return err
}
