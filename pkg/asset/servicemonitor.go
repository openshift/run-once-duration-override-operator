package asset

import (
	scvMonV1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *Asset) ServiceMonitor() *serviceMonitor {
	return &serviceMonitor{
		asset: a,
	}
}

type serviceMonitor struct {
	asset *Asset
}

func (s *serviceMonitor) Name() string {
	return s.asset.Values().Name
}

func (s *serviceMonitor) New() *scvMonV1.ServiceMonitor{
	values := s.asset.Values()

	return &scvMonV1.ServiceMonitor{
		TypeMeta: v1.TypeMeta{
			Kind: "ServiceMonitor",
			APIVersion: "monitoring.coreos.com/v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "run-once-duration-override-operator",
			Namespace: values.Namespace,
			Annotations: map[string]string{
				"exclude.release.openshift.io/internal-openshift-hosted": "true",
				"include.release.openshift.io/self-managed-high-availability": "true",
				"include.release.openshift.io/single-node-developer": "true",
			},
		},
		Spec: scvMonV1.ServiceMonitorSpec{
			Endpoints: []scvMonV1.Endpoint{
				{
					BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
					Path: "/metrics",
					Port: "https",
					Scheme: "https",
					TLSConfig: &scvMonV1.TLSConfig{
						CAFile: "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
						SafeTLSConfig: scvMonV1.SafeTLSConfig{
							ServerName: "metrics.openshift-run-once-duration-override-operator.svc",
						},
					},
				},
			},
			NamespaceSelector: scvMonV1.NamespaceSelector{
				MatchNames: []string{
					"run-once-duration-override-operator",
				},
			},
			Selector: v1.LabelSelector{
				MatchLabels: map[string]string{
					"runoncedurationoverride.operator": "true",
				},
			},
		},
	}
}
