package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	rodoclient "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
)

const (
	rodoNamespace = "openshift-run-once-duration-override-operator"
	rodoDSLabel   = "runoncedurationoverride=true"
)

var clusterVersionGVR = schema.GroupVersionResource{
	Group:    "config.openshift.io",
	Version:  "v1",
	Resource: "clusterversions",
}

var _ = g.Describe("[OTP][Operator][Serial] RunOnceDurationOverride Operator Functionality", g.Ordered, g.Serial, func() {
	var (
		ctx           context.Context
		kubeClient    *k8sclient.Clientset
		dynamicClient dynamic.Interface
		rodoClient    *rodoclient.Clientset
		sub           *operatorsv1alpha1.Subscription
		og            *operatorsv1.OperatorGroup
		nsCreated     bool
		ogCreated     bool
		subCreated    bool
		crCreated     bool
	)

	g.BeforeAll(func() {
		g.By("Setting up test environment")
		ctx = context.TODO()
		kubeClient = GetKubeClient()
		dynamicClient = GetDynamicClient()
		rodoClient = GetRunOnceDurationOverrideClient()

		g.By("Checking packagemanifest for expected RODO version")
		catalogSources := []string{"cs-rodoo", "redhat-operators", "qe-app-registry"}
		expectedVersion := "v1.4.1"
		var err error
		sub, err = packagemanifestRODO(ctx, dynamicClient, "run-once-duration-override-operator", "openshift-marketplace", expectedVersion, catalogSources)
		if err != nil {
			clusterVersion := "unknown"
			if cv, cvErr := dynamicClient.Resource(clusterVersionGVR).Get(ctx, "version", metav1.GetOptions{}); cvErr == nil {
				if v, ok, _ := unstructured.NestedString(cv.Object, "status", "desired", "version"); ok {
					clusterVersion = v
				}
			}
			g.Skip(fmt.Sprintf("RODO package %s not found in catalog sources %v on cluster version %s, skipping: %v", expectedVersion, catalogSources, clusterVersion, err))
		}
		klog.Infof("RODO package version %s confirmed in catalog %s", sub.Spec.StartingCSV, sub.Spec.CatalogSource)

		g.By("Creating operator namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: rodoNamespace,
				Labels: map[string]string{
					"openshift.io/cluster-monitoring": "true",
				},
			},
		}
		_, err = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		nsCreated = true

		g.By("Setting up OperatorGroup")
		og = &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-run-once-duration-override-operator",
				Namespace: rodoNamespace,
			},
			Spec: operatorsv1.OperatorGroupSpec{
				TargetNamespaces: []string{rodoNamespace},
			},
		}
		sub.Namespace = rodoNamespace

		g.By("Creating OperatorGroup")
		err = createOLMOperatorGroup(ctx, dynamicClient, og)
		o.Expect(err).NotTo(o.HaveOccurred())
		ogCreated = true

		g.By("Creating Subscription")
		err = createOLMSubscription(ctx, dynamicClient, sub)
		o.Expect(err).NotTo(o.HaveOccurred())
		subCreated = true

		g.By("Waiting for RODO operator deployment")
		err = waitForRODODeploymentReady(ctx, kubeClient, rodoNamespace, "run-once-duration-override-operator")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Waiting for CSV to succeed")
		var csvName string
		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			name, err := getRODOCSVName(ctx, dynamicClient, rodoNamespace, "")
			if err != nil {
				klog.V(2).Infof("CSV not yet available: %v", err)
				return false, nil
			}
			csvName = name
			return true, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(csvName).To(o.Equal(sub.Spec.StartingCSV), "discovered CSV should match the Subscription's StartingCSV")
		err = waitForRODOCSVSucceeded(ctx, dynamicClient, rodoNamespace, csvName)
		o.Expect(err).NotTo(o.HaveOccurred())

		klog.Infof("RODO operator successfully installed via OLM, CSV: %s", csvName)

		g.By("Creating RunOnceDurationOverride CR with activeDeadlineSeconds=60")
		err = createRunOnceDurationOverride(ctx, rodoClient, 60)
		o.Expect(err).NotTo(o.HaveOccurred())
		crCreated = true

		g.By("Waiting for DaemonSet to be ready")
		err = waitForDaemonSetReady(ctx, kubeClient, rodoNamespace, "runoncedurationoverride")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Waiting for all DS pods to be running")
		err = waitForDaemonSetPodsRunning(ctx, kubeClient, rodoNamespace, rodoDSLabel)
		o.Expect(err).NotTo(o.HaveOccurred())

		klog.Infof("RODO operator fully operational with activeDeadlineSeconds=60")
	})

	g.AfterAll(func() {
		g.By("Cleaning up operator resources")

		if crCreated {
			if err := deleteRunOnceDurationOverride(ctx, rodoClient, "cluster"); err != nil {
				klog.Warningf("Failed to delete RunOnceDurationOverride CR: %v", err)
			}
		}

		if subCreated {
			if err := deleteOLMSubscription(ctx, dynamicClient, sub); err != nil {
				klog.Warningf("Failed to delete Subscription: %v", err)
			}
		}

		if ogCreated {
			if err := deleteOLMOperatorGroup(ctx, dynamicClient, og); err != nil {
				klog.Warningf("Failed to delete OperatorGroup: %v", err)
			}
		}

		if nsCreated {
			g.By("Deleting operator namespace")
			err := kubeClient.CoreV1().Namespaces().Delete(ctx, rodoNamespace, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				klog.Warningf("Failed to delete namespace %s: %v", rodoNamespace, err)
			}

			g.By("Waiting for namespace deletion")
			if err := waitForNamespaceDeletion(ctx, kubeClient, rodoNamespace); err != nil {
				klog.Warningf("Namespace deletion wait failed: %v", err)
			}
		}
	})

	g.It("[OTP][Operator][Serial] should install RODO and verify activeDeadlineSeconds override [Disruptive][Slow][Timeout:30m]", func() {
		g.By("Testing activeDeadlineSeconds override on pods with RestartPolicy Never and OnFailure")
		testActiveDeadlineSecondsOverride(ctx, kubeClient)
	})

	g.It("[OTP][Operator][Serial] should set activeDeadlineSeconds as min of pod and operator values [Disruptive][Slow][Timeout:30m]", func() {
		g.By("Testing min(pod.activeDeadlineSeconds, operator.activeDeadlineSeconds)")
		testActiveDeadlineSecondsMinValue(ctx, kubeClient, rodoClient)
	})

	g.It("[OTP][Operator][Serial] should have relatedImages defined in CSV for disconnected cluster support [Timeout:10m]", func() {
		g.By("Checking relatedImages in CSV")
		testCSVRelatedImages(ctx, dynamicClient)
	})
})

// testActiveDeadlineSecondsOverride validates OCP-60351/OCP-60352:
// Pods with RestartPolicy Never/OnFailure should get activeDeadlineSeconds=60 from the operator,
// and should fail with DeadlineExceeded after the deadline.
func testActiveDeadlineSecondsOverride(ctx context.Context, kubeClient *k8sclient.Clientset) {
	testNS := "e2e-rodo-override-test"
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNS,
			Labels: map[string]string{
				"runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled": "true",
			},
		},
	}

	klog.Infof("Creating test namespace %s with admission label", testNS)
	_, err := kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
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
func testActiveDeadlineSecondsMinValue(ctx context.Context, kubeClient *k8sclient.Clientset, rodoClient *rodoclient.Clientset) {
	testNS := "e2e-rodo-minvalue-test"
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNS,
			Labels: map[string]string{
				"runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled": "true",
			},
		},
	}

	klog.Infof("Creating test namespace %s with admission label", testNS)
	_, err := kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
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
	err = patchRunOnceDurationOverrideADS(ctx, rodoClient, "cluster", 80)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Waiting for DaemonSet to be ready after patching")
	err = waitForDaemonSetReady(ctx, kubeClient, rodoNamespace, "runoncedurationoverride")
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

// testCSVRelatedImages validates OCP-83033:
// CSV should have .spec.relatedImages defined correctly for disconnected cluster support.
func testCSVRelatedImages(ctx context.Context, dynamicClient dynamic.Interface) {
	g.By("Getting CSV name for RODO operator")
	csvName, err := getRODOCSVName(ctx, dynamicClient, rodoNamespace, "")
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Checking relatedImages in CSV " + csvName)
	images, err := getRODOCSVRelatedImages(ctx, dynamicClient, rodoNamespace, csvName)
	o.Expect(err).NotTo(o.HaveOccurred())

	hasOperator := false
	hasOperand := false
	for _, img := range images {
		klog.Infof("Related image: name=%s, image=%s", img.Name, img.Image)
		if strings.Contains(img.Name, "run-once-duration-override-operator") || strings.Contains(img.Image, "run-once-duration-override-operator") {
			hasOperator = true
		}
		if strings.Contains(img.Name, "run-once-duration-override-operand") || strings.Contains(img.Image, "run-once-duration-override-operand") {
			hasOperand = true
		}
	}

	// For older versions (e.g., 1.3.0), only the operator image may be present
	if !strings.Contains(csvName, "v1.3.0") {
		o.Expect(hasOperand).To(o.BeTrue(), "CSV should have run-once-duration-override-operand in relatedImages")
	}
	o.Expect(hasOperator).To(o.BeTrue(), "CSV should have run-once-duration-override-operator in relatedImages")

	klog.Infof("CSV %s has correct relatedImages (operator=%v, operand=%v)", csvName, hasOperator, hasOperand)
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
