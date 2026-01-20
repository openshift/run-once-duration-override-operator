package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	clocktesting "k8s.io/utils/clock/testing"
	utilpointer "k8s.io/utils/pointer"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	runoncedurationoverridescheme "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/scheme"
	"github.com/openshift/run-once-duration-override-operator/test/e2e/bindata"
)

// Ginkgo test specs - calls the shared test functions
var _ = g.Describe("[sig-scheduling][Operator][Serial] RunOnceDurationOverride Operator", g.Ordered, func() {
	var (
		ctx           context.Context
		cancelFnc     context.CancelFunc
		kubeClient    *k8sclient.Clientset
		testNamespace string
	)

	g.BeforeAll(func() {
		g.By("Setting up the operator")
		var err error
		ctx, cancelFnc, kubeClient, err = setupOperator(g.GinkgoTB())
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.AfterAll(func() {
		if cancelFnc != nil {
			cancelFnc()
		}
	})

	g.Context("when webhook is active", func() {
		g.It("should set ActiveDeadlineSeconds on pods in labeled namespaces [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			g.By("Creating test namespace and verifying webhook sets ActiveDeadlineSeconds")
			testNamespace = testActiveDeadlineSecondsWebhook(g.GinkgoTB(), ctx, kubeClient)
			g.DeferCleanup(func() {
				g.By("Cleaning up test namespace")
				cleanupTestNamespace(g.GinkgoTB(), ctx, kubeClient, testNamespace)
			})
		})
	})
})

// setupOperator sets up the operator and waits for it to be ready.
// This function works with both standard Go testing and Ginkgo.
func setupOperator(t testing.TB) (context.Context, context.CancelFunc, *k8sclient.Clientset, error) {
	ctx, cancelFnc := context.WithCancel(context.Background())

	// Verify required environment variables
	if os.Getenv("KUBECONFIG") == "" {
		return ctx, cancelFnc, nil, fmt.Errorf("KUBECONFIG environment variable must be set")
	}
	if os.Getenv("RELEASE_IMAGE_LATEST") == "" {
		return ctx, cancelFnc, nil, fmt.Errorf("RELEASE_IMAGE_LATEST environment variable must be set")
	}
	if os.Getenv("NAMESPACE") == "" {
		return ctx, cancelFnc, nil, fmt.Errorf("NAMESPACE environment variable must be set")
	}

	// Initialize clients
	kubeClient := GetKubeClient()
	apiExtClient := GetApiExtensionClient()
	runOnceDurationOverrideClient := GetRunOnceDurationOverrideClient()

	eventRecorder := events.NewKubeRecorder(
		kubeClient.CoreV1().Events("default"),
		"test-e2e",
		&corev1.ObjectReference{},
		clocktesting.NewFakePassiveClock(time.Now()),
	)

	// Define and apply required assets
	assets := []struct {
		path           string
		readerAndApply func(objBytes []byte) error
	}{
		{
			path: "assets/00_operator-namespace.yaml",
			readerAndApply: func(objBytes []byte) error {
				_, _, err := resourceapply.ApplyNamespace(ctx, kubeClient.CoreV1(), eventRecorder, resourceread.ReadNamespaceV1OrDie(objBytes))
				return err
			},
		},
		{
			path: "assets/01_sa.yaml",
			readerAndApply: func(objBytes []byte) error {
				_, _, err := resourceapply.ApplyServiceAccount(ctx, kubeClient.CoreV1(), eventRecorder, resourceread.ReadServiceAccountV1OrDie(objBytes))
				return err
			},
		},
		{
			path: "assets/03_clusterrole.yaml",
			readerAndApply: func(objBytes []byte) error {
				_, _, err := resourceapply.ApplyClusterRole(ctx, kubeClient.RbacV1(), eventRecorder, resourceread.ReadClusterRoleV1OrDie(objBytes))
				return err
			},
		},
		{
			path: "assets/04_clusterrolebinding.yaml",
			readerAndApply: func(objBytes []byte) error {
				_, _, err := resourceapply.ApplyClusterRoleBinding(ctx, kubeClient.RbacV1(), eventRecorder, resourceread.ReadClusterRoleBindingV1OrDie(objBytes))
				return err
			},
		},
		{
			path: "assets/05_role.yaml",
			readerAndApply: func(objBytes []byte) error {
				_, _, err := resourceapply.ApplyRole(ctx, kubeClient.RbacV1(), eventRecorder, resourceread.ReadRoleV1OrDie(objBytes))
				return err
			},
		},
		{
			path: "assets/06_rolebinding.yaml",
			readerAndApply: func(objBytes []byte) error {
				_, _, err := resourceapply.ApplyRoleBinding(ctx, kubeClient.RbacV1(), eventRecorder, resourceread.ReadRoleBindingV1OrDie(objBytes))
				return err
			},
		},
		{
			path: "assets/07_deployment.yaml",
			readerAndApply: func(objBytes []byte) error {
				required := resourceread.ReadDeploymentV1OrDie(objBytes)
				// Override the operator image with the one built in CI
				registry := strings.Split(os.Getenv("RELEASE_IMAGE_LATEST"), "/")[0]
				required.Spec.Template.Spec.Containers[0].Image = registry + "/" + os.Getenv("NAMESPACE") + "/pipeline:run-once-duration-override-operator"

				// Set RELATED_IMAGE_OPERAND_IMAGE env
				for i, env := range required.Spec.Template.Spec.Containers[0].Env {
					if env.Name == "RELATED_IMAGE_OPERAND_IMAGE" {
						required.Spec.Template.Spec.Containers[0].Env[i].Value = "registry.ci.openshift.org/ocp/4.20:run-once-duration-override-webhook"
						break
					}
				}

				_, _, err := resourceapply.ApplyDeployment(ctx, kubeClient.AppsV1(), eventRecorder, required, 1000)
				return err
			},
		},
		{
			path: "assets/08_crd.yaml",
			readerAndApply: func(objBytes []byte) error {
				_, _, err := resourceapply.ApplyCustomResourceDefinitionV1(ctx, apiExtClient.ApiextensionsV1(), eventRecorder, resourceread.ReadCustomResourceDefinitionV1OrDie(objBytes))
				return err
			},
		},
		{
			path: "assets/09_cr.yaml",
			readerAndApply: func(objBytes []byte) error {
				requiredObj, err := runtime.Decode(runoncedurationoverridescheme.Codecs.UniversalDecoder(runoncedurationoverridev1.SchemeGroupVersion), objBytes)
				if err != nil {
					return err
				}
				requiredSS := requiredObj.(*runoncedurationoverridev1.RunOnceDurationOverride)
				_, err = runOnceDurationOverrideClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Create(ctx, requiredSS, metav1.CreateOptions{})
				return err
			},
		},
	}

	// Apply all assets
	klog.Infof("Creating operator resources (namespace, CRD, RBAC, deployment)")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var lastErr error
		allSucceeded := true
		for _, asset := range assets {
			klog.Infof("Creating %v", asset.path)
			if err := asset.readerAndApply(bindata.MustAsset(asset.path)); err != nil {
				klog.Errorf("Unable to create %v: %v", asset.path, err)
				lastErr = err
				allSucceeded = false
			}
		}
		if allSucceeded {
			break
		}
		if time.Now().Before(deadline) {
			time.Sleep(1 * time.Second)
		} else if lastErr != nil {
			return ctx, cancelFnc, nil, fmt.Errorf("failed to create assets: %w", lastErr)
		}
	}

	// Wait for operator pod to be running
	klog.Infof("Waiting for operator pod to be running")
	deadline = time.Now().Add(1 * time.Minute)
	operatorRunning := false
	for time.Now().Before(deadline) {
		podItems, err := kubeClient.CoreV1().Pods("openshift-run-once-duration-override-operator").List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, pod := range podItems.Items {
				if !strings.HasPrefix(pod.Name, "run-once-duration-override-") {
					continue
				}
				if pod.Status.Phase == corev1.PodRunning && pod.GetDeletionTimestamp() == nil {
					klog.Infof("Operator pod %v is running", pod.Name)
					operatorRunning = true
					break
				}
			}
		}
		if operatorRunning {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if !operatorRunning {
		return ctx, cancelFnc, nil, fmt.Errorf("operator pod not running after timeout")
	}

	// Count master nodes for webhook verification
	klog.Infof("Counting master nodes")
	nodeItems, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return ctx, cancelFnc, nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	webhooksExpected := 0
	for _, node := range nodeItems.Items {
		if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
			webhooksExpected++
		}
	}

	// Wait for webhook daemonset pods to be running
	klog.Infof("Waiting for webhook daemonset pods to be running")
	deadline = time.Now().Add(1 * time.Minute)
	webhooksReady := false
	for time.Now().Before(deadline) {
		podItems, err := kubeClient.CoreV1().Pods("openshift-run-once-duration-override-operator").List(ctx, metav1.ListOptions{})
		if err == nil {
			webhooksRunning := 0
			for _, pod := range podItems.Items {
				if !strings.HasPrefix(pod.Name, "runoncedurationoverride-") {
					continue
				}
				if pod.Status.Phase == corev1.PodRunning && pod.GetDeletionTimestamp() == nil {
					webhooksRunning++
				}
			}
			klog.Infof("Webhook pods running: %d/%d", webhooksRunning, webhooksExpected)
			if webhooksRunning >= webhooksExpected {
				webhooksReady = true
				break
			}
		}
		time.Sleep(5 * time.Second)
	}
	if !webhooksReady {
		return ctx, cancelFnc, nil, fmt.Errorf("webhook pods not ready after timeout")
	}

	// Wait for mutating webhook configuration to be created
	klog.Infof("Waiting for mutating webhook configuration")
	deadline = time.Now().Add(2 * time.Minute)
	webhookConfigured := false
	for time.Now().Before(deadline) {
		mutatingWebhooks, err := kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, mutatingWebhook := range mutatingWebhooks.Items {
				if strings.HasPrefix(mutatingWebhook.Name, "runoncedurationoverrides") {
					webhookConfigured = true
					break
				}
			}
		}
		if webhookConfigured {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if !webhookConfigured {
		return ctx, cancelFnc, nil, fmt.Errorf("mutating webhook configuration not found after timeout")
	}

	klog.Infof("All operator components are running and ready")
	return ctx, cancelFnc, kubeClient, nil
}

// testActiveDeadlineSecondsWebhook tests that the webhook sets ActiveDeadlineSeconds on pods.
// This function works with both standard Go testing and Ginkgo.
// Returns the test namespace name for cleanup.
func testActiveDeadlineSecondsWebhook(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) string {
	// Create a test namespace with the required label
	testNamespace := "e2e-test-runoncedurationoverriding"
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
			Labels: map[string]string{
				"runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled": "true",
			},
		},
	}

	klog.Infof("Creating test namespace with webhook label")
	_, err := kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test namespace: %v", err)
	}

	// Create a test pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      "test-mutating-admission-pod",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: utilpointer.BoolPtr(true),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{{
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: utilpointer.BoolPtr(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
				},
				Name:            "pause",
				ImagePullPolicy: "Always",
				Image:           "kubernetes/pause",
				Ports:           []corev1.ContainerPort{{ContainerPort: 80}},
			}},
		},
	}

	klog.Infof("Creating test pod")
	_, err = kubeClient.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test pod: %v", err)
	}

	// Verify the pod gets ActiveDeadlineSeconds set to 800
	klog.Infof("Verifying ActiveDeadlineSeconds is set to 800")
	deadline := time.Now().Add(2 * time.Minute)
	verified := false
	for time.Now().Before(deadline) {
		retrievedPod, err := kubeClient.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Unable to get pod: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if retrievedPod.Spec.NodeName == "" {
			klog.Infof("Pod not yet assigned to a node")
			time.Sleep(1 * time.Second)
			continue
		}
		klog.Infof("Pod successfully assigned to node: %v", retrievedPod.Spec.NodeName)

		if retrievedPod.Spec.ActiveDeadlineSeconds == nil {
			klog.Infof("pod.Spec.ActiveDeadlineSeconds is not set")
			time.Sleep(1 * time.Second)
			continue
		}

		if *retrievedPod.Spec.ActiveDeadlineSeconds != 800 {
			klog.Infof("pod.Spec.ActiveDeadlineSeconds is set to %d, expected 800", *retrievedPod.Spec.ActiveDeadlineSeconds)
			time.Sleep(1 * time.Second)
			continue
		}

		klog.Infof("pod.Spec.ActiveDeadlineSeconds = %v (expected: 800)", *retrievedPod.Spec.ActiveDeadlineSeconds)
		verified = true
		break
	}

	if !verified {
		t.Fatalf("pod should have ActiveDeadlineSeconds set to 800")
	}

	return testNamespace
}

// cleanupTestNamespace deletes the test namespace.
func cleanupTestNamespace(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset, testNamespace string) {
	if testNamespace == "" {
		return
	}
	klog.Infof("Cleaning up test namespace: %s", testNamespace)
	err := kubeClient.CoreV1().Namespaces().Delete(ctx, testNamespace, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete test namespace: %v", err)
	}
}
