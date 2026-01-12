package runoncedurationoverride

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/controller/factory"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/operatorclient"
	"github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

func isOwnedByOperator(obj interface{}) bool {
	// Check if obj is a RunOnceDurationOverride
	if rodoo, ok := obj.(*runoncedurationoverridev1.RunOnceDurationOverride); ok {
		return rodoo.Name == operatorclient.OperatorConfigName
	}

	// For other types, check if they are owned by the operator
	metaObj, err := runtime.GetMetaObject(obj)
	if err != nil {
		klog.V(4).Infof("failed to get meta object: %v", err)
		return false
	}

	ownerName := getOwnerName(metaObj)
	return ownerName == operatorclient.OperatorConfigName
}

var _ factory.EventFilterFunc = isOwnedByOperator

// newEventHandler returns a cache.ResourceEventHandler appropriate for
// reconciliation of RunOnceDurationOverride object(s).
func newEventHandler(queue workqueue.RateLimitingInterface) cache.ResourceEventHandler {
	enqueue := func(obj interface{}) {
		if !isOwnedByOperator(obj) {
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

	add := func(obj interface{}) {
		_, err := cache.MetaNamespaceKeyFunc(obj)
		if err != nil {
			klog.Errorf("could not extract key, type=%T", obj)
			return
		}
		enqueue(obj)
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
			_, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			enqueue(obj)
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
		if !isOwnedByOperator(obj) {
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
