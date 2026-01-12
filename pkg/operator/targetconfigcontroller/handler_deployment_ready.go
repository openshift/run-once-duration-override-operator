package targetconfigcontroller

import (
	"fmt"

	"k8s.io/klog/v2"
	controllerreconciler "sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/targetconfigcontroller/internal/condition"
)

func NewDeploymentReadyHandler(deploy deploy.Interface) *deploymentReadyHandler {
	return &deploymentReadyHandler{
		deploy: deploy,
	}
}

type deploymentReadyHandler struct {
	deploy deploy.Interface
}

func (c *deploymentReadyHandler) Handle(context *ReconcileRequestContext, original *appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, result controllerreconciler.Result, handleErr error) {
	current = original

	available, err := c.deploy.IsAvailable()
	if available {
		klog.V(2).Infof("key=%s resource=%s deployment is ready", original.Name, c.deploy.Name())

		condition.NewBuilderWithStatus(&current.Status).WithInstallReady()
		current.Status.Version = context.OperandVersion()
		current.Status.Image = context.OperandImage()
		return
	}

	klog.V(2).Infof("key=%s resource=%s deployment is not ready", original.Name, c.deploy.Name())

	if err == nil {
		err = fmt.Errorf("name=%s waiting for deployment to complete", c.deploy.Name())
	}

	handleErr = condition.NewInstallReadinessError(appsv1.DeploymentNotReady, err)
	return
}
