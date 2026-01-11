package runoncedurationoverride

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

func getOwnerName(ownerAnnotationKey string, object metav1.Object) string {
	// We check for annotations and owner references
	// If both exist, owner references takes precedence.
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil && ownerRef.Kind == runoncedurationoverridev1.RunOnceDurationOverrideKind {
		return ownerRef.Name
	}

	annotations := object.GetAnnotations()
	if len(annotations) > 0 {
		owner, ok := annotations[ownerAnnotationKey]
		if ok && owner != "" {
			return owner
		}
	}

	return ""
}

func newResourceEventHandler(queue workqueue.RateLimitingInterface, lister runoncedurationoverridev1listers.RunOnceDurationOverrideLister, ownerAnnotationKey string) cache.ResourceEventHandler {
	enqueueOwner := func(obj interface{}, context string) {
		metaObj, err := runtime.GetMetaObject(obj)
		if err != nil {
			klog.Errorf("[secondarywatch] %s: invalid object, type=%T", context, obj)
			return
		}

		ownerName := getOwnerName(ownerAnnotationKey, metaObj)
		if ownerName == "" {
			klog.V(3).Infof("[secondarywatch] %s: could not find owner for %s/%s", context, metaObj.GetNamespace(), metaObj.GetName())
			return
		}

		cro, err := lister.Get(ownerName)
		if err != nil {
			klog.V(3).Infof("[secondarywatch] %s: ignoring request to enqueue - %s", context, err.Error())
			return
		}

		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: cro.GetNamespace(),
				Name:      cro.GetName(),
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
