package runoncedurationoverride

import (
	"errors"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
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

	// create lister(s) for secondary resources
	deployment := options.InformerFactory.Apps().V1().Deployments()
	daemonset := options.InformerFactory.Apps().V1().DaemonSets()
	pod := options.InformerFactory.Core().V1().Pods()
	configmap := options.InformerFactory.Core().V1().ConfigMaps()
	service := options.InformerFactory.Core().V1().Services()
	secret := options.InformerFactory.Core().V1().Secrets()
	serviceaccount := options.InformerFactory.Core().V1().ServiceAccounts()
	webhook := options.InformerFactory.Admissionregistration().V1().MutatingWebhookConfigurations()

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

	// We need a queue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Create RunOnceDurationOverride informer using the standard informer factory
	rodooInformer := options.OperatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides()

	// Add event handler to the informer
	_, err = rodooInformer.Informer().AddEventHandler(newEventHandler(queue))
	if err != nil {
		return
	}

	lister := rodooInformer.Lister()
	informer := rodooInformer.Informer()

	// setup operand asset
	operandAsset := asset.New(options.RuntimeContext)

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
		informer:   informer,
		reconciler: r,
		lister:     lister,
	}

	// setup watches for secondary resources
	handler := newResourceEventHandler(queue, lister, operandAsset.Values().OwnerAnnotationKey)

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
		_, err = informer.AddEventHandler(handler)
		if err != nil {
			return
		}
	}

	return
}

type runOnceDurationOverrideController struct {
	workers    int
	queue      workqueue.RateLimitingInterface
	informer   cache.SharedIndexInformer
	reconciler controllerreconciler.Reconciler
	lister     runoncedurationoverridev1listers.RunOnceDurationOverrideLister
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
