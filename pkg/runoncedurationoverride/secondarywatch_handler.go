package runoncedurationoverride

import (
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

func newResourceEventHandler(enqueuer runtime.Enqueuer) cache.ResourceEventHandler {
	enqueueOwner := func(obj interface{}, context string) {
		metaObj, err := runtime.GetMetaObject(obj)
		if err != nil {
			klog.Errorf("[secondarywatch] %s: invalid object, type=%T", context, obj)
			return
		}

		if err := enqueuer.Enqueue(metaObj); err != nil {
			klog.V(3).Infof("[secondarywatch] %s: failed to enqueue owner - %s", context, err.Error())
		}
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

			if err := enqueuer.Enqueue(newMetaObj); err != nil {
				klog.V(3).Infof("[secondarywatch] OnUpdate: failed to enqueue owner - %s", err.Error())
			}
		},
		DeleteFunc: func(obj interface{}) {
			enqueueOwner(obj, "OnDelete")
		},
	}
}
