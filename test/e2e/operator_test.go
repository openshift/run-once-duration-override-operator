package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextclientv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	utilpointer "k8s.io/utils/pointer"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	runoncedurationoverrideclient "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
	runoncedurationoverridescheme "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/scheme"
	"github.com/openshift/run-once-duration-override-operator/test/e2e/bindata"
)

func TestMain(m *testing.M) {
	if os.Getenv("KUBECONFIG") == "" {
		klog.Errorf("KUBECONFIG environment variable not set")
		os.Exit(1)
	}

	if os.Getenv("RELEASE_IMAGE_LATEST") == "" {
		klog.Errorf("RELEASE_IMAGE_LATEST environment variable not set")
		os.Exit(1)
	}

	if os.Getenv("NAMESPACE") == "" {
		klog.Errorf("NAMESPACE environment variable not set")
		os.Exit(1)
	}

	kubeClient := getKubeClientOrDie()
	apiExtClient := getApiExtensionKubeClient()
	runOnceDurationOverrideClient := getRunOnceDurationOverrideClient()

	eventRecorder := events.NewKubeRecorder(kubeClient.CoreV1().Events("default"), "test-e2e", &corev1.ObjectReference{})

	ctx, cancelFnc := context.WithCancel(context.TODO())
	defer cancelFnc()

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
				// override the operator image with the one built in the CI

				// E.g. RELEASE_IMAGE_LATEST=registry.build01.ci.openshift.org/ci-op-fy99k61r/release@sha256:0d05e600baea6df9dcd453d3b72c925b8672685cd94f0615c1089af4aa39f215
				registry := strings.Split(os.Getenv("RELEASE_IMAGE_LATEST"), "/")[0]

				required.Spec.Template.Spec.Containers[0].Image = registry + "/" + os.Getenv("NAMESPACE") + "/pipeline:run-once-duration-override-operator"
				// RELATED_IMAGE_OPERAND_IMAGE env
				for i, env := range required.Spec.Template.Spec.Containers[0].Env {
					if env.Name == "RELATED_IMAGE_OPERAND_IMAGE" {
						required.Spec.Template.Spec.Containers[0].Env[i].Value = "registry.ci.openshift.org/ocp/4.14:run-once-duration-override-webhook"
						break
					}
				}

				_, _, err := resourceapply.ApplyDeployment(
					ctx,
					kubeClient.AppsV1(),
					eventRecorder,
					required,
					1000, // any random high number
				)
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
					klog.Errorf("Unable to decode assets/09_cr.yaml: %v", err)
					return err
				}
				requiredSS := requiredObj.(*runoncedurationoverridev1.RunOnceDurationOverride)

				_, err = runOnceDurationOverrideClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Create(ctx, requiredSS, metav1.CreateOptions{})
				return err
			},
		},
	}

	// create required resources, e.g. namespace, crd, roles
	if err := wait.PollImmediate(1*time.Second, 10*time.Second, func() (bool, error) {
		for _, asset := range assets {
			klog.Infof("Creating %v", asset.path)
			if err := asset.readerAndApply(bindata.MustAsset(asset.path)); err != nil {
				klog.Errorf("Unable to create %v: %v", asset.path, err)
				return false, nil
			}
		}

		return true, nil
	}); err != nil {
		klog.Errorf("Unable to create RODOO resources: %v", err)
		os.Exit(1)
	}

	var runOnceDurationoverridePod *corev1.Pod
	// Wait until the run once duration override pod is running
	if err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (bool, error) {
		klog.Infof("Listing pods...")
		podItems, err := kubeClient.CoreV1().Pods("run-once-duration-override-operator").List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Errorf("Unable to list pods: %v", err)
			return false, nil
		}
		for _, pod := range podItems.Items {
			if !strings.HasPrefix(pod.Name, "run-once-duration-override-") {
				continue
			}
			klog.Infof("Checking pod: %v, phase: %v, deletionTS: %v\n", pod.Name, pod.Status.Phase, pod.GetDeletionTimestamp())
			if pod.Status.Phase == corev1.PodRunning && pod.GetDeletionTimestamp() == nil {
				runOnceDurationoverridePod = pod.DeepCopy()
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		klog.Errorf("Unable to wait for the RODOO pod to run")
		os.Exit(1)
	}

	klog.Infof("Run Once Duration Override running in %v", runOnceDurationoverridePod.Name)

	webhooksRunning := make(map[string]bool) // node, exists
	// Get the number of master nodes
	if err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (bool, error) {
		klog.Infof("Listing nodes...")
		nodeItems, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Errorf("Unable to list nodes: %v", err)
			return false, nil
		}
		for _, node := range nodeItems.Items {
			if _, exists := node.Labels["node-role.kubernetes.io/master"]; !exists {
				continue
			}
			webhooksRunning[node.Name] = false
		}
		return true, nil
	}); err != nil {
		klog.Errorf("Unable to collect master nodes")
		os.Exit(1)
	}

	// Wait until the webhook pods are running
	if err := wait.PollImmediate(5*time.Second, time.Minute, func() (bool, error) {
		klog.Infof("Listing pods...")
		podItems, err := kubeClient.CoreV1().Pods("run-once-duration-override-operator").List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Errorf("Unable to list pods: %v", err)
			return false, nil
		}
		for _, pod := range podItems.Items {
			if !strings.HasPrefix(pod.Name, "runoncedurationoverride-") {
				continue
			}
			klog.Infof("Checking pod: %v, phase: %v, deletionTS: %v\n", pod.Name, pod.Status.Phase, pod.GetDeletionTimestamp())
			if pod.Status.Phase == corev1.PodRunning && pod.GetDeletionTimestamp() == nil {
				webhooksRunning[pod.Spec.NodeName] = true
			}
		}

		for node, running := range webhooksRunning {
			if !running {
				klog.Infof("Admission scheduled on node %q is not running", node)
				return false, nil
			}
		}

		return true, nil
	}); err != nil {
		klog.Errorf("Unable to wait for all webhook pods running: %v", err)
		os.Exit(1)
	}

	klog.Infof("All daemonset webhooks are running")

	os.Exit(m.Run())
}

func TestRunOnceDurationOverriding(t *testing.T) {
	// runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled: "true"
	ctx, cancelFnc := context.WithCancel(context.TODO())
	defer cancelFnc()

	clientSet := getKubeClientOrDie()

	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e-" + strings.ToLower(t.Name()),
			Labels: map[string]string{
				"runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled": "true",
			},
		},
	}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace.Name,
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
						Drop: []corev1.Capability{
							"ALL",
						},
					},
				},
				Name:            "pause",
				ImagePullPolicy: "Always",
				Image:           "kubernetes/pause",
				Ports:           []corev1.ContainerPort{{ContainerPort: 80}},
			}},
		},
	}

	if _, err := clientSet.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create a pod: %v", err)
	}

	if err := wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
		klog.Infof("Listing pods...")
		pod, err := clientSet.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Unable to get pod: %v", err)
			return false, nil
		}
		if pod.Spec.NodeName == "" {
			klog.Infof("Pod not yet assigned to a node")
			return false, nil
		}
		klog.Infof("Pod successfully assigned to a node: %v", pod.Spec.NodeName)

		if pod.Spec.ActiveDeadlineSeconds == nil {
			klog.Infof("pod.Spec.ActiveDeadlineSeconds is not set")
			return false, nil
		}

		if *pod.Spec.ActiveDeadlineSeconds != 800 {
			klog.Infof("pod.Spec.ActiveDeadlineSeconds is set to %d not 800", *pod.Spec.ActiveDeadlineSeconds)
			return false, nil
		}

		klog.Infof("pod.Spec.ActiveDeadlineSeconds = %v", *pod.Spec.ActiveDeadlineSeconds)

		return true, nil
	}); err != nil {
		t.Fatalf("Unable to wait for a scheduled pod: %v", err)
	}
}

func getKubeClientOrDie() *k8sclient.Clientset {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Errorf("Unable to build config: %v", err)
		os.Exit(1)
	}
	client, err := k8sclient.NewForConfig(config)
	if err != nil {
		klog.Errorf("Unable to build client: %v", err)
		os.Exit(1)
	}
	return client
}

func getApiExtensionKubeClient() *apiextclientv1.Clientset {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Errorf("Unable to build config: %v", err)
		os.Exit(1)
	}
	client, err := apiextclientv1.NewForConfig(config)
	if err != nil {
		klog.Errorf("Unable to build client: %v", err)
		os.Exit(1)
	}
	return client
}

func getRunOnceDurationOverrideClient() *runoncedurationoverrideclient.Clientset {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Errorf("Unable to build config: %v", err)
		os.Exit(1)
	}
	client, err := runoncedurationoverrideclient.NewForConfig(config)
	if err != nil {
		klog.Errorf("Unable to build client: %v", err)
		os.Exit(1)
	}
	return client
}
