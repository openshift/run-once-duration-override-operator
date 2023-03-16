package asset

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (a *Asset) MetricsService() *metricsService {
	return &metricsService{
		asset: a,
	}
}

type metricsService struct {
	asset *Asset
}

func (s *metricsService) Name() string {
	return s.asset.Values().Name
}

func (s *metricsService) New() *corev1.Service {
	values := s.asset.Values()

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MetricsService",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metrics",
			Namespace: values.Namespace,
			Labels: map[string]string{
				values.OwnerLabelKey: values.OwnerLabelValue,
			},
			Annotations: map[string]string{
				"include.release.openshift.io/self-managed-high-availability": "true",
				"include.release.openshift.io/single-node-developer":          "true",
				"service.alpha.openshift.io/serving-cert-secret-name":         "run-once-duration-override-operator-serving-cert",
				"exclude.release.openshift.io/internal-openshift-hosted":      "true",
				"prometheus.io/scrape": "true",
				"prometheus.io/scheme": "https",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				values.SelectorLabelKey: values.SelectorLabelValue,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       10258,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10258),
				},
			},
			Type:            corev1.ServiceTypeClusterIP,
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
}
