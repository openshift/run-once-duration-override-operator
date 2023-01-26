package reconciler

import (
	"context"
	"reflect"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/apps/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatusUpdater updates the status of a RunOnceDurationOverride resource.
type StatusUpdater struct {
	client versioned.Interface
}

// Update updates the status of a RunOnceDurationOverride resource.
// If the status inside of the desired object is equal to that of the observed then
// the function does not make an update call.
func (u *StatusUpdater) Update(observed, desired *appsv1.RunOnceDurationOverride) error {
	if reflect.DeepEqual(&observed.Status, &desired.Status) {
		return nil
	}

	_, err := u.client.AppsV1().RunOnceDurationOverrides().UpdateStatus(context.TODO(), desired, metav1.UpdateOptions{})
	return err
}
