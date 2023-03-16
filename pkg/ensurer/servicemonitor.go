package ensurer

import (
	scvMonV1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/run-once-duration-override-operator/pkg/dynamic"
)

func NewServiceMonitorEnsurer(client dynamic.Ensurer) *ServiceMonitorEnsurer {
	return &ServiceMonitorEnsurer{
		client: client,
	}
}

type ServiceMonitorEnsurer struct {
	client dynamic.Ensurer
}

func (s *ServiceMonitorEnsurer) Ensure(servicemonitor *scvMonV1.ServiceMonitor) (current *scvMonV1.ServiceMonitor, err error) {
	unstructured, errGot := s.client.Ensure("servicemonitors", servicemonitor)
	if errGot != nil {
		err = errGot
		return
	}

	current = &scvMonV1.ServiceMonitor{}
	if conversionErr := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.UnstructuredContent(), current); conversionErr != nil {
		err = conversionErr
		return
	}

	return
}
