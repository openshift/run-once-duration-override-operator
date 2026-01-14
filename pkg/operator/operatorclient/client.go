package operatorclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/apiserver/jsonpatch"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	applyconfiguration "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	runoncedurationoverrideapplyconfiguration "github.com/openshift/run-once-duration-override-operator/pkg/generated/applyconfiguration/runoncedurationoverride/v1"
	operatorconfigclientv1 "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/typed/runoncedurationoverride/v1"
	runoncedurationoverrideinformerv1 "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions/runoncedurationoverride/v1"
)

const (
	OperatorName            = "runoncedurationoverride"
	OperatorNamespace       = "openshift-run-once-duration-override-operator"
	OperatorConfigName      = "cluster"
	OperatorOwnerAnnotation = "runoncedurationoverride.operator.openshift.io/owner"
)

type RunOnceDurationOverrideOperatorClient interface {
	Informer() cache.SharedIndexInformer
	GetObjectMeta() (meta *metav1.ObjectMeta, err error)
	GetOperatorState() (spec *runoncedurationoverridev1.RunOnceDurationOverrideSpec, status *runoncedurationoverridev1.RunOnceDurationOverrideStatus, resourceVersion string, err error)
	GetOperatorStateWithQuorum(ctx context.Context) (*runoncedurationoverridev1.RunOnceDurationOverrideSpec, *runoncedurationoverridev1.RunOnceDurationOverrideStatus, string, error)
	UpdateOperatorSpec(ctx context.Context, oldResourceVersion string, in *operatorv1.OperatorSpec) (out *operatorv1.OperatorSpec, newResourceVersion string, err error)
	UpdateOperatorStatus(ctx context.Context, oldResourceVersion string, in *runoncedurationoverridev1.RunOnceDurationOverrideStatus) (out *runoncedurationoverridev1.RunOnceDurationOverrideStatus, err error)
	ApplyOperatorSpec(ctx context.Context, fieldManager string, applyConfiguration *applyconfiguration.OperatorSpecApplyConfiguration) (err error)
	ApplyOperatorStatus(ctx context.Context, fieldManager string, applyConfiguration *applyconfiguration.OperatorStatusApplyConfiguration) (err error)
	PatchOperatorStatus(ctx context.Context, jsonPatch *jsonpatch.PatchSet) (err error)
}

var _ RunOnceDurationOverrideOperatorClient = &RunOnceDurationOverrideClient{}

type RunOnceDurationOverrideClient struct {
	Ctx                             context.Context
	RunOnceDurationOverrideInformer runoncedurationoverrideinformerv1.RunOnceDurationOverrideInformer
	OperatorClient                  operatorconfigclientv1.RunOnceDurationOverrideV1Interface
}

func (c RunOnceDurationOverrideClient) Informer() cache.SharedIndexInformer {
	return c.RunOnceDurationOverrideInformer.Informer()
}

func (c RunOnceDurationOverrideClient) GetOperatorState() (spec *runoncedurationoverridev1.RunOnceDurationOverrideSpec, status *runoncedurationoverridev1.RunOnceDurationOverrideStatus, resourceVersion string, err error) {
	instance, err := c.RunOnceDurationOverrideInformer.Lister().Get(OperatorConfigName)
	if err != nil {
		return nil, nil, "", err
	}
	return &instance.Spec, &instance.Status, instance.ResourceVersion, nil
}

func (c *RunOnceDurationOverrideClient) GetObjectMeta() (meta *metav1.ObjectMeta, err error) {
	instance, err := c.RunOnceDurationOverrideInformer.Lister().Get(OperatorConfigName)
	if err != nil {
		return nil, err
	}
	return &instance.ObjectMeta, nil
}

func (c RunOnceDurationOverrideClient) GetOperatorStateWithQuorum(ctx context.Context) (*runoncedurationoverridev1.RunOnceDurationOverrideSpec, *runoncedurationoverridev1.RunOnceDurationOverrideStatus, string, error) {
	instance, err := c.OperatorClient.RunOnceDurationOverrides().Get(ctx, OperatorConfigName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, "", err
	}
	return &instance.Spec, &instance.Status, instance.ResourceVersion, nil
}

func (c *RunOnceDurationOverrideClient) UpdateOperatorSpec(ctx context.Context, resourceVersion string, spec *operatorv1.OperatorSpec) (out *operatorv1.OperatorSpec, newResourceVersion string, err error) {
	original, err := c.RunOnceDurationOverrideInformer.Lister().Get(OperatorConfigName)
	if err != nil {
		return nil, "", err
	}
	copy := original.DeepCopy()
	copy.ResourceVersion = resourceVersion
	copy.Spec.OperatorSpec = *spec

	ret, err := c.OperatorClient.RunOnceDurationOverrides().Update(ctx, copy, metav1.UpdateOptions{})
	if err != nil {
		return nil, "", err
	}

	return &ret.Spec.OperatorSpec, ret.ResourceVersion, nil
}

func (c *RunOnceDurationOverrideClient) UpdateOperatorStatus(ctx context.Context, resourceVersion string, status *runoncedurationoverridev1.RunOnceDurationOverrideStatus) (out *runoncedurationoverridev1.RunOnceDurationOverrideStatus, err error) {
	original, err := c.RunOnceDurationOverrideInformer.Lister().Get(OperatorConfigName)
	if err != nil {
		return nil, err
	}
	copy := original.DeepCopy()
	copy.ResourceVersion = resourceVersion
	copy.Status = *status

	ret, err := c.OperatorClient.RunOnceDurationOverrides().UpdateStatus(ctx, copy, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	return &ret.Status, nil
}

func (c *RunOnceDurationOverrideClient) ApplyOperatorSpec(ctx context.Context, fieldManager string, desiredConfiguration *applyconfiguration.OperatorSpecApplyConfiguration) error {
	if desiredConfiguration == nil {
		return fmt.Errorf("applyConfiguration must have a value")
	}

	desiredSpec := &runoncedurationoverrideapplyconfiguration.RunOnceDurationOverrideSpecApplyConfiguration{
		OperatorSpecApplyConfiguration: *desiredConfiguration,
	}
	desired := runoncedurationoverrideapplyconfiguration.RunOnceDurationOverride(OperatorConfigName)
	desired.WithSpec(desiredSpec)

	instance, err := c.RunOnceDurationOverrideInformer.Lister().Get(OperatorConfigName)
	switch {
	case apierrors.IsNotFound(err):
		// do nothing and proceed with the apply
	case err != nil:
		return fmt.Errorf("unable to get operator configuration: %w", err)
	default:
		original, err := runoncedurationoverrideapplyconfiguration.ExtractRunOnceDurationOverride(instance, fieldManager)
		if err != nil {
			return fmt.Errorf("unable to extract operator configuration from spec: %w", err)
		}
		if equality.Semantic.DeepEqual(original, desired) {
			return nil
		}
	}

	_, err = c.OperatorClient.RunOnceDurationOverrides().Apply(ctx, desired, metav1.ApplyOptions{
		Force:        true,
		FieldManager: fieldManager,
	})
	if err != nil {
		return fmt.Errorf("unable to Apply for operator using fieldManager %q: %w", fieldManager, err)
	}

	return nil
}

func (c *RunOnceDurationOverrideClient) ApplyOperatorStatus(ctx context.Context, fieldManager string, desiredConfiguration *applyconfiguration.OperatorStatusApplyConfiguration) error {
	if desiredConfiguration == nil {
		return fmt.Errorf("applyConfiguration must have a value")
	}

	desired := runoncedurationoverrideapplyconfiguration.RunOnceDurationOverride(OperatorConfigName)
	instance, err := c.RunOnceDurationOverrideInformer.Lister().Get(OperatorConfigName)
	switch {
	case apierrors.IsNotFound(err):
		// do nothing and proceed with the apply
		v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desiredConfiguration.Conditions, nil)
		desiredStatus := c.convertApplyConfigToFullStatus(&runoncedurationoverridev1.RunOnceDurationOverrideStatus{}, desiredConfiguration)
		desired.WithStatus(desiredStatus)
	case err != nil:
		return fmt.Errorf("unable to get operator configuration: %w", err)
	default:
		previous, err := runoncedurationoverrideapplyconfiguration.ExtractRunOnceDurationOverrideStatus(instance, fieldManager)
		if err != nil {
			return fmt.Errorf("unable to extract operator configuration: %w", err)
		}

		operatorStatus := &applyconfiguration.OperatorStatusApplyConfiguration{}
		if previous.Status != nil {
			jsonBytes, err := json.Marshal(previous.Status)
			if err != nil {
				return fmt.Errorf("unable to serialize operator configuration: %w", err)
			}
			if err := json.Unmarshal(jsonBytes, operatorStatus); err != nil {
				return fmt.Errorf("unable to deserialize operator configuration: %w", err)
			}
		}

		switch {
		case desiredConfiguration.Conditions != nil && operatorStatus != nil:
			v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desiredConfiguration.Conditions, operatorStatus.Conditions)
		case desiredConfiguration.Conditions != nil && operatorStatus == nil:
			v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desiredConfiguration.Conditions, nil)
		}

		original := runoncedurationoverrideapplyconfiguration.RunOnceDurationOverride(OperatorConfigName)
		if previous.Status != nil {
			originalStatus := c.convertApplyConfigToFullStatus(&instance.Status, &previous.Status.OperatorStatusApplyConfiguration)
			original.WithStatus(originalStatus)
		}

		desiredStatus := c.convertApplyConfigToFullStatus(&instance.Status, desiredConfiguration)
		desired.WithStatus(desiredStatus)

		if equality.Semantic.DeepEqual(original, desired) {
			return nil
		}
	}

	_, err = c.OperatorClient.RunOnceDurationOverrides().ApplyStatus(ctx, desired, metav1.ApplyOptions{
		Force:        true,
		FieldManager: fieldManager,
	})
	if err != nil {
		return fmt.Errorf("unable to ApplyStatus for operator using fieldManager %q: %w", fieldManager, err)
	}

	return nil
}

func (c *RunOnceDurationOverrideClient) PatchOperatorStatus(ctx context.Context, jsonPatch *jsonpatch.PatchSet) (err error) {
	jsonPatchBytes, err := jsonPatch.Marshal()
	if err != nil {
		return err
	}
	_, err = c.OperatorClient.RunOnceDurationOverrides().Patch(ctx, OperatorConfigName, types.JSONPatchType, jsonPatchBytes, metav1.PatchOptions{}, "/status")
	return err
}

// UpdateRunOnceDurationOverrideStatusFunc is a function that modifies the RunOnceDurationOverrideStatus.
type UpdateRunOnceDurationOverrideStatusFunc func(status *runoncedurationoverridev1.RunOnceDurationOverrideStatus) error

// UpdateStatus applies the update functions to the full RunOnceDurationOverrideStatus and persists it.
// This is similar to v1helpers.UpdateStatus but works with the complete custom status structure
// including Hash, Resources, Image, and CertsRotateAt fields that handlers modify.
func UpdateStatus(ctx context.Context, client *RunOnceDurationOverrideClient, updateFuncs ...UpdateRunOnceDurationOverrideStatusFunc) (*runoncedurationoverridev1.RunOnceDurationOverrideStatus, bool, error) {
	updated := false
	var updatedOperatorStatus *runoncedurationoverridev1.RunOnceDurationOverrideStatus
	numberOfAttempts := 0
	previousResourceVersion := ""
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		defer func() {
			numberOfAttempts++
		}()
		var oldStatus *runoncedurationoverridev1.RunOnceDurationOverrideStatus
		var resourceVersion string
		var err error

		// prefer lister if we haven't already failed.
		_, oldStatus, resourceVersion, err = client.GetOperatorState()
		if err != nil {
			return err
		}
		if resourceVersion == previousResourceVersion {
			listerResourceVersion := resourceVersion
			// this indicates that we've had a conflict and the lister has not caught up, so do a live GET
			_, oldStatus, resourceVersion, err = client.GetOperatorStateWithQuorum(ctx)
			if err != nil {
				return err
			}
			klog.V(2).Infof("lister was stale at resourceVersion=%v, live get showed resourceVersion=%v", listerResourceVersion, resourceVersion)
		}
		previousResourceVersion = resourceVersion

		newStatus := oldStatus.DeepCopy()
		for _, update := range updateFuncs {
			if err := update(newStatus); err != nil {
				return err
			}
		}

		if equality.Semantic.DeepEqual(oldStatus, newStatus) {
			// We return the newStatus which is a deep copy of oldStatus but with all update funcs applied.
			updatedOperatorStatus = newStatus
			return nil
		}
		if klog.V(4).Enabled() {
			klog.Infof("Operator status changed: %v", operatorStatusJSONPatchNoError(oldStatus, newStatus))
		}

		updatedOperatorStatus, err = client.UpdateOperatorStatus(ctx, resourceVersion, newStatus)
		updated = err == nil
		return err
	})

	return updatedOperatorStatus, updated, err
}

func operatorStatusJSONPatchNoError(original, modified *runoncedurationoverridev1.RunOnceDurationOverrideStatus) string {
	if original == nil {
		return "original object is nil"
	}
	if modified == nil {
		return "modified object is nil"
	}

	return cmp.Diff(original, modified)
}

func (c *RunOnceDurationOverrideClient) convertApplyConfigToFullStatus(
	currentStatus *runoncedurationoverridev1.RunOnceDurationOverrideStatus,
	desiredOperatorStatus *applyconfiguration.OperatorStatusApplyConfiguration,
) *runoncedurationoverrideapplyconfiguration.RunOnceDurationOverrideStatusApplyConfiguration {
	applyConfig := runoncedurationoverrideapplyconfiguration.RunOnceDurationOverrideStatus()
	applyConfig.OperatorStatusApplyConfiguration = *desiredOperatorStatus

	if currentStatus.Hash.Configuration != "" || currentStatus.Hash.ServingCert != "" {
		hashApplyConfig := runoncedurationoverrideapplyconfiguration.RunOnceDurationOverrideResourceHash()
		if currentStatus.Hash.Configuration != "" {
			hashApplyConfig.WithConfiguration(currentStatus.Hash.Configuration)
		}
		if currentStatus.Hash.ServingCert != "" {
			hashApplyConfig.WithServingCert(currentStatus.Hash.ServingCert)
		}
		applyConfig.WithHash(hashApplyConfig)
	}

	if currentStatus.Resources.ConfigurationRef != nil || currentStatus.Resources.DeploymentRef != nil ||
		currentStatus.Resources.ServiceCertSecretRef != nil || currentStatus.Resources.ServiceCAConfigMapRef != nil {
		resourcesApplyConfig := runoncedurationoverrideapplyconfiguration.RunOnceDurationOverrideResources()
		if currentStatus.Resources.ConfigurationRef != nil {
			resourcesApplyConfig.WithConfigurationRef(*currentStatus.Resources.ConfigurationRef)
		}
		if currentStatus.Resources.DeploymentRef != nil {
			resourcesApplyConfig.WithDeploymentRef(*currentStatus.Resources.DeploymentRef)
		}
		if currentStatus.Resources.ServiceCertSecretRef != nil {
			resourcesApplyConfig.WithServiceCertSecretRef(*currentStatus.Resources.ServiceCertSecretRef)
		}
		if currentStatus.Resources.ServiceCAConfigMapRef != nil {
			resourcesApplyConfig.WithServiceCAConfigMapRef(*currentStatus.Resources.ServiceCAConfigMapRef)
		}
		applyConfig.WithResources(resourcesApplyConfig)
	}

	if currentStatus.Image != "" {
		applyConfig.WithImage(currentStatus.Image)
	}

	if !currentStatus.CertsRotateAt.IsZero() {
		applyConfig.WithCertsRotateAt(currentStatus.CertsRotateAt)
	}

	return applyConfig
}
