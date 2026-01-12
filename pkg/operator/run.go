package operator

import (
	"fmt"
	"net/http"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/targetconfigcontroller"
	"github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

const (
	// DefaultCR is the name of the global cluster-scoped RunOnceDurationOverride object that
	// the operator will be watching for.
	// All other RunOnceDurationOverride object(s) are ignored since the operand is
	// basically a cluster singleton.
	DefaultCR = "cluster"

	// Default worker count is 1.
	DefaultWorkerCount = 1

	// Default ResyncPeriod for primary (RunOnceDurationOverride objects)
	DefaultResyncPeriodPrimaryResource = 1 * time.Hour

	// Default ResyncPeriod for all secondary resources that the operator manages.
	DefaultResyncPeriodSecondaryResource = 15 * time.Hour
)

func NewRunner() Interface {
	return &runner{
		done: make(chan struct{}, 0),
	}
}

type runner struct {
	done chan struct{}
}

func (r *runner) Run(config *Config, errorCh chan<- error) {
	defer func() {
		close(r.done)
		klog.V(1).Infof("[operator] exiting")
	}()

	operatorClient, err := versioned.NewForConfig(config.RestConfig)
	if err != nil {
		errorCh <- fmt.Errorf("failed to construct client for apps.openshift.io - %s", err.Error())
		return
	}

	kubeClient, err := kubernetes.NewForConfig(config.RestConfig)
	if err != nil {
		errorCh <- fmt.Errorf("failed to construct client for kubernetes - %s", err.Error())
		return
	}

	context := runtime.NewOperandContext(config.Name, config.Namespace, DefaultCR, config.OperandImage, config.OperandVersion)

	// create informer factory for secondary resources
	kubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		DefaultResyncPeriodSecondaryResource,
		informers.WithNamespace(config.Namespace),
	)

	// create informer factory for primary resource
	operatorInformerFactory := operatorinformers.NewSharedInformerFactory(
		operatorClient,
		DefaultResyncPeriodPrimaryResource,
	)

	// create recorder for resource apply operations
	recorder := events.NewLoggingEventRecorder(config.Name, clock.RealClock{})

	// start the controllers
	c := targetconfigcontroller.NewTargetConfigController(
		operatorClient,
		kubeClient,
		context,
		kubeInformerFactory,
		operatorInformerFactory,
		recorder,
	)

	// start informer factory for secondary resources
	kubeInformerFactory.Start(config.ShutdownContext.Done())

	// start informer factory for primary resource
	operatorInformerFactory.Start(config.ShutdownContext.Done())

	// Serve a simple HTTP health check.
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	go http.ListenAndServe(":8080", healthMux)

	errorCh <- nil
	klog.V(1).Infof("operator is starting controller")

	go c.Run(config.ShutdownContext, DefaultWorkerCount)

	<-config.ShutdownContext.Done()
}

func (r *runner) Done() <-chan struct{} {
	return r.done
}
