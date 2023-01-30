package condition

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/apps/v1"
)

func NewInstallReadinessError(reason string, err error) error {
	return &installReadinessError{
		reconciliationError{
			Reason: reason,
			Err:    err,
		},
	}
}

func NewAvailableError(reason string, err error) error {
	return &availableError{
		reconciliationError{
			Reason: reason,
			Err:    err,
		},
	}
}

func FromError(err error, time metav1.Time) *appsv1.RunOnceDurationOverrideCondition {
	switch e := err.(type) {
	case *installReadinessError:
		return &appsv1.RunOnceDurationOverrideCondition{
			Type:               appsv1.InstallReadinessFailure,
			Reason:             e.Reason,
			Message:            e.Error(),
			Status:             corev1.ConditionTrue,
			LastTransitionTime: time,
		}
	case *availableError:
		return &appsv1.RunOnceDurationOverrideCondition{
			Type:               appsv1.Available,
			Reason:             e.Reason,
			Message:            e.Error(),
			Status:             corev1.ConditionFalse,
			LastTransitionTime: time,
		}
	}

	return nil
}

type installReadinessError struct {
	reconciliationError
}

type availableError struct {
	reconciliationError
}

type reconciliationError struct {
	Reason string
	Err    error
}

func (e *reconciliationError) Error() string {
	return e.Err.Error()
}
