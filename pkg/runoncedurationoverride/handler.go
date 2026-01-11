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
func newEventHandler(queue workqueue.RateLimitingInterface) cache.ResourceEventHandler {
	enqueue := func(key string) {
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

	add := func(obj interface{}) {
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err != nil {
			klog.Errorf("could not extract key, type=%T", obj)
			return
		}
		enqueue(key)
	}

	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			add(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			// We don't distinguish between an add and update.
			add(newObj)
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			enqueue(key)
		},
	}
}
