package runoncedurationoverride

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/operatorclient"
	"github.com/openshift/run-once-duration-override-operator/pkg/runtime"
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

func getOwnerName(object metav1.Object) string {
	// We check for annotations and owner references
	// If both exist, owner references takes precedence.
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil && ownerRef.Kind == runoncedurationoverridev1.RunOnceDurationOverrideKind {
		return ownerRef.Name
	}

	annotations := object.GetAnnotations()
	if len(annotations) > 0 {
		owner, ok := annotations[operatorclient.OperatorOwnerAnnotation]
		if ok && owner != "" {
			return owner
		}
	}

	return ""
}

func newResourceEventHandler(queue workqueue.RateLimitingInterface) cache.ResourceEventHandler {
	enqueueOwner := func(obj interface{}, context string) {
		metaObj, err := runtime.GetMetaObject(obj)
		if err != nil {
			klog.Errorf("[secondarywatch] %s: invalid object, type=%T", context, obj)
			return
		}

		ownerName := getOwnerName(metaObj)
		if ownerName == "" {
			klog.V(3).Infof("[secondarywatch] %s: could not find owner for %s/%s", context, metaObj.GetNamespace(), metaObj.GetName())
			return
		}

		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      operatorclient.OperatorConfigName,
			},
		}

		queue.Add(request)
	}

	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			enqueueOwner(obj, "OnAdd")
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldMetaObj, err := runtime.GetMetaObject(oldObj)
			if err != nil {
				klog.Errorf("[secondarywatch] OnUpdate: invalid object, type=%T", oldObj)
				return
			}

			newMetaObj, err := runtime.GetMetaObject(newObj)
			if err != nil {
				klog.Errorf("[secondarywatch] OnUpdate: invalid object, type=%T", newObj)
				return
			}

			if oldMetaObj.GetResourceVersion() == newMetaObj.GetResourceVersion() {
				klog.V(3).Infof("[secondarywatch] OnUpdate: resource version has not changed, not going to enqueue owner, type=%T resource-version=%s", newMetaObj, newMetaObj.GetResourceVersion())
				return
			}

			enqueueOwner(newMetaObj, "OnUpdate")
		},
		DeleteFunc: func(obj interface{}) {
			enqueueOwner(obj, "OnDelete")
		},
	}
}
