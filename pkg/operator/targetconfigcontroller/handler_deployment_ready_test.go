package targetconfigcontroller

import (
	"testing"

	operatorsv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

type fakeDeployInterface struct {
	available bool
	err       error
	name      string
}

func (f *fakeDeployInterface) Name() string {
	return f.name
}

func (f *fakeDeployInterface) IsAvailable() (bool, error) {
	return f.available, f.err
}

func (f *fakeDeployInterface) Get() (object runtime.Object, accessor metav1.Object, err error) {
	return nil, nil, nil
}

func (f *fakeDeployInterface) Ensure(parent, child deploy.Applier, generations []operatorsv1.GenerationStatus) (object runtime.Object, accessor metav1.Object, err error) {
	return nil, nil, nil
}

func TestDeploymentReadyHandler(t *testing.T) {
	tests := []struct {
		name                  string
		deployAvailable       bool
		deployError           error
		expectError           bool
		expectCondition       bool
		expectConditionType   string
		expectConditionStatus operatorsv1.ConditionStatus
		expectVersionSet      bool
		expectImageSet        bool
	}{
		{
			name:                  "Deployment Available - Sets InstallReadinessFailure=False",
			deployAvailable:       true,
			deployError:           nil,
			expectError:           false,
			expectCondition:       true,
			expectConditionType:   appsv1.InstallReadinessFailure,
			expectConditionStatus: operatorsv1.ConditionFalse,
			expectVersionSet:      true,
			expectImageSet:        true,
		},
		{
			name:            "Deployment Not Available - Returns Error",
			deployAvailable: false,
			deployError:     nil,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployInterface := &fakeDeployInterface{
				available: tt.deployAvailable,
				err:       tt.deployError,
				name:      "test-deployment",
			}

			handler := NewDeploymentReadyHandler(deployInterface)

			runtimeContext := operatorruntime.NewOperandContext(
				"test-operator",
				"test-namespace",
				"cluster",
				"test-image:latest",
				"v1.0.0",
			)
			reconcileContext := NewReconcileRequestContext(runtimeContext)

			original := &appsv1.RunOnceDurationOverride{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
			}

			current, _, err := handler.Handle(reconcileContext, original)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.expectCondition {
				condition := v1helpers.FindOperatorCondition(current.Status.Conditions, tt.expectConditionType)
				if condition == nil {
					t.Errorf("expected condition %s to be set, but it was not found", tt.expectConditionType)
				} else {
					if condition.Status != tt.expectConditionStatus {
						t.Errorf("expected condition %s status to be %s, got %s", tt.expectConditionType, tt.expectConditionStatus, condition.Status)
					}
					if condition.LastTransitionTime.IsZero() {
						t.Errorf("expected condition %s to have LastTransitionTime set", tt.expectConditionType)
					}
				}
			}

			if tt.expectVersionSet {
				if current.Status.Version != runtimeContext.OperandVersion() {
					t.Errorf("expected version to be %s, got %s", runtimeContext.OperandVersion(), current.Status.Version)
				}
			}

			if tt.expectImageSet {
				if current.Status.Image != runtimeContext.OperandImage() {
					t.Errorf("expected image to be %s, got %s", runtimeContext.OperandImage(), current.Status.Image)
				}
			}
		})
	}
}
