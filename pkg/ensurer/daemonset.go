package ensurer

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/run-once-duration-override-operator/pkg/dynamic"
)

func NewDaemonSetEnsurer(client dynamic.Ensurer) *DaemonSetEnsurer {
	return &DaemonSetEnsurer{
		client: client,
	}
}

type DaemonSetEnsurer struct {
	client dynamic.Ensurer
}

func (e *DaemonSetEnsurer) Ensure(ds *appsv1.DaemonSet) (current *appsv1.DaemonSet, err error) {
	unstructured, errGot := e.client.Ensure("daemonsets", ds)
	if errGot != nil {
		err = errGot
		return
	}

	current = &appsv1.DaemonSet{}
	if conversionErr := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.UnstructuredContent(), current); conversionErr != nil {
		err = conversionErr
		return
	}

	return
}
