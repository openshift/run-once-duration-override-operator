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

	configclient "github.com/openshift/client-go/config/clientset/versioned"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/configobservation/configobservercontroller"
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

	kubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		DefaultResyncPeriodSecondaryResource,
		informers.WithNamespace(operatorclient.OperatorNamespace),
	)

	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(kubeClient,
		"",
		operatorclient.OperatorNamespace,
	)

	operatorInformerFactory := operatorinformers.NewSharedInformerFactory(
		operatorClient,
		DefaultResyncPeriodPrimaryResource,
	)

	configClient, err := configclient.NewForConfig(cc.KubeConfig)
	if err != nil {
		return fmt.Errorf("failed to construct config client: %s", err.Error())
	}
	configInformers := configinformers.NewSharedInformerFactory(configClient, 10*time.Minute)

	recorder := events.NewLoggingEventRecorder(operatorclient.OperatorName, clock.RealClock{})

	runOnceDurationOverrideClient := &operatorclient.RunOnceDurationOverrideClient{
		Ctx:                             ctx,
		RunOnceDurationOverrideInformer: operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides(),
		OperatorClient:                  operatorClient.RunOnceDurationOverrideV1(),
	}

	operatorClientWrapper := operatorclient.NewOperatorClientWrapper(runOnceDurationOverrideClient)

	resourceSyncController := resourcesynccontroller.NewResourceSyncController(
		"RunOnceDurationOverrideOperator",
		operatorClientWrapper,
		kubeInformersForNamespaces,
		kubeClient.CoreV1(),
		kubeClient.CoreV1(),
		recorder,
	)

	configObserver := configobservercontroller.NewConfigObserver(
		operatorClientWrapper,
		configInformers,
		resourceSyncController,
		recorder,
	)

	c := targetconfigcontroller.NewTargetConfigController(
		runOnceDurationOverrideClient,
		kubeClient,
		operandContext,
		kubeInformerFactory,
		operatorInformerFactory,
		recorder,
	)

	kubeInformerFactory.Start(ctx.Done())
	kubeInformersForNamespaces.Start(ctx.Done())
	operatorInformerFactory.Start(ctx.Done())
	configInformers.Start(ctx.Done())

	// Serve a simple HTTP health check.
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	go http.ListenAndServe(":8080", healthMux)

	klog.V(1).Infof("operator is starting controllers")

	go resourceSyncController.Run(ctx, 1)
	go configObserver.Run(ctx, 1)
	go c.Run(ctx, DefaultWorkerCount)

	<-ctx.Done()
	return nil
}
