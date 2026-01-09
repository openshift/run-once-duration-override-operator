package runoncedurationoverride

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// newEventHandler returns a cache.ResourceEventHandler appropriate for
// reconciliation of RunOnceDurationOverride object(s).
func newEventHandler(queue workqueue.RateLimitingInterface) eventHandler {
	return eventHandler{
		queue: queue,
	}
}

var _ cache.ResourceEventHandler = eventHandler{}

type eventHandler struct {
	// The underlying work queue where the keys are added for reconciliation.
	queue workqueue.RateLimitingInterface
}

func (e eventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	key, err := cache.MetaNamespaceKeyFunc(obj)

	if err != nil {
		klog.Errorf("OnAdd: could not extract key, type=%T", obj)
		return
	}

	e.add(key, e.queue)
}

// OnUpdate creates UpdateEvent and calls Update on eventHandler
func (e eventHandler) OnUpdate(oldObj, newObj interface{}) {
	// We don't distinguish between an add and update.
	e.OnAdd(newObj, false)
}

func (e eventHandler) OnDelete(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	e.add(key, e.queue)
}

func (e eventHandler) add(key string, queue workqueue.RateLimitingInterface) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}

	queue.Add(request)
}
