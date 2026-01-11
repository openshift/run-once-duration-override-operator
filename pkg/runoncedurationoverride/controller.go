package runoncedurationoverride

import (
	"context"
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

const (
	ControllerName = "runoncedurationoverride"
)

type Options struct {
	ResyncPeriod    time.Duration
	Workers         int
	Client          *operatorruntime.Client
	RuntimeContext  operatorruntime.OperandContext
	InformerFactory informers.SharedInformerFactory
	ShutdownContext context.Context
	Recorder        events.Recorder
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

	// Create a new RunOnceDurationOverrides watcher
	client := options.Client.Operator
	watcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.RunOnceDurationOverrideV1().RunOnceDurationOverrides().List(context.TODO(), options)
		},

		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Watch(context.TODO(), options)
		},
	}

	// We need a queue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Bind the work queue to a cache with the help of an informer. This way we
	// make sure that whenever the cache is updated, the RunOnceDurationOverride
	// key is added to the work queue.
	// Note that when we finally process the item from the workqueue, we might
	// see a newer version of the RunOnceDurationOverride than the version which
	// was responsible for triggering the update.
	indexer, informer := cache.NewIndexerInformer(watcher, &runoncedurationoverridev1.RunOnceDurationOverride{}, options.ResyncPeriod,
		newEventHandler(queue), cache.Indexers{})

	lister := listers.NewRunOnceDurationOverrideLister(indexer)

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
	informer   cache.Controller
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

func (c *runOnceDurationOverrideController) Informer() cache.Controller {
	return c.informer
}

func (c *runOnceDurationOverrideController) Reconciler() controllerreconciler.Reconciler {
	return c.reconciler
}
