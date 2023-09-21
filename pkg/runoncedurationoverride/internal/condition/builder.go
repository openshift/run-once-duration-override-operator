package condition

import (
	"time"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock"
)

func NewBuilderWithStatus(status *appsv1.RunOnceDurationOverrideStatus) *Builder {
	return &Builder{
		clock:  clock.RealClock{},
		status: status,
	}
}

func Find(status *appsv1.RunOnceDurationOverrideStatus, conditionType appsv1.RunOnceDurationOverrideConditionType) *appsv1.RunOnceDurationOverrideCondition {
	for i := range status.Conditions {
		c := &status.Conditions[i]
		if c.Type == conditionType {
			return c
		}
	}

	return nil
}

// Equal returns true if the two given conditions are equal.
// We deem two conditions equal if Type, Status, Reason and Message are a match
// (despite LastTransitionTime being different).
func Equal(this, that *appsv1.RunOnceDurationOverrideCondition) bool {
	if this.Type == that.Type &&
		this.Status == that.Status &&
		this.Reason == that.Reason &&
		this.Message == that.Message {
		return true
	}

	return false
}

func DeepCopyWithDefaultLastTransitionTime(status *appsv1.RunOnceDurationOverrideStatus) (copy *appsv1.RunOnceDurationOverrideStatus) {
	copy = status.DeepCopy()
	for i := range copy.Conditions {
		copy.Conditions[i].LastTransitionTime = metav1.NewTime(time.Time{})
	}

	return
}

type Builder struct {
	clock  clock.Clock
	status *appsv1.RunOnceDurationOverrideStatus
}

func (b *Builder) WithError(err error) (builder *Builder) {
	builder = b
	if err == nil {
		return
	}

	b.init()

	desired := FromError(err, metav1.NewTime(b.clock.Now()))
	b.WithCondition(desired)

	return b
}

func (b *Builder) WithInstallReady() (builder *Builder) {
	b.init()

	desired := &appsv1.RunOnceDurationOverrideCondition{
		Type:               appsv1.InstallReadinessFailure,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(b.clock.Now()),
	}
	b.WithCondition(desired)

	return b
}

func (b *Builder) WithAvailable(status corev1.ConditionStatus, message string) (builder *Builder) {
	b.init()

	desired := &appsv1.RunOnceDurationOverrideCondition{
		Type:               appsv1.Available,
		Status:             status,
		Message:            message,
		LastTransitionTime: metav1.NewTime(b.clock.Now()),
	}
	b.WithCondition(desired)

	return b
}

func (b *Builder) WithCondition(desired *appsv1.RunOnceDurationOverrideCondition) {
	if desired == nil {
		return
	}

	current := Find(b.status, desired.Type)
	if current == nil {
		b.status.Conditions = append(b.status.Conditions, *desired)
		return
	}

	if Equal(desired, current) {
		return
	}

	current.Reason = desired.Reason
	current.Message = desired.Message
	current.Status = desired.Status
	current.LastTransitionTime = desired.LastTransitionTime
}

func (b *Builder) init() {
	if b.status == nil {
		b.status = &appsv1.RunOnceDurationOverrideStatus{Conditions: []appsv1.RunOnceDurationOverrideCondition{}}
	}
}
