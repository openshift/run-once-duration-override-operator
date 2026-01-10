package runoncedurationoverride

import (
	"context"
	"errors"
	"time"

	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/handlers"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride/internal/reconciler"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
	"github.com/openshift/run-once-duration-override-operator/pkg/secondarywatch"
)

const (
	ControllerName = "runoncedurationoverride"
)

type Options struct {
	ResyncPeriod   time.Duration
	Workers        int
	Client         *operatorruntime.Client
	RuntimeContext operatorruntime.OperandContext
	Lister         *secondarywatch.Lister
	Recorder       events.Recorder
}

func New(options *Options) (c Interface, e operatorruntime.Enqueuer, err error) {
	if options == nil || options.Client == nil || options.RuntimeContext == nil {
		err = errors.New("invalid input to New")
		return
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

	// initialize install strategy, we use daemonset
	d := deploy.NewDaemonSetInstall(options.Lister.AppsV1DaemonSetLister(), options.RuntimeContext, operandAsset, options.Client.Kubernetes, options.Recorder)

	reconciler := reconciler.NewReconciler(&handlers.Options{
		OperandContext:  options.RuntimeContext,
		Client:          options.Client,
		PrimaryLister:   lister,
		SecondaryLister: options.Lister,
		Asset:           operandAsset,
		Deploy:          d,
	})

	c = &runOnceDurationOverrideController{
		workers:    options.Workers,
		queue:      queue,
		informer:   informer,
		reconciler: reconciler,
		lister:     lister,
	}
	e = &enqueuer{
		queue:              queue,
		lister:             lister,
		ownerAnnotationKey: operandAsset.Values().OwnerAnnotationKey,
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
