package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	corev1 "k8s.io/api/core/v1"
	apiextclientv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	rodooclient "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/operatorclient"
)

const (
	rodooDSLabel = "runoncedurationoverride=true"
)

func isOperatorOLMInstallationEnabled() bool {
	return os.Getenv("NO_OLM") == "" && os.Getenv("OPERATOR_IMAGE") == "" && os.Getenv("OPERAND_IMAGE") == ""
}

// Ginkgo test specs for migrated OTP tests
var _ = g.Describe("[OTP][Operator][Serial] RunOnceDurationOverride Operator Functionality", g.Ordered, g.Serial, func() {
	var (
		ctx           context.Context
		cancelFnc     context.CancelFunc
		kubeClient    *k8sclient.Clientset
		dynamicClient dynamic.Interface
		rodooClient   *rodooclient.Clientset
		apiExtClient  *apiextclientv1.Clientset
	)

	g.BeforeAll(func() {
		g.By("Setting up test environment")
		var err error
		kubeClient = GetKubeClient()
		dynamicClient = GetDynamicClient()
		rodooClient = GetRunOnceDurationOverrideClient()
		apiExtClient = GetApiExtensionClient()
		ctx, cancelFnc = context.WithCancel(context.TODO())

		if !isOperatorOLMInstallationEnabled() {
			err = setupOperator(ctx, kubeClient, rodooClient, apiExtClient)
		} else {
			err = installOperatorWithSubscription(ctx, kubeClient, rodooClient, dynamicClient, operatorclient.OperatorNamespace)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.AfterAll(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cleanupCancel()

		if isOperatorOLMInstallationEnabled() {
			g.By("Cleaning up operator installation")

			og := &operatorsv1.OperatorGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rodoo-og",
					Namespace: operatorclient.OperatorNamespace,
				},
			}
			sub, err := packagemanifestRODOO(cleanupCtx, dynamicClient, "run-once-duration-override-operator", operatorclient.OperatorNamespace, []string{"redhat-operators"})
			if err != nil {
				klog.Warningf("Failed to get packagemanifest for cleanup: %v", err)
			}

			if err := deleteRunOnceDurationOverride(cleanupCtx, rodooClient, operatorclient.OperatorConfigName); err != nil {
				klog.Warningf("Failed to delete RunOnceDurationOverride: %v", err)
			}
			if sub != nil {
				if err := deleteSubscription(cleanupCtx, dynamicClient, sub); err != nil {
					klog.Warningf("Failed to delete Subscription: %v", err)
				}
			}
			if err := deleteOperatorGroup(cleanupCtx, dynamicClient, og); err != nil {
				klog.Warningf("Failed to delete OperatorGroup: %v", err)
			}
		} else {
			// Cluster-scoped CR is not removed by namespace deletion
			if err := deleteRunOnceDurationOverride(cleanupCtx, rodooClient, operatorclient.OperatorConfigName); err != nil {
				klog.Warningf("Failed to delete RunOnceDurationOverride: %v", err)
			}
		}

		g.By("Deleting operator namespace")
		err := kubeClient.CoreV1().Namespaces().Delete(cleanupCtx, operatorclient.OperatorNamespace, metav1.DeleteOptions{})
		if err != nil {
			klog.Warningf("Failed to delete namespace %s: %v", operatorclient.OperatorNamespace, err)
		}

		g.By("Ensuring namespace is fully deleted")
		err = wait.PollUntilContextTimeout(cleanupCtx, 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, err := kubeClient.CoreV1().Namespaces().Get(ctx, operatorclient.OperatorNamespace, metav1.GetOptions{})
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					klog.Infof("Namespace %s successfully deleted", operatorclient.OperatorNamespace)
					return true, nil
				}
				klog.Warningf("Error checking namespace: %v", err)
				return false, nil
			}
			klog.Infof("Waiting for namespace %s to be fully deleted...", operatorclient.OperatorNamespace)
			return false, nil
		})
		if err != nil {
			klog.Warningf("Timeout waiting for namespace deletion: %v", err)
		}

		if cancelFnc != nil {
			cancelFnc()
		}
	})

	g.It("[Operator][Serial] should set ActiveDeadlineSeconds on pods in labeled namespaces [Slow][Timeout:15m]", func() {
		g.By("Testing webhook sets ActiveDeadlineSeconds")
		if isOperatorOLMInstallationEnabled() {
			g.Skip("Skipping. The non-OLM webhook test requires bindata CR with ADS=800")
		}
		testNamespace := testActiveDeadlineSecondsWebhook(g.GinkgoTB(), ctx, kubeClient)
		g.DeferCleanup(func() {
			cleanupTestNamespace(g.GinkgoTB(), ctx, kubeClient, testNamespace)
		})
	})

	// OCP-60351, OCP-60352
	g.It("[OTP][Operator][Serial] should install RODOO and verify activeDeadlineSeconds override [Disruptive][Slow][Timeout:30m]", func() {
		g.By("Testing activeDeadlineSeconds override on pods with RestartPolicy Never and OnFailure")
		testActiveDeadlineSecondsOverride(g.GinkgoTB(), ctx, kubeClient, rodooClient)
	})

	// OCP-62690
	g.It("[OTP][Operator][Serial] should set activeDeadlineSeconds as min of pod and operator values [Disruptive][Slow][Timeout:30m]", func() {
		g.By("Testing min(pod.activeDeadlineSeconds, operator.activeDeadlineSeconds)")
		testActiveDeadlineSecondsMinValue(g.GinkgoTB(), ctx, kubeClient, rodooClient)
	})

	// OCP-83033
	g.It("[OTP][Operator][Serial] should validate RelatedImages defined in CSV [Slow][Timeout:15m]", func() {
		g.By("Testing RelatedImages defined in CSV")
		if !isOperatorOLMInstallationEnabled() {
			g.Skip("Skipping. The operator is not installed via OLM")
		}
		testRelatedImages(g.GinkgoTB(), ctx, kubeClient)
	})
})

// Test implementations

// testActiveDeadlineSecondsOverride validates OCP-60351/OCP-60352:
// Pods with RestartPolicy Never/OnFailure should get activeDeadlineSeconds=60 from the operator,
// and should fail with DeadlineExceeded after the deadline.
func testActiveDeadlineSecondsOverride(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset, rodooClient *rodooclient.Clientset) {
	g.By("Setting operator activeDeadlineSeconds=60")
	err := patchRunOnceDurationOverrideADS(ctx, rodooClient, operatorclient.OperatorConfigName, 60)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = waitForDaemonSetReady(ctx, kubeClient, operatorclient.OperatorNamespace, "runoncedurationoverride")
	o.Expect(err).NotTo(o.HaveOccurred())

	testNS := "e2e-rodoo-override-test"
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNS,
			Labels: map[string]string{
				"runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled": "true",
			},
		},
	}

	klog.Infof("Creating test namespace %s with admission label", testNS)
	_, err = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer func() {
		klog.Infof("Cleaning up test namespace %s", testNS)
		_ = kubeClient.CoreV1().Namespaces().Delete(ctx, testNS, metav1.DeleteOptions{})
	}()

	time.Sleep(30 * time.Second)

	type testPod struct {
		name          string
		restartPolicy corev1.RestartPolicy
	}

	pods := []testPod{
		{name: "restartpod-never", restartPolicy: corev1.RestartPolicyNever},
		{name: "restartpod-onfailure", restartPolicy: corev1.RestartPolicyOnFailure},
	}

	for _, tp := range pods {
		klog.Infof("Creating pod %s with RestartPolicy=%s", tp.name, tp.restartPolicy)
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tp.name,
				Namespace: testNS,
				Labels:    map[string]string{"app": tp.name},
			},
			Spec: corev1.PodSpec{
				RestartPolicy: tp.restartPolicy,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: ptr.To(true),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				Containers: []corev1.Container{{
					Name:    "busybox",
					Image:   "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f",
					Command: []string{"/bin/sh", "-ec", "while sleep 5; do date; done"},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				}},
			},
		}

		_, err := kubeClient.CoreV1().Pods(testNS).Create(ctx, pod, metav1.CreateOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	for _, tp := range pods {
		g.By("Verifying pod " + tp.name + " is Running")
		err := waitForPodPhase(ctx, kubeClient, testNS, tp.name, corev1.PodRunning)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verifying activeDeadlineSeconds=60 on pod " + tp.name)
		err = waitForPodActiveDeadlineSeconds(ctx, kubeClient, testNS, tp.name, 60)
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	for _, tp := range pods {
		g.By("Waiting for pod " + tp.name + " to fail with DeadlineExceeded")
		err := waitForPodPhase(ctx, kubeClient, testNS, tp.name, corev1.PodFailed)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = verifyPodDeadlineExceeded(ctx, kubeClient, testNS, tp.name)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// testActiveDeadlineSecondsMinValue validates OCP-62690:
// activeDeadlineSeconds should be set as min(pod.spec.activeDeadlineSeconds, operator.activeDeadlineSeconds)
func testActiveDeadlineSecondsMinValue(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset, rodooClient *rodooclient.Clientset) {
	g.By("Setting operator activeDeadlineSeconds=60")
	err := patchRunOnceDurationOverrideADS(ctx, rodooClient, operatorclient.OperatorConfigName, 60)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = waitForDaemonSetReady(ctx, kubeClient, operatorclient.OperatorNamespace, "runoncedurationoverride")
	o.Expect(err).NotTo(o.HaveOccurred())

	testNS := "e2e-rodoo-minvalue-test"
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNS,
			Labels: map[string]string{
				"runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled": "true",
			},
		},
	}

	klog.Infof("Creating test namespace %s with admission label", testNS)
	_, err = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer func() {
		klog.Infof("Cleaning up test namespace %s", testNS)
		_ = kubeClient.CoreV1().Namespaces().Delete(ctx, testNS, metav1.DeleteOptions{})
	}()

	time.Sleep(30 * time.Second)

	// Pod with ADS=120, operator ADS=60 -> expected min = 60
	g.By("Creating pod with activeDeadlineSeconds=120 (operator ADS=60, expected min=60)")
	podADS120 := newTestPodWithADS("pod-ads-120", testNS, corev1.RestartPolicyOnFailure, 120)
	_, err = kubeClient.CoreV1().Pods(testNS).Create(ctx, podADS120, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Verifying pod pod-ads-120 is Running")
	err = waitForPodPhase(ctx, kubeClient, testNS, "pod-ads-120", corev1.PodRunning)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Verifying activeDeadlineSeconds=60 on pod pod-ads-120 (min of 120 and 60)")
	err = waitForPodActiveDeadlineSeconds(ctx, kubeClient, testNS, "pod-ads-120", 60)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Waiting for pod pod-ads-120 to fail with DeadlineExceeded")
	err = waitForPodPhase(ctx, kubeClient, testNS, "pod-ads-120", corev1.PodFailed)
	o.Expect(err).NotTo(o.HaveOccurred())

	err = verifyPodDeadlineExceeded(ctx, kubeClient, testNS, "pod-ads-120")
	o.Expect(err).NotTo(o.HaveOccurred())

	// Patch operator to ADS=80
	g.By("Patching RunOnceDurationOverride to activeDeadlineSeconds=80")
	err = patchRunOnceDurationOverrideADS(ctx, rodooClient, operatorclient.OperatorConfigName, 80)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Waiting for DaemonSet to be ready after patching")
	err = waitForDaemonSetReady(ctx, kubeClient, operatorclient.OperatorNamespace, "runoncedurationoverride")
	o.Expect(err).NotTo(o.HaveOccurred())

	time.Sleep(30 * time.Second)

	// Pod with ADS=240, operator ADS=80 -> expected min = 80
	g.By("Creating pod with activeDeadlineSeconds=240 (operator ADS=80, expected min=80)")
	podADS240 := newTestPodWithADS("pod-ads-240", testNS, corev1.RestartPolicyNever, 240)
	_, err = kubeClient.CoreV1().Pods(testNS).Create(ctx, podADS240, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Verifying pod pod-ads-240 is Running")
	err = waitForPodPhase(ctx, kubeClient, testNS, "pod-ads-240", corev1.PodRunning)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Verifying activeDeadlineSeconds=80 on pod pod-ads-240 (min of 240 and 80)")
	err = waitForPodActiveDeadlineSeconds(ctx, kubeClient, testNS, "pod-ads-240", 80)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Waiting for pod pod-ads-240 to fail with DeadlineExceeded")
	err = waitForPodPhase(ctx, kubeClient, testNS, "pod-ads-240", corev1.PodFailed)
	o.Expect(err).NotTo(o.HaveOccurred())

	err = verifyPodDeadlineExceeded(ctx, kubeClient, testNS, "pod-ads-240")
	o.Expect(err).NotTo(o.HaveOccurred())
}

// testRelatedImages tests that CSV has relatedImages defined correctly
func testRelatedImages(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	dynamicClient := GetDynamicClient()

	g.By("Getting CSV name for RODOO operator")
	csvName, err := getCSVName(ctx, dynamicClient, operatorclient.OperatorNamespace, "")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	klog.Infof("Found CSV: %s", csvName)

	g.By("Verifying CSV has relatedImages defined")
	relatedImages, err := getCSVRelatedImages(ctx, dynamicClient, operatorclient.OperatorNamespace, csvName)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(len(relatedImages)).To(o.BeNumerically(">", 0), "CSV should have at least one relatedImage")

	var foundOperator, foundOperand bool
	for _, img := range relatedImages {
		klog.Infof("Found relatedImage: %s -> %s", img.Name, img.Image)
		if strings.Contains(img.Name, "run-once-duration-override-operator") || strings.Contains(img.Image, "run-once-duration-override-operator") {
			foundOperator = true
		}
		if strings.Contains(img.Name, "run-once-duration-override-operand") || strings.Contains(img.Image, "run-once-duration-override-operand") {
			foundOperand = true
		}
	}

	// For older versions (e.g., 1.3.0), only the operator image may be present
	if !strings.Contains(csvName, "v1.3.0") {
		o.Expect(foundOperand).To(o.BeTrue(), "CSV should contain run-once-duration-override-operand related image")
	}
	o.Expect(foundOperator).To(o.BeTrue(), "CSV should contain run-once-duration-override-operator related image")

	klog.Infof("RelatedImages validation completed successfully - found %d images", len(relatedImages))
}

// newTestPodWithADS creates a test pod spec with a specified activeDeadlineSeconds value
func newTestPodWithADS(name, namespace string, restartPolicy corev1.RestartPolicy, activeDeadlineSeconds int64) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": name},
		},
		Spec: corev1.PodSpec{
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			RestartPolicy:         restartPolicy,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{{
				Name:    "busybox",
				Image:   "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f",
				Command: []string{"/bin/sh", "-ec", "while sleep 5; do date; done"},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: ptr.To(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
				},
			}},
		},
	}
}
