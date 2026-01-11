package runoncedurationoverride

import (
	"github.com/openshift/run-once-duration-override-operator/pkg/runtime"
	"k8s.io/client-go/informers"
)

// SecondaryStarterFunc refers to a function that can be called to start watch on secondary resources.
type SecondaryStarterFunc func(enqueuer runtime.Enqueuer) error

func (s SecondaryStarterFunc) Start(enqueuer runtime.Enqueuer) error {
	return s(enqueuer)
}

// NewSecondaryWatch sets up watch on secondary resources.
// The function returns lister(s) that can be used to query secondary resources
// and a SecondaryStarterFunc that can be called to start the watch.
func NewSecondaryWatch(factory informers.SharedInformerFactory) (lister *SecondaryLister, startFunc SecondaryStarterFunc) {

	deployment := factory.Apps().V1().Deployments()
	daemonset := factory.Apps().V1().DaemonSets()
	pod := factory.Core().V1().Pods()
	configmap := factory.Core().V1().ConfigMaps()
	service := factory.Core().V1().Services()
	secret := factory.Core().V1().Secrets()
	serviceaccount := factory.Core().V1().ServiceAccounts()
	webhook := factory.Admissionregistration().V1().MutatingWebhookConfigurations()

	startFunc = func(enqueuer runtime.Enqueuer) error {
		handler := newResourceEventHandler(enqueuer)

		_, err := deployment.Informer().AddEventHandler(handler)
		if err != nil {
			return err
		}
		_, err = daemonset.Informer().AddEventHandler(handler)
		if err != nil {
			return err
		}
		_, err = pod.Informer().AddEventHandler(handler)
		if err != nil {
			return err
		}
		_, err = configmap.Informer().AddEventHandler(handler)
		if err != nil {
			return err
		}
		_, err = service.Informer().AddEventHandler(handler)
		if err != nil {
			return err
		}
		_, err = secret.Informer().AddEventHandler(handler)
		if err != nil {
			return err
		}
		_, err = serviceaccount.Informer().AddEventHandler(handler)
		if err != nil {
			return err
		}
		_, err = webhook.Informer().AddEventHandler(handler)
		if err != nil {
			return err
		}

		return nil
	}

	lister = &SecondaryLister{
		deployment:     deployment.Lister(),
		daemonset:      daemonset.Lister(),
		pod:            pod.Lister(),
		configmap:      configmap.Lister(),
		service:        service.Lister(),
		secret:         secret.Lister(),
		serviceaccount: serviceaccount.Lister(),
		webhook:        webhook.Lister(),
	}

	return
}
