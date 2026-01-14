package operatorclient

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/apiserver/jsonpatch"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	applyconfiguration "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
)

var _ v1helpers.OperatorClient = &OperatorClientWrapper{}

// OperatorClientWrapper wraps RunOnceDurationOverrideClient to implement v1helpers.OperatorClient
// by translating between custom RunOnceDurationOverride types and generic operatorv1 types.
type OperatorClientWrapper struct {
	client *RunOnceDurationOverrideClient
}

// NewOperatorClientWrapper creates a wrapper around RunOnceDurationOverrideClient that implements
// v1helpers.OperatorClient interface.
func NewOperatorClientWrapper(client *RunOnceDurationOverrideClient) *OperatorClientWrapper {
	return &OperatorClientWrapper{
		client: client,
	}
}

func (w *OperatorClientWrapper) Informer() cache.SharedIndexInformer {
	return w.client.Informer()
}

func (w *OperatorClientWrapper) GetObjectMeta() (*metav1.ObjectMeta, error) {
	return w.client.GetObjectMeta()
}

func (w *OperatorClientWrapper) GetOperatorState() (*operatorv1.OperatorSpec, *operatorv1.OperatorStatus, string, error) {
	spec, status, resourceVersion, err := w.client.GetOperatorState()
	if err != nil {
		return nil, nil, "", err
	}
	return &spec.OperatorSpec, &status.OperatorStatus, resourceVersion, nil
}

func (w *OperatorClientWrapper) GetOperatorStateWithQuorum(ctx context.Context) (*operatorv1.OperatorSpec, *operatorv1.OperatorStatus, string, error) {
	spec, status, resourceVersion, err := w.client.GetOperatorStateWithQuorum(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	return &spec.OperatorSpec, &status.OperatorStatus, resourceVersion, nil
}

func (w *OperatorClientWrapper) UpdateOperatorSpec(ctx context.Context, oldResourceVersion string, in *operatorv1.OperatorSpec) (*operatorv1.OperatorSpec, string, error) {
	// Get the current full spec to preserve custom fields
	currentSpec, _, _, err := w.client.GetOperatorState()
	if err != nil {
		return nil, "", err
	}

	// Create updated spec with custom fields preserved
	updatedSpec := currentSpec.DeepCopy()
	updatedSpec.OperatorSpec = *in

	// Update using the full spec
	result, resourceVersion, err := w.client.UpdateOperatorSpec(ctx, oldResourceVersion, updatedSpec)
	if err != nil {
		return nil, "", err
	}

	return &result.OperatorSpec, resourceVersion, nil
}

func (w *OperatorClientWrapper) UpdateOperatorStatus(ctx context.Context, oldResourceVersion string, in *operatorv1.OperatorStatus) (*operatorv1.OperatorStatus, error) {
	// Get the current full status to preserve custom fields
	_, currentStatus, _, err := w.client.GetOperatorState()
	if err != nil {
		return nil, err
	}

	// Create updated status with custom fields preserved
	updatedStatus := currentStatus.DeepCopy()
	updatedStatus.OperatorStatus = *in

	// Update using the full status
	result, err := w.client.UpdateOperatorStatus(ctx, oldResourceVersion, updatedStatus)
	if err != nil {
		return nil, err
	}

	return &result.OperatorStatus, nil
}

func (w *OperatorClientWrapper) ApplyOperatorSpec(ctx context.Context, fieldManager string, applyConfiguration *applyconfiguration.OperatorSpecApplyConfiguration) error {
	return w.client.ApplyOperatorSpec(ctx, fieldManager, applyConfiguration)
}

func (w *OperatorClientWrapper) ApplyOperatorStatus(ctx context.Context, fieldManager string, applyConfiguration *applyconfiguration.OperatorStatusApplyConfiguration) error {
	return w.client.ApplyOperatorStatus(ctx, fieldManager, applyConfiguration)
}

func (w *OperatorClientWrapper) PatchOperatorStatus(ctx context.Context, jsonPatch *jsonpatch.PatchSet) error {
	return w.client.PatchOperatorStatus(ctx, jsonPatch)
}
