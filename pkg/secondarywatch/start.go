package secondarywatch

import (
	"context"

	"github.com/openshift/run-once-duration-override-operator/pkg/runtime"
	"k8s.io/client-go/informers"
)

// StarterFunc refers to a function that can be called to start watch on secondary resources.
type StarterFunc func(enqueuer runtime.Enqueuer, shutdown context.Context) error

func (s StarterFunc) Start(enqueuer runtime.Enqueuer, shutdown context.Context) error {
	return s(enqueuer, shutdown)
}

// New sets up watch on secondary resources.
// The function returns lister(s) that can be used to query secondary resources
// and a StarterFunc that can be called to start the watch.
func New(factory informers.SharedInformerFactory) (lister *Lister, startFunc StarterFunc) {

	deployment := factory.Apps().V1().Deployments()
	daemonset := factory.Apps().V1().DaemonSets()
	pod := factory.Core().V1().Pods()
	configmap := factory.Core().V1().ConfigMaps()
	service := factory.Core().V1().Services()
	secret := factory.Core().V1().Secrets()
	serviceaccount := factory.Core().V1().ServiceAccounts()
	webhook := factory.Admissionregistration().V1().MutatingWebhookConfigurations()

	startFunc = func(enqueuer runtime.Enqueuer, shutdown context.Context) error {
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

	lister = &Lister{
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
