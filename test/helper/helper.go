package helper

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/apps/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
)

var (
	WaitInterval = 1 * time.Second
	WaitTimeout  = 5 * time.Minute
)

type Disposer func()

func (d Disposer) Dispose() {
	d()
}

type ConditionFunc func(override *appsv1.RunOnceDurationOverride) bool

type Client struct {
	Operator   versioned.Interface
	Kubernetes kubernetes.Interface
}

func NewClient(t *testing.T, config *rest.Config) *Client {
	operator, err := versioned.NewForConfig(config)
	require.NoErrorf(t, err, "failed to construct client for apps.openshift.io - %v", err)

	kubeclient, err := kubernetes.NewForConfig(config)
	require.NoErrorf(t, err, "failed to construct client for kubernetes - %v", err)

	return &Client{
		Operator:   operator,
		Kubernetes: kubeclient,
	}
}

func EnsureAdmissionWebhook(t *testing.T, client versioned.Interface, name string, cluster appsv1.RunOnceDurationOverride) (current *appsv1.RunOnceDurationOverride, changed bool) {
	changed = true

	var err error
	current, err = client.AppsV1().RunOnceDurationOverrides().Create(context.TODO(), &cluster, metav1.CreateOptions{})
	if err == nil {
		return
	}

	if !k8serrors.IsAlreadyExists(err) {
		require.FailNowf(t, "unexpected error - %s", err.Error())
	}

	current, err = client.AppsV1().RunOnceDurationOverrides().Get(context.TODO(), "cluster", metav1.GetOptions{})
	require.NoErrorf(t, err, "failed to get - %v", err)
	require.NotNil(t, current)

	// if the desired spec matches current spec then no change.
	if reflect.DeepEqual(current.Spec, cluster.Spec) {
		changed = false
		return
	}

	current.Spec = *cluster.Spec.DeepCopy()
	current, err = client.AppsV1().RunOnceDurationOverrides().Update(context.TODO(), current, metav1.UpdateOptions{})
	require.NoErrorf(t, err, "failed to update - %v", err)
	require.NotNil(t, current)
	return
}

func RemoveAdmissionWebhook(t *testing.T, client versioned.Interface, name string) {
	_, err := client.AppsV1().RunOnceDurationOverrides().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			require.FailNowf(t, "unexpected error - %s", err.Error())
		}

		return
	}

	err = client.AppsV1().RunOnceDurationOverrides().Delete(context.TODO(), name, metav1.DeleteOptions{})
	require.NoError(t, err)
}

func NewNamespace(t *testing.T, client kubernetes.Interface, name string, optIn bool) (ns *corev1.Namespace, disposer Disposer) {
	request := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", name),
		},
	}

	if optIn {
		request.ObjectMeta.Labels = map[string]string{
			"runoncedurationoverrides.admission.apps.openshift.io/enabled": "true",
		}
	}

	object, err := client.CoreV1().Namespaces().Create(context.TODO(), request, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NotNil(t, object)

	ns = object
	disposer = func() {
		err := client.CoreV1().Namespaces().Delete(context.TODO(), object.Name, metav1.DeleteOptions{})
		require.NoError(t, err)
	}
	return
}

func NewPod(t *testing.T, client kubernetes.Interface, namespace string, spec corev1.PodSpec) (pod *corev1.Pod, disposer Disposer) {
	request := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "croe2e-",
		},
		Spec: spec,
	}

	object, err := client.CoreV1().Pods(namespace).Create(context.TODO(), request, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NotNil(t, object)

	pod = object
	disposer = func() {
		err := client.CoreV1().Pods(object.Namespace).Delete(context.TODO(), object.Name, metav1.DeleteOptions{})
		require.NoError(t, err)
	}
	return
}

func Wait(t *testing.T, client versioned.Interface, name string, f ConditionFunc) (override *appsv1.RunOnceDurationOverride) {
	err := wait.Poll(WaitInterval, WaitTimeout, func() (done bool, err error) {
		override, err = client.AppsV1().RunOnceDurationOverrides().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return
		}

		if override == nil || !f(override) {
			return
		}

		done = true
		return
	})

	require.NoErrorf(t, err, "wait.Poll returned error - %v", err)
	require.NotNil(t, override)
	return
}

func GetAvailableConditionFunc(original *appsv1.RunOnceDurationOverride, expectNewResourceVersion bool) ConditionFunc {
	return func(current *appsv1.RunOnceDurationOverride) bool {
		switch {
		// we expect current to have a different resource version than original
		case expectNewResourceVersion:
			return original.ResourceVersion != current.ResourceVersion && IsAvailable(current)
		default:
			return IsAvailable(current)
		}
	}
}

func GetCondition(override *appsv1.RunOnceDurationOverride, condType appsv1.RunOnceDurationOverrideConditionType) *appsv1.RunOnceDurationOverrideCondition {
	for i := range override.Status.Conditions {
		condition := &override.Status.Conditions[i]
		if condition.Type == condType {
			return condition
		}
	}

	return nil
}

func IsAvailable(override *appsv1.RunOnceDurationOverride) bool {
	available := GetCondition(override, appsv1.Available)
	readinessFailure := GetCondition(override, appsv1.InstallReadinessFailure)
	if available == nil || readinessFailure == nil {
		return false
	}

	if available.Status != corev1.ConditionTrue || readinessFailure.Status != corev1.ConditionFalse {
		return false
	}

	return true
}
