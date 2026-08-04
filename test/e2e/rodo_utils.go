package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blang/semver/v4"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	operatorsv1api "github.com/openshift/api/operator/v1"
	olmlib "github.com/openshift/library-go/test/library/olm"
	rodov1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	rodoclient "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
)

// createOLMOperatorGroup creates an OperatorGroup for the operator
func createOLMOperatorGroup(ctx context.Context, dynamicClient dynamic.Interface, og *operatorsv1.OperatorGroup) error {
	klog.Infof("Creating OperatorGroup %s in namespace %s", og.Name, og.Namespace)

	og.SetGroupVersionKind(operatorsv1.SchemeGroupVersion.WithKind("OperatorGroup"))
	unstructuredOG, err := runtime.DefaultUnstructuredConverter.ToUnstructured(og)
	if err != nil {
		return fmt.Errorf("failed to convert OperatorGroup to unstructured: %w", err)
	}

	err = olmlib.CreateOperatorGroup(ctx, dynamicClient, &unstructured.Unstructured{Object: unstructuredOG})
	if err != nil {
		return fmt.Errorf("failed to create OperatorGroup %s: %w", og.Name, err)
	}

	klog.Infof("Successfully created OperatorGroup %s", og.Name)
	return nil
}

// deleteOLMOperatorGroup deletes the OperatorGroup
func deleteOLMOperatorGroup(ctx context.Context, dynamicClient dynamic.Interface, og *operatorsv1.OperatorGroup) error {
	klog.Infof("Deleting OperatorGroup %s in namespace %s", og.Name, og.Namespace)

	og.SetGroupVersionKind(operatorsv1.SchemeGroupVersion.WithKind("OperatorGroup"))
	unstructuredOG, err := runtime.DefaultUnstructuredConverter.ToUnstructured(og)
	if err != nil {
		return fmt.Errorf("failed to convert OperatorGroup to unstructured: %w", err)
	}

	err = olmlib.DeleteOperatorGroup(ctx, dynamicClient, &unstructured.Unstructured{Object: unstructuredOG})
	if err != nil {
		return fmt.Errorf("failed to delete OperatorGroup %s: %w", og.Name, err)
	}

	klog.Infof("Successfully deleted OperatorGroup %s", og.Name)
	return nil
}

// createOLMSubscription creates a Subscription for the operator
func createOLMSubscription(ctx context.Context, dynamicClient dynamic.Interface, sub *operatorsv1alpha1.Subscription) error {
	klog.Infof("Creating Subscription %s in namespace %s", sub.Name, sub.Namespace)

	sub.SetGroupVersionKind(operatorsv1alpha1.SchemeGroupVersion.WithKind("Subscription"))
	unstructuredSub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sub)
	if err != nil {
		return fmt.Errorf("failed to convert Subscription to unstructured: %w", err)
	}

	err = olmlib.CreateSubscription(ctx, dynamicClient, &unstructured.Unstructured{Object: unstructuredSub})
	if err != nil {
		return fmt.Errorf("failed to create Subscription %s: %w", sub.Name, err)
	}

	klog.Infof("Successfully created Subscription %s", sub.Name)
	return nil
}

// deleteOLMSubscription deletes the Subscription
func deleteOLMSubscription(ctx context.Context, dynamicClient dynamic.Interface, sub *operatorsv1alpha1.Subscription) error {
	klog.Infof("Deleting Subscription %s in namespace %s", sub.Name, sub.Namespace)

	sub.SetGroupVersionKind(operatorsv1alpha1.SchemeGroupVersion.WithKind("Subscription"))
	unstructuredSub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sub)
	if err != nil {
		return fmt.Errorf("failed to convert Subscription to unstructured: %w", err)
	}

	err = olmlib.DeleteSubscription(ctx, dynamicClient, &unstructured.Unstructured{Object: unstructuredSub})
	if err != nil {
		return fmt.Errorf("failed to delete Subscription %s: %w", sub.Name, err)
	}

	klog.Infof("Successfully deleted Subscription %s", sub.Name)
	return nil
}

var packageManifestGVR = schema.GroupVersionResource{
	Group:    "packages.operators.coreos.com",
	Version:  "v1",
	Resource: "packagemanifests",
}

// packagemanifestRODO queries packagemanifest values for run-once-duration-override-operator
// from specific catalog sources. It filters by catalog label and verifies the expected version
// before returning a matching subscription.
func packagemanifestRODO(ctx context.Context, dynamicClient dynamic.Interface, packageName, namespace, expectedVersion string, catalogNames []string) (*operatorsv1alpha1.Subscription, error) {
	klog.Infof("Fetching packagemanifest values for %s with expected version %s", packageName, expectedVersion)

	var lastErr error
	for _, catalogName := range catalogNames {
		klog.Infof("Checking catalog: %s", catalogName)

		err := olmlib.CatalogSourceExists(ctx, dynamicClient, catalogName, namespace)
		if err != nil {
			klog.Infof("Catalog source %s not available: %v", catalogName, err)
			lastErr = err
			continue
		}

		pmList, err := dynamicClient.Resource(packageManifestGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("catalog=%s", catalogName),
		})
		if err != nil {
			klog.Infof("Failed to list packagemanifests for catalog %s: %v", catalogName, err)
			lastErr = err
			continue
		}

		var found *unstructured.Unstructured
		for i := range pmList.Items {
			if pmList.Items[i].GetName() == packageName {
				found = &pmList.Items[i]
				break
			}
		}

		if found == nil {
			klog.Infof("Package %s not found in catalog %s", packageName, catalogName)
			lastErr = fmt.Errorf("package %s not found in catalog %s", packageName, catalogName)
			continue
		}

		defaultChannel, found2, err := unstructured.NestedString(found.Object, "status", "defaultChannel")
		if err != nil {
			lastErr = fmt.Errorf("package %s in catalog %s: failed to read defaultChannel: %w", packageName, catalogName, err)
			continue
		}
		if !found2 || defaultChannel == "" {
			lastErr = fmt.Errorf("package %s in catalog %s has no defaultChannel", packageName, catalogName)
			continue
		}

		channels, found2, err := unstructured.NestedSlice(found.Object, "status", "channels")
		if err != nil {
			lastErr = fmt.Errorf("package %s in catalog %s: failed to read channels: %w", packageName, catalogName, err)
			continue
		}
		if !found2 {
			lastErr = fmt.Errorf("package %s in catalog %s has no channels", packageName, catalogName)
			continue
		}
		var startingCSV string
		for _, ch := range channels {
			chMap, ok := ch.(map[string]interface{})
			if !ok {
				continue
			}
			name, _, err := unstructured.NestedString(chMap, "name")
			if err != nil {
				continue
			}
			if name == defaultChannel {
				startingCSV, _, err = unstructured.NestedString(chMap, "currentCSV")
				if err != nil {
					lastErr = fmt.Errorf("package %s in catalog %s: failed to read currentCSV: %w", packageName, catalogName, err)
					break
				}
				break
			}
		}

		if startingCSV == "" {
			lastErr = fmt.Errorf("package %s in catalog %s has no currentCSV for channel %s", packageName, catalogName, defaultChannel)
			continue
		}

		if !csvVersionEquals(startingCSV, expectedVersion) {
			klog.Infof("Package %s in catalog %s has version %s, expected %s, trying next catalog", packageName, catalogName, startingCSV, expectedVersion)
			lastErr = fmt.Errorf("package %s in catalog %s has version %s, expected %s", packageName, catalogName, startingCSV, expectedVersion)
			continue
		}

		klog.Infof("Found matching package manifest in catalog %s: channel=%s, startingCSV=%s", catalogName, defaultChannel, startingCSV)

		sub := &operatorsv1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      packageName,
				Namespace: namespace,
			},
			Spec: &operatorsv1alpha1.SubscriptionSpec{
				CatalogSource:          catalogName,
				CatalogSourceNamespace: namespace,
				Package:                packageName,
				Channel:                defaultChannel,
				StartingCSV:            startingCSV,
			},
		}

		return sub, nil
	}

	return nil, fmt.Errorf("package %s with version %s not found in any of the specified catalogs %v: %w", packageName, expectedVersion, catalogNames, lastErr)
}

// createRunOnceDurationOverride creates a RunOnceDurationOverride CR
func createRunOnceDurationOverride(ctx context.Context, rodoClient *rodoclient.Clientset, activeDeadlineSeconds int64) error {
	klog.Infof("Creating RunOnceDurationOverride CR with activeDeadlineSeconds=%d", activeDeadlineSeconds)

	cr := &rodov1.RunOnceDurationOverride{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: rodov1.RunOnceDurationOverrideSpec{
			OperatorSpec: operatorsv1api.OperatorSpec{
				ManagementState: operatorsv1api.Managed,
			},
			RunOnceDurationOverrideConfig: rodov1.RunOnceDurationOverrideConfig{
				Spec: rodov1.RunOnceDurationOverrideConfigSpec{
					ActiveDeadlineSeconds: activeDeadlineSeconds,
				},
			},
		},
	}

	_, err := rodoClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Create(ctx, cr, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create RunOnceDurationOverride CR: %w", err)
	}

	klog.Infof("Successfully created RunOnceDurationOverride CR")
	return nil
}

// deleteRunOnceDurationOverride deletes the RunOnceDurationOverride CR
func deleteRunOnceDurationOverride(ctx context.Context, rodoClient *rodoclient.Clientset, name string) error {
	klog.Infof("Deleting RunOnceDurationOverride CR %s", name)

	err := rodoClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete RunOnceDurationOverride CR %s: %w", name, err)
	}

	klog.Infof("Successfully deleted RunOnceDurationOverride CR %s", name)
	return nil
}

// patchRunOnceDurationOverrideADS patches the RunOnceDurationOverride CR to change activeDeadlineSeconds
func patchRunOnceDurationOverrideADS(ctx context.Context, rodoClient *rodoclient.Clientset, name string, newADS int64) error {
	klog.Infof("Patching RunOnceDurationOverride CR %s with activeDeadlineSeconds=%d", name, newADS)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cr, err := rodoClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get RunOnceDurationOverride CR: %w", err)
		}

		cr.Spec.RunOnceDurationOverrideConfig.Spec.ActiveDeadlineSeconds = newADS

		_, err = rodoClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Update(ctx, cr, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to patch RunOnceDurationOverride CR: %w", err)
	}

	klog.Infof("Successfully patched RunOnceDurationOverride CR activeDeadlineSeconds to %d", newADS)
	return nil
}

// waitForRODODeploymentReady waits for a deployment to have the expected number of ready replicas
func waitForRODODeploymentReady(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, name string) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		deployment, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Failed to get deployment %s/%s: %v", namespace, name, err)
			return false, nil
		}

		if deployment.Spec.Replicas == nil {
			return false, nil
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

// getRODOCSVName gets the CSV name for the operator
func getRODOCSVName(ctx context.Context, dynamicClient dynamic.Interface, namespace, labelSelector string) (string, error) {
	csvName, err := olmlib.GetTheLatestCSVName(ctx, dynamicClient, namespace, labelSelector)
	if err != nil {
		return "", fmt.Errorf("failed to get CSV name: %w", err)
	}

	klog.Infof("Found CSV: %s", csvName)
	return csvName, nil
}

// getRODOCSVRelatedImages gets the relatedImages from a CSV
func getRODOCSVRelatedImages(ctx context.Context, dynamicClient dynamic.Interface, namespace, csvName string) ([]olmlib.RelatedImage, error) {
	csvUnstructured, err := dynamicClient.Resource(olmlib.CSVGVR()).Namespace(namespace).Get(ctx, csvName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CSV %s: %w", csvName, err)
	}

	images, err := olmlib.GetCSVRelatedImages(csvUnstructured)
	if err != nil {
		return nil, fmt.Errorf("failed to get CSV related images: %w", err)
	}

	klog.Infof("Found %d related images in CSV %s", len(images), csvName)
	return images, nil
}

// waitForRODOCSVSucceeded waits for CSV to reach Succeeded phase
func waitForRODOCSVSucceeded(ctx context.Context, dynamicClient dynamic.Interface, namespace, csvName string) error {
	klog.Infof("Waiting for CSV %s/%s to succeed", namespace, csvName)

	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
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

// waitForNamespaceDeletion waits for a namespace to be fully deleted
func waitForNamespaceDeletion(ctx context.Context, kubeClient *k8sclient.Clientset, namespace string) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err := kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			klog.Infof("Namespace %s fully deleted", namespace)
			return true, nil
		}
		if err != nil {
			return false, err
		}
		klog.Infof("Waiting for namespace %s to be deleted...", namespace)
		return false, nil
	})
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

// csvVersionEquals extracts the semver from a CSV name (e.g. "operator.v1.4.1")
// and compares it against an expected version string (e.g. "v1.4.1" or "1.4.1").
func csvVersionEquals(csvName, expectedVersion string) bool {
	csvVer := csvName
	if idx := strings.LastIndex(csvName, ".v"); idx >= 0 {
		csvVer = csvName[idx+2:]
	} else {
		csvVer = strings.TrimPrefix(csvVer, "v")
	}

	expected := strings.TrimPrefix(expectedVersion, "v")

	csvSemver, err := semver.Parse(csvVer)
	if err != nil {
		return false
	}
	expectedSemver, err := semver.Parse(expected)
	if err != nil {
		return false
	}

	return csvSemver.EQ(expectedSemver)
}
