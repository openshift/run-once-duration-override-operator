package targetconfigcontroller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

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
