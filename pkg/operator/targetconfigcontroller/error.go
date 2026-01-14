package targetconfigcontroller

import (
	operatorv1 "github.com/openshift/api/operator/v1"
	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
)

// ConditionType represents the type of condition to set
type ConditionType string

const (
	// ConditionTypeInstallReadinessFailure maps to InstallReadinessFailure condition (for install readiness errors)
	ConditionTypeInstallReadinessFailure ConditionType = "InstallReadinessFailure"
	// ConditionTypeAvailable maps to Available condition (for availability errors)
	ConditionTypeAvailable ConditionType = "Available"
)

// HandlerError wraps an error with condition type, reason, and status for status conditions
type HandlerError struct {
	ConditionType ConditionType
	Reason        string
	Status        operatorv1.ConditionStatus
	Err           error
}

func (e *HandlerError) Error() string {
	return e.Err.Error()
}

func (e *HandlerError) Unwrap() error {
	return e.Err
}

// NewInstallReadinessError creates an error that sets InstallReadinessFailure=True condition
// This replaces the old NewInstallReadinessError from internal/condition
func NewInstallReadinessError(reason string, err error) error {
	if err == nil {
		return nil
	}
	return &HandlerError{
		ConditionType: ConditionTypeInstallReadinessFailure,
		Reason:        reason,
		Status:        operatorv1.ConditionTrue,
		Err:           err,
	}
}

// NewAvailableError creates an error that sets Available=False condition
// This replaces the old NewAvailableError from internal/condition
func NewAvailableError(reason string, err error) error {
	if err == nil {
		return nil
	}
	return &HandlerError{
		ConditionType: ConditionTypeAvailable,
		Reason:        reason,
		Status:        operatorv1.ConditionFalse,
		Err:           err,
	}
}

// GetConditionType extracts the condition type from an error
func GetConditionType(err error) string {
	if err == nil {
		return string(ConditionTypeInstallReadinessFailure)
	}

	if handlerErr, ok := err.(*HandlerError); ok {
		return string(handlerErr.ConditionType)
	}

	return string(ConditionTypeInstallReadinessFailure)
}

// GetReason extracts the reason from an error if it's a HandlerError, otherwise returns a default
func GetReason(err error) string {
	if err == nil {
		return ""
	}

	if handlerErr, ok := err.(*HandlerError); ok {
		return handlerErr.Reason
	}

	return string(appsv1.InternalError)
}

// GetStatus extracts the status from an error if it's a HandlerError, otherwise returns True for Degraded
func GetStatus(err error) operatorv1.ConditionStatus {
	if err == nil {
		return operatorv1.ConditionFalse
	}

	if handlerErr, ok := err.(*HandlerError); ok {
		return handlerErr.Status
	}

	return operatorv1.ConditionTrue
}
