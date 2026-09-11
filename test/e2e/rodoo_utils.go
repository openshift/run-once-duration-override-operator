package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	o "github.com/onsi/gomega"

	operatorsv1api "github.com/openshift/api/operator/v1"
	olmlib "github.com/openshift/library-go/test/library/olm"
	rodoov1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	rodooclient "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
)

func installOperatorWithSubscription(
	ctx context.Context,
	kubeClient *k8sclient.Clientset,
	rodooClient *rodooclient.Clientset,
	dynamicClient dynamic.Interface,
	rodooNamespace string,
) error {
	klog.Infof("Creating the operator namespace")
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: rodooNamespace,
			Labels: map[string]string{
				"openshift.io/cluster-monitoring": "true",
			},
		},
	}
	_, err := kubeClient.CoreV1().Namespaces().Create(ctx, namespaceObj, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}

	klog.Infof("Setting up OperatorGroup")
	og := &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rodoo-og",
			Namespace: namespaceObj.Name,
		},
		Spec: operatorsv1.OperatorGroupSpec{
			TargetNamespaces: []string{namespaceObj.Name},
		},
	}

	klog.Infof("Fetching subscription details from packagemanifest")
	sub, err := packagemanifestRODOO(ctx, dynamicClient, "run-once-duration-override-operator", namespaceObj.Name, []string{"redhat-operators"})
	if err != nil {
		return err
	}

	klog.Infof("Creating OperatorGroup")
	err = createOperatorGroup(ctx, dynamicClient, og)
	if err != nil {
		return err
	}

	klog.Infof("Creating Subscription")
	err = createSubscription(ctx, dynamicClient, sub)
	if err != nil {
		return err
	}

	klog.Infof("Waiting for RODOO operator deployment")
	err = waitForDeploymentReady(ctx, kubeClient, namespaceObj.Name, "run-once-duration-override-operator")
	o.Expect(err).NotTo(o.HaveOccurred())

	klog.Infof("Waiting for CSV to succeed")
	var csvName string
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		name, err := getCSVName(ctx, dynamicClient, namespaceObj.Name, "")
		if err != nil {
			klog.V(2).Infof("CSV not yet available: %v", err)
			return false, nil
		}
		csvName = name
		return true, nil
	})
	if err != nil {
		return err
	}
	err = waitForCSVSucceeded(ctx, dynamicClient, namespaceObj.Name, csvName)
	if err != nil {
		return err
	}

	klog.Infof("RODOO operator successfully installed via OLM, CSV: %s", csvName)

	klog.Infof("Creating RunOnceDurationOverride CR")
	err = createRunOnceDurationOverride(ctx, rodooClient, 60)
	if err != nil {
		return err
	}

	klog.Infof("Waiting for DaemonSet to be ready")
	err = waitForDaemonSetReady(ctx, kubeClient, namespaceObj.Name, "runoncedurationoverride")
	if err != nil {
		return err
	}

	klog.Infof("Waiting for all DS pods to be running")
	err = waitForDaemonSetPodsRunning(ctx, kubeClient, namespaceObj.Name, rodooDSLabel)
	if err != nil {
		return err
	}

	klog.Infof("RODOO operator fully operational with activeDeadlineSeconds=60")
	return nil
}

// createOperatorGroup creates an OperatorGroup for the operator
func createOperatorGroup(ctx context.Context, dynamicClient dynamic.Interface, og *operatorsv1.OperatorGroup) error {
	klog.Infof("Creating OperatorGroup %s in namespace %s", og.Name, og.Namespace)

	unstructuredOG, err := runtime.DefaultUnstructuredConverter.ToUnstructured(og)
	if err != nil {
		return fmt.Errorf("failed to convert OperatorGroup to unstructured: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredOG}
	u.SetAPIVersion("operators.coreos.com/v1")
	u.SetKind("OperatorGroup")

	err = olmlib.CreateOperatorGroup(ctx, dynamicClient, u)
	if err != nil {
		return fmt.Errorf("failed to create OperatorGroup %s: %w", og.Name, err)
	}

	klog.Infof("Successfully created OperatorGroup %s", og.Name)
	return nil
}

// deleteOperatorGroup deletes the OperatorGroup
func deleteOperatorGroup(ctx context.Context, dynamicClient dynamic.Interface, og *operatorsv1.OperatorGroup) error {
	klog.Infof("Deleting OperatorGroup %s in namespace %s", og.Name, og.Namespace)

	unstructuredOG, err := runtime.DefaultUnstructuredConverter.ToUnstructured(og)
	if err != nil {
		return fmt.Errorf("failed to convert OperatorGroup to unstructured: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredOG}
	u.SetAPIVersion("operators.coreos.com/v1")
	u.SetKind("OperatorGroup")

	err = olmlib.DeleteOperatorGroup(ctx, dynamicClient, u)
	if err != nil {
		return fmt.Errorf("failed to delete OperatorGroup %s: %w", og.Name, err)
	}

	klog.Infof("Successfully deleted OperatorGroup %s", og.Name)
	return nil
}

// createSubscription creates a Subscription for the operator
func createSubscription(ctx context.Context, dynamicClient dynamic.Interface, sub *operatorsv1alpha1.Subscription) error {
	klog.Infof("Creating Subscription %s in namespace %s", sub.Name, sub.Namespace)

	unstructuredSub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sub)
	if err != nil {
		return fmt.Errorf("failed to convert Subscription to unstructured: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredSub}
	u.SetAPIVersion("operators.coreos.com/v1alpha1")
	u.SetKind("Subscription")

	err = olmlib.CreateSubscription(ctx, dynamicClient, u)
	if err != nil {
		return fmt.Errorf("failed to create Subscription %s: %w", sub.Name, err)
	}

	klog.Infof("Successfully created Subscription %s", sub.Name)
	return nil
}

// deleteSubscription deletes the Subscription
func deleteSubscription(ctx context.Context, dynamicClient dynamic.Interface, sub *operatorsv1alpha1.Subscription) error {
	klog.Infof("Deleting Subscription %s in namespace %s", sub.Name, sub.Namespace)

	unstructuredSub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sub)
	if err != nil {
		return fmt.Errorf("failed to convert Subscription to unstructured: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredSub}
	u.SetAPIVersion("operators.coreos.com/v1alpha1")
	u.SetKind("Subscription")

	err = olmlib.DeleteSubscription(ctx, dynamicClient, u)
	if err != nil {
		return fmt.Errorf("failed to delete Subscription %s: %w", sub.Name, err)
	}

	klog.Infof("Successfully deleted Subscription %s", sub.Name)
	return nil
}

// packagemanifestRODOO fetches packagemanifest values dynamically for run-once-duration-override-operator
func packagemanifestRODOO(ctx context.Context, dynamicClient dynamic.Interface, packageName, namespace string, catalogNames []string) (*operatorsv1alpha1.Subscription, error) {
	klog.Infof("Fetching packagemanifest values for %s", packageName)

	unstructuredSub, err := olmlib.BuildSubscriptionFromPackageManifest(ctx, dynamicClient, packageName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get packagemanifest %s: %w", packageName, err)
	}

	sub := &operatorsv1alpha1.Subscription{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredSub.Object, sub)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Subscription from unstructured: %w", err)
	}

	// The unstructured Subscription uses "package" for the package name field,
	// but the typed Subscription struct maps Package to JSON tag "name".
	// FromUnstructured looks for "name" in spec and finds nothing, leaving
	// Spec.Package empty. Set it explicitly from the known packageName.
	if sub.Spec.Package == "" {
		sub.Spec.Package = packageName
	}

	klog.Infof("Found package manifest: channel=%s, source=%s, startingCSV=%s",
		sub.Spec.Channel, sub.Spec.CatalogSource, sub.Spec.StartingCSV)

	return sub, nil
}

// createRunOnceDurationOverride creates a RunOnceDurationOverride CR
func createRunOnceDurationOverride(ctx context.Context, rodooClient *rodooclient.Clientset, activeDeadlineSeconds int64) error {
	klog.Infof("Creating RunOnceDurationOverride CR with activeDeadlineSeconds=%d", activeDeadlineSeconds)

	cr := &rodoov1.RunOnceDurationOverride{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: rodoov1.RunOnceDurationOverrideSpec{
			OperatorSpec: operatorsv1api.OperatorSpec{
				ManagementState: operatorsv1api.Managed,
			},
			RunOnceDurationOverrideConfig: rodoov1.RunOnceDurationOverrideConfig{
				Spec: rodoov1.RunOnceDurationOverrideConfigSpec{
					ActiveDeadlineSeconds: activeDeadlineSeconds,
				},
			},
		},
	}

	_, err := rodooClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Create(ctx, cr, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create RunOnceDurationOverride CR: %w", err)
	}

	klog.Infof("Successfully created RunOnceDurationOverride CR")
	return nil
}

// deleteRunOnceDurationOverride deletes the RunOnceDurationOverride CR
func deleteRunOnceDurationOverride(ctx context.Context, rodooClient *rodooclient.Clientset, name string) error {
	klog.Infof("Deleting RunOnceDurationOverride CR %s", name)

	err := rodooClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete RunOnceDurationOverride CR %s: %w", name, err)
	}

	klog.Infof("Successfully deleted RunOnceDurationOverride CR %s", name)
	return nil
}

// patchRunOnceDurationOverrideADS patches the RunOnceDurationOverride CR to change activeDeadlineSeconds
func patchRunOnceDurationOverrideADS(ctx context.Context, rodooClient *rodooclient.Clientset, name string, newADS int64) error {
	klog.Infof("Patching RunOnceDurationOverride CR %s with activeDeadlineSeconds=%d", name, newADS)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cr, err := rodooClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get RunOnceDurationOverride CR: %w", err)
		}

		cr.Spec.RunOnceDurationOverrideConfig.Spec.ActiveDeadlineSeconds = newADS

		_, err = rodooClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Update(ctx, cr, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to patch RunOnceDurationOverride CR: %w", err)
	}

	klog.Infof("Successfully patched RunOnceDurationOverride CR activeDeadlineSeconds to %d", newADS)
	return nil
}

// waitForDeploymentReady waits for a deployment to have the expected number of ready replicas
func waitForDeploymentReady(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, name string) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		deployment, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Failed to get deployment %s/%s: %v", namespace, name, err)
			return false, nil
		}

		if deployment.Spec.Replicas == nil {
			return false, fmt.Errorf("deployment %s/%s has nil Spec.Replicas", namespace, name)
		}

		if deployment.Status.ReadyReplicas >= *deployment.Spec.Replicas {
			klog.Infof("Deployment %s/%s is ready with %d replicas", namespace, name, deployment.Status.ReadyReplicas)
			return true, nil
		}

		klog.Infof("Waiting for deployment %s/%s: %d/%d replicas ready",
			namespace, name, deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
		return false, nil
	})
}

// waitForDaemonSetReady waits for a daemonset to have numberReady == desiredNumberScheduled
func waitForDaemonSetReady(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, name string) error {
	return wait.PollUntilContextTimeout(ctx, 10*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		ds, err := kubeClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Failed to get daemonset %s/%s: %v", namespace, name, err)
			return false, nil
		}

		if ds.Status.DesiredNumberScheduled > 0 &&
			ds.Status.NumberReady == ds.Status.DesiredNumberScheduled &&
			ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled &&
			ds.Status.ObservedGeneration >= ds.Generation {
			klog.Infof("DaemonSet %s/%s is ready: %d/%d (generation %d)", namespace, name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled, ds.Status.ObservedGeneration)
			return true, nil
		}

		klog.Infof("Waiting for daemonset %s/%s: ready=%d/%d, updated=%d/%d, generation=%d/%d",
			namespace, name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled,
			ds.Status.UpdatedNumberScheduled, ds.Status.DesiredNumberScheduled,
			ds.Status.ObservedGeneration, ds.Generation)
		return false, nil
	})
}

// waitForDaemonSetPodsRunning waits for all pods matching the label selector to be in Running phase
func waitForDaemonSetPodsRunning(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, labelSelector string) error {
	return wait.PollUntilContextTimeout(ctx, 10*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			klog.Warningf("Failed to list pods: %v", err)
			return false, nil
		}

		if len(pods.Items) == 0 {
			klog.Infof("No pods found with label %s, waiting...", labelSelector)
			return false, nil
		}

		allRunning := true
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				klog.Infof("Pod %s is in phase %s, waiting...", pod.Name, pod.Status.Phase)
				allRunning = false
			}
		}

		if allRunning {
			klog.Infof("All %d DS pods are running", len(pods.Items))
		}

		return allRunning, nil
	})
}

// waitForPodPhase waits for a pod to reach the expected phase
func waitForPodPhase(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, podName string, expectedPhase corev1.PodPhase) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		pod, err := kubeClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Failed to get pod %s/%s: %v", namespace, podName, err)
			return false, nil
		}

		if pod.Status.Phase == expectedPhase {
			klog.Infof("Pod %s is in expected phase %s", podName, expectedPhase)
			return true, nil
		}

		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return false, fmt.Errorf("pod %s reached terminal phase %s, expected %s", podName, pod.Status.Phase, expectedPhase)
		}

		klog.Infof("Pod %s is in phase %s, expected %s", podName, pod.Status.Phase, expectedPhase)
		return false, nil
	})
}

// waitForPodActiveDeadlineSeconds waits for a pod to have the expected activeDeadlineSeconds value
func waitForPodActiveDeadlineSeconds(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, podName string, expectedADS int64) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		pod, err := kubeClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Failed to get pod %s/%s: %v", namespace, podName, err)
			return false, nil
		}

		if pod.Spec.ActiveDeadlineSeconds == nil {
			klog.Infof("Pod %s activeDeadlineSeconds is not yet set", podName)
			return false, nil
		}

		if *pod.Spec.ActiveDeadlineSeconds == expectedADS {
			klog.Infof("Pod %s activeDeadlineSeconds is correctly set to %d", podName, expectedADS)
			return true, nil
		}

		klog.Infof("Pod %s activeDeadlineSeconds is %d, expected %d", podName, *pod.Spec.ActiveDeadlineSeconds, expectedADS)
		return false, nil
	})
}

// getCSVName gets the CSV name for the operator using label selector
func getCSVName(ctx context.Context, dynamicClient dynamic.Interface, namespace, labelSelector string) (string, error) {
	csvName, err := olmlib.GetTheLatestCSVName(ctx, dynamicClient, namespace, labelSelector)
	if err != nil {
		return "", fmt.Errorf("failed to list CSVs: %w", err)
	}

	klog.Infof("Found CSV: %s", csvName)
	return csvName, nil
}

// RelatedImage represents an image referenced in a CSV
type RelatedImage struct {
	Name  string
	Image string
}

// getCSVRelatedImages gets the relatedImages from a CSV
func getCSVRelatedImages(ctx context.Context, dynamicClient dynamic.Interface, namespace, csvName string) ([]RelatedImage, error) {
	csvUnstructured, err := dynamicClient.Resource(olmlib.CSVGVR()).Namespace(namespace).Get(ctx, csvName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CSV %s: %w", csvName, err)
	}

	libImages, err := olmlib.GetCSVRelatedImages(csvUnstructured)
	if err != nil {
		return nil, fmt.Errorf("failed to get CSV related images: %w", err)
	}

	images := make([]RelatedImage, len(libImages))
	for i, img := range libImages {
		images[i] = RelatedImage{
			Name:  img.Name,
			Image: img.Image,
		}
	}

	klog.Infof("Found %d related images in CSV %s", len(images), csvName)
	return images, nil
}

// waitForCSVSucceeded waits for CSV to reach Succeeded phase
func waitForCSVSucceeded(ctx context.Context, dynamicClient dynamic.Interface, namespace, csvName string) error {
	klog.Infof("Waiting for CSV %s/%s to succeed", namespace, csvName)

	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		csv, err := dynamicClient.Resource(olmlib.CSVGVR()).Namespace(namespace).Get(ctx, csvName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				klog.Infof("CSV %s not found yet, waiting...", csvName)
				return false, nil
			}
			return false, err
		}

		phase, found, err := unstructured.NestedString(csv.Object, "status", "phase")
		if err != nil || !found {
			klog.Infof("CSV %s phase not yet available, waiting...", csvName)
			return false, nil
		}

		klog.Infof("CSV %s current phase: %s", csvName, phase)

		if phase == "Succeeded" {
			return true, nil
		}

		if phase == "Failed" {
			return false, fmt.Errorf("CSV %s failed", csvName)
		}

		return false, nil
	})

	if err != nil {
		return fmt.Errorf("failed waiting for CSV to succeed: %w", err)
	}

	klog.Infof("CSV %s succeeded", csvName)
	return nil
}

// verifyPodDeadlineExceeded checks that a pod has status reason DeadlineExceeded
func verifyPodDeadlineExceeded(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, podName string) error {
	pod, err := kubeClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	if pod.Status.Reason == "DeadlineExceeded" {
		klog.Infof("Pod %s has DeadlineExceeded reason as expected", podName)
		return nil
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && strings.Contains(cs.State.Terminated.Reason, "DeadlineExceeded") {
			klog.Infof("Pod %s container %s has DeadlineExceeded reason as expected", podName, cs.Name)
			return nil
		}
	}

	return fmt.Errorf("pod %s does not have DeadlineExceeded reason, status reason: %s, phase: %s", podName, pod.Status.Reason, pod.Status.Phase)
}
