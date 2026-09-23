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
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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

const (
	operatorNamespace = "openshift-run-once-duration-override-operator"
	operatorSAName    = "run-once-duration-override-operator"
	testSAName        = "rodo-e2e-test"
	netpolName        = "run-once-duration-override-operand"
	curlImage         = "curlimages/curl"
	httpServerImage   = "registry.access.redhat.com/ubi9/ubi:latest"
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

	g.Context("NetworkPolicy Traffic Blocking", func() {
		g.It("should allow traffic to unlabeled pods (positive control) [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			var targetName, curlName string
			g.DeferCleanup(func() {
				fd := metav1.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(0)}
				if targetName != "" {
					_ = kubeClient.CoreV1().Pods(operatorNamespace).Delete(ctx, targetName, fd)
				}
				if curlName != "" {
					_ = kubeClient.CoreV1().Pods(operatorNamespace).Delete(ctx, curlName, fd)
				}
			})
			pod := newHTTPServerPod("netpol-control-", operatorNamespace, nil, "")
			createdTarget, err := kubeClient.CoreV1().Pods(operatorNamespace).Create(ctx, pod, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			targetName = createdTarget.Name
			podIP := waitForPodReady(ctx, kubeClient, operatorNamespace, targetName)
			cmd := []string{"curl", "-sk", "--connect-timeout", "5", "-o", "/dev/null", "-w", "%{http_code}", fmt.Sprintf("http://%s:8080", podIP)}
			curlPod := newNetpolTestPod("netpol-curl-", operatorNamespace, curlImage, cmd, nil, "")
			createdCurl, err := kubeClient.CoreV1().Pods(operatorNamespace).Create(ctx, curlPod, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			curlName = createdCurl.Name
			result := waitForCurlResult(ctx, kubeClient, operatorNamespace, curlName)
			o.Expect(result).NotTo(o.Equal("000"), "traffic to unlabeled pod should not be blocked")
		})
		g.It("should block ingress from the same namespace [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			assertTrafficBlocked(ctx, kubeClient, true, operatorNamespace, nil)
		})
		g.It("should block ingress from a different namespace [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			assertTrafficBlocked(ctx, kubeClient, true, "default", nil)
		})
		g.It("should block egress to the API server [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			assertTrafficBlocked(ctx, kubeClient, false, operatorNamespace,
				utilpointer.StringPtr("https://kubernetes.default.svc.cluster.local/healthz"))
		})
		g.It("should block egress to external IPs [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			assertTrafficBlocked(ctx, kubeClient, false, operatorNamespace,
				utilpointer.StringPtr("https://1.1.1.1"))
		})
	})

	g.Context("NetworkPolicy Reconciliation", func() {
		g.It("should revert patched ingress rules [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			patchAndExpectRevert(ctx, kubeClient, func(np *networkingv1.NetworkPolicy) {
				np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: protocolPtr(corev1.ProtocolTCP), Port: portPtr(9999)},
				}}}
			}, func(np *networkingv1.NetworkPolicy) bool { return len(np.Spec.Ingress) == 0 })
		})
		g.It("should revert patched egress rules [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			patchAndExpectRevert(ctx, kubeClient, func(np *networkingv1.NetworkPolicy) {
				np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{
					{To: []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}}}},
				}
			}, func(np *networkingv1.NetworkPolicy) bool { return len(np.Spec.Egress) == 0 })
		})
		g.It("should revert patched podSelector [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			patchAndExpectRevert(ctx, kubeClient, func(np *networkingv1.NetworkPolicy) {
				np.Spec.PodSelector.MatchLabels["runoncedurationoverride"] = "false"
			}, func(np *networkingv1.NetworkPolicy) bool {
				return np.Spec.PodSelector.MatchLabels["runoncedurationoverride"] == "true"
			})
		})
		g.It("should restore removed policyTypes [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			patchAndExpectRevert(ctx, kubeClient, func(np *networkingv1.NetworkPolicy) {
				np.Spec.PolicyTypes = []networkingv1.PolicyType{}
			}, func(np *networkingv1.NetworkPolicy) bool { return len(np.Spec.PolicyTypes) == 2 })
		})
		g.It("should recreate a deleted NetworkPolicy [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			err := kubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Delete(ctx, netpolName, metav1.DeleteOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Eventually(func() error {
				_, err := kubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Get(ctx, netpolName, metav1.GetOptions{})
				return err
			}, 30*time.Second, 2*time.Second).Should(o.Succeed())
		})
		g.It("should recreate the NetworkPolicy with the correct spec [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			verifyExpectedNetworkPolicySpec(ctx, kubeClient)
		})
	})

	g.Context("NetworkPolicy Config Drift Recovery", func() {
		g.It("should reconcile a tampered policy after operator restart [Suite:openshift/run-once-duration-override-operator/operator/serial]", func() {
			g.DeferCleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
				defer cancel()
				scaleOperator(cleanupCtx, kubeClient, 1)
			})

			g.By("Scaling operator to zero replicas")
			scaleOperator(ctx, kubeClient, 0)
			o.Eventually(func() bool {
				pods, err := kubeClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: "runoncedurationoverride.operator=true",
				})
				if err != nil {
					return false
				}
				return len(pods.Items) == 0
			}, 2*time.Minute, 2*time.Second).Should(o.BeTrue())

			g.By("Tampering with the NetworkPolicy while operator is down")
			patchNetworkPolicy(ctx, kubeClient, func(np *networkingv1.NetworkPolicy) {
				np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: protocolPtr(corev1.ProtocolTCP), Port: portPtr(9999)},
				}}}
				np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{
					{To: []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}}}},
				}
			})

			g.By("Scaling operator back up")
			scaleOperator(ctx, kubeClient, 1)
			o.Eventually(func() int32 {
				deploy, err := kubeClient.AppsV1().Deployments(operatorNamespace).Get(ctx, "run-once-duration-override-operator", metav1.GetOptions{})
				if err != nil {
					return 0
				}
				return deploy.Status.ReadyReplicas
			}, 2*time.Minute, 2*time.Second).Should(o.BeNumerically(">=", int32(1)))

			g.By("Verifying operator reconciled the policy back to deny-all")
			o.Eventually(func() bool {
				np, err := kubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Get(ctx, netpolName, metav1.GetOptions{})
				return err == nil && len(np.Spec.Ingress) == 0 && len(np.Spec.Egress) == 0
			}, 2*time.Minute, 2*time.Second).Should(o.BeTrue())
			verifyExpectedNetworkPolicySpec(ctx, kubeClient)
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
						required.Spec.Template.Spec.Containers[0].Env[i].Value = "quay.io/jchaloup/run-once-duration-override:4.22.0"
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
				if errors.IsAlreadyExists(err) {
					return nil
				}
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
		podItems, err := kubeClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{})
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
		podItems, err := kubeClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{})
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

	klog.Infof("Waiting for %s NetworkPolicy to be created by operator", netpolName)
	deadline = time.Now().Add(2 * time.Minute)
	netpolReady := false
	for time.Now().Before(deadline) {
		_, err = kubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Get(ctx, netpolName, metav1.GetOptions{})
		if err == nil {
			klog.Infof("%s NetworkPolicy found", netpolName)
			netpolReady = true
			break
		}
		time.Sleep(5 * time.Second)
	}
	if !netpolReady {
		return ctx, cancelFnc, nil, fmt.Errorf("%s NetworkPolicy not created by operator after timeout", netpolName)
	}

	klog.Infof("All operator components are running and ready")
	return ctx, cancelFnc, kubeClient, nil
}

func verifyExpectedNetworkPolicySpec(ctx context.Context, kubeClient *k8sclient.Clientset) {
	np, err := kubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Get(ctx, netpolName, metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(np.Spec.PolicyTypes).To(o.ConsistOf(networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress))
	o.Expect(np.Spec.PodSelector.MatchLabels).To(o.HaveKeyWithValue("runoncedurationoverride", "true"))
	o.Expect(np.Spec.Ingress).To(o.BeEmpty())
	o.Expect(np.Spec.Egress).To(o.BeEmpty())
}

func assertTrafficBlocked(ctx context.Context, kubeClient *k8sclient.Clientset, needsTarget bool, curlNamespace string, url *string) {
	if curlNamespace != operatorNamespace {
		ensureTestServiceAccount(ctx, kubeClient, curlNamespace)
	}

	cm, err := kubeClient.CoreV1().ConfigMaps(operatorNamespace).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "netpol-test-owner-", Namespace: operatorNamespace},
		Data:       map[string]string{"purpose": "fake owner for netpol test pods"},
	}, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	ownerUID := cm.UID

	var targetName, curlName string
	g.DeferCleanup(func() {
		fd := metav1.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(0)}
		if targetName != "" {
			_ = kubeClient.CoreV1().Pods(operatorNamespace).Delete(ctx, targetName, fd)
		}
		if curlName != "" {
			_ = kubeClient.CoreV1().Pods(curlNamespace).Delete(ctx, curlName, fd)
		}
		_ = kubeClient.CoreV1().ConfigMaps(operatorNamespace).Delete(ctx, cm.Name, metav1.DeleteOptions{})
		if curlNamespace != operatorNamespace {
			_ = kubeClient.CoreV1().ServiceAccounts(curlNamespace).Delete(ctx, testSAName, metav1.DeleteOptions{})
		}
	})

	curlURL := ""
	if needsTarget {
		pod := newHTTPServerPod("netpol-target-", operatorNamespace, &ownerUID, cm.Name)
		created, createErr := kubeClient.CoreV1().Pods(operatorNamespace).Create(ctx, pod, metav1.CreateOptions{})
		o.Expect(createErr).NotTo(o.HaveOccurred())
		targetName = created.Name
		podIP := waitForPodReady(ctx, kubeClient, operatorNamespace, targetName)
		curlURL = fmt.Sprintf("http://%s:8080", podIP)
	} else {
		curlURL = *url
	}

	var curlOwner *types.UID
	if !needsTarget {
		curlOwner = &ownerUID
	}
	cmd := []string{"curl", "-sk", "--connect-timeout", "5", "-o", "/dev/null", "-w", "%{http_code}", curlURL}
	curlPod := newNetpolTestPod("netpol-curl-", curlNamespace, curlImage, cmd, curlOwner, cm.Name)
	created, err := kubeClient.CoreV1().Pods(curlNamespace).Create(ctx, curlPod, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	curlName = created.Name
	result := waitForCurlResult(ctx, kubeClient, curlNamespace, curlName)
	o.Expect(result).To(o.Equal("000"), "traffic should be blocked (expected curl output '000')")
}

func newNetpolTestPod(prefix, namespace, image string, command []string, ownerUID *types.UID, ownerCMName string) *corev1.Pod {
	saName := testSAName
	if namespace == operatorNamespace {
		saName = operatorSAName
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix,
			Namespace:    namespace,
			Annotations:  map[string]string{"openshift.io/required-scc": "nonroot-v2"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: saName,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: utilpointer.BoolPtr(true), RunAsUser: utilpointer.Int64Ptr(1000),
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{{
				Name: "test", Image: image, Command: command,
				TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: utilpointer.BoolPtr(false),
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
			}},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
	if ownerUID != nil {
		pod.Labels = map[string]string{"runoncedurationoverride": "true"}
		pod.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "v1", Kind: "ConfigMap", Name: ownerCMName, UID: *ownerUID,
			Controller: utilpointer.BoolPtr(true), BlockOwnerDeletion: utilpointer.BoolPtr(false),
		}}
	}
	return pod
}

func newHTTPServerPod(prefix, namespace string, ownerUID *types.UID, ownerCMName string) *corev1.Pod {
	pod := newNetpolTestPod(prefix, namespace, httpServerImage, []string{"python3", "-m", "http.server", "8080"}, ownerUID, ownerCMName)
	pod.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 8080}}
	pod.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstr.FromInt32(8080)},
		},
	}
	return pod
}

func waitForPodReady(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, name string) string {
	var podIP string
	o.Eventually(func() bool {
		p, err := kubeClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil || len(p.Status.ContainerStatuses) == 0 || !p.Status.ContainerStatuses[0].Ready {
			return false
		}
		podIP = p.Status.PodIP
		return true
	}, 2*time.Minute, 2*time.Second).Should(o.BeTrue())
	return podIP
}

func waitForCurlResult(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, name string) string {
	var logs []byte
	o.Eventually(func() bool {
		p, err := kubeClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil || (p.Status.Phase != corev1.PodSucceeded && p.Status.Phase != corev1.PodFailed) {
			return false
		}
		logs, _ = kubeClient.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{}).Do(ctx).Raw()
		return true
	}, 90*time.Second, 2*time.Second).Should(o.BeTrue())
	return string(logs)
}

func patchAndExpectRevert(ctx context.Context, kubeClient *k8sclient.Clientset, mutate func(*networkingv1.NetworkPolicy), check func(*networkingv1.NetworkPolicy) bool) {
	patchNetworkPolicy(ctx, kubeClient, mutate)
	o.Eventually(func() bool {
		np, err := kubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Get(ctx, netpolName, metav1.GetOptions{})
		return err == nil && check(np)
	}, 30*time.Second, 2*time.Second).Should(o.BeTrue())
}

func patchNetworkPolicy(ctx context.Context, kubeClient *k8sclient.Clientset, mutate func(*networkingv1.NetworkPolicy)) {
	np, err := kubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Get(ctx, netpolName, metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	mutate(np)
	_, err = kubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Update(ctx, np, metav1.UpdateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
}

func ensureTestServiceAccount(ctx context.Context, kubeClient *k8sclient.Clientset, namespace string) {
	_, err := kubeClient.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: testSAName, Namespace: namespace},
	}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func scaleOperator(ctx context.Context, kubeClient *k8sclient.Clientset, replicas int32) {
	scale, err := kubeClient.AppsV1().Deployments(operatorNamespace).GetScale(ctx, "run-once-duration-override-operator", metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	scale.Spec.Replicas = replicas
	_, err = kubeClient.AppsV1().Deployments(operatorNamespace).UpdateScale(ctx, "run-once-duration-override-operator", scale, metav1.UpdateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
}

func protocolPtr(p corev1.Protocol) *corev1.Protocol { return &p }
func portPtr(port int) *intstr.IntOrString           { v := intstr.FromInt32(int32(port)); return &v }

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

	ensureTestServiceAccount(ctx, kubeClient, testNamespace)

	// Create a test pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   testNamespace,
			Name:        "test-mutating-admission-pod",
			Annotations: map[string]string{"openshift.io/required-scc": "nonroot-v2"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: testSAName,
			RestartPolicy:      corev1.RestartPolicyOnFailure,
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
				Name:                     "pause",
				ImagePullPolicy:          "Always",
				Image:                    "kubernetes/pause",
				Ports:                    []corev1.ContainerPort{{ContainerPort: 80}},
				TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
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
