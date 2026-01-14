package operator

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/operatorclient"
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

	// OperandImageEnvName is the environment variable name for the operand image
	OperandImageEnvName = "RELATED_IMAGE_OPERAND_IMAGE"

	// OperandVersionEnvName is the environment variable name for the operand version
	OperandVersionEnvName = "OPERAND_VERSION"
)

func RunOperator(ctx context.Context, cc *controllercmd.ControllerContext) error {
	defer klog.V(1).Infof("[operator] exiting")

	operandImage := os.Getenv(OperandImageEnvName)
	if operandImage == "" {
		return fmt.Errorf("%s environment variable must be set", OperandImageEnvName)
	}

	operandVersion := os.Getenv(OperandVersionEnvName)
	if operandVersion == "" {
		return fmt.Errorf("%s environment variable must be set", OperandVersionEnvName)
	}

	operatorClient, err := versioned.NewForConfig(cc.KubeConfig)
	if err != nil {
		return fmt.Errorf("failed to construct client for apps.openshift.io - %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return fmt.Errorf("failed to construct client for kubernetes - %s", err.Error())
	}

	operandContext := runtime.NewOperandContext(operatorclient.OperatorName, operatorclient.OperatorNamespace, DefaultCR, operandImage, operandVersion)

	// create informer factory for secondary resources
	kubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		DefaultResyncPeriodSecondaryResource,
		informers.WithNamespace(operatorclient.OperatorNamespace),
	)

	// create informer factory for primary resource
	operatorInformerFactory := operatorinformers.NewSharedInformerFactory(
		operatorClient,
		DefaultResyncPeriodPrimaryResource,
	)

	// create recorder for resource apply operations
	recorder := events.NewLoggingEventRecorder(operatorclient.OperatorName, clock.RealClock{})

	// start the controllers
	c := targetconfigcontroller.NewTargetConfigController(
		&operatorclient.RunOnceDurationOverrideClient{
			Ctx:                             ctx,
			RunOnceDurationOverrideInformer: operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides(),
			OperatorClient:                  operatorClient.RunOnceDurationOverrideV1(),
		},
		kubeClient,
		operandContext,
		kubeInformerFactory,
		operatorInformerFactory,
		recorder,
	)

	// start informer factory for secondary resources
	kubeInformerFactory.Start(ctx.Done())

	// start informer factory for primary resource
	operatorInformerFactory.Start(ctx.Done())

	// Serve a simple HTTP health check.
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	go http.ListenAndServe(":8080", healthMux)

	klog.V(1).Infof("operator is starting controllers")

	// start target config controller
	go c.Run(ctx, DefaultWorkerCount)

	<-ctx.Done()
	return nil
}
