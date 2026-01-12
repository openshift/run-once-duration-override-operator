package operator

import (
	"context"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/operator/events"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	fakeclientset "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/fake"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/operatorclient"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/targetconfigcontroller"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

var daemonSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}

// fakeSyncContext implements factory.SyncContext for testing
type fakeSyncContext struct {
	recorder events.Recorder
}

func (f *fakeSyncContext) Queue() workqueue.RateLimitingInterface {
	return nil
}

func (f *fakeSyncContext) QueueKey() string {
	return operatorclient.OperatorConfigName
}

func (f *fakeSyncContext) Recorder() events.Recorder {
	return f.recorder
}

type testOperatorSetup struct {
	kubeClient              *kubefake.Clientset
	operatorClient          *fakeclientset.Clientset
	expectedNames           *expectedResourceNames
	ctx                     context.Context
	cancel                  context.CancelFunc
	namespace               string
	kubeInformerFactory     informers.SharedInformerFactory
	operatorInformerFactory operatorinformers.SharedInformerFactory
	runtimeContext          operatorruntime.OperandContext
	recorder                events.Recorder
}

func setupTestOperator(t *testing.T) *testOperatorSetup {
	namespace := "test-namespace"
	operatorName := "test-operator"
	crName := DefaultCR

	fakeKubeClient := kubefake.NewSimpleClientset()

	setDaemonSetReady := func(ds *appsv1.DaemonSet) {
		ds.Status.ObservedGeneration = ds.Generation
		ds.Status.DesiredNumberScheduled = 1
		ds.Status.CurrentNumberScheduled = 1
		ds.Status.NumberAvailable = 1
		ds.Status.UpdatedNumberScheduled = 1
		ds.Status.NumberUnavailable = 0
	}

	// Add reactor to automatically update DaemonSet status after create/update
	fakeKubeClient.PrependReactor("create", "daemonsets", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(kubetesting.CreateAction)
		ds := createAction.GetObject().(*appsv1.DaemonSet).DeepCopy()

		err = fakeKubeClient.Tracker().Create(daemonSetGVR, ds, ds.Namespace)
		if err != nil {
			return true, nil, err
		}

		setDaemonSetReady(ds)
		err = fakeKubeClient.Tracker().Update(daemonSetGVR, ds, ds.Namespace)

		return true, ds, err
	})

	fakeKubeClient.PrependReactor("update", "daemonsets", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
		updateAction := action.(kubetesting.UpdateAction)
		ds := updateAction.GetObject().(*appsv1.DaemonSet).DeepCopy()

		err = fakeKubeClient.Tracker().Update(daemonSetGVR, ds, ds.Namespace)
		if err != nil {
			return true, nil, err
		}

		setDaemonSetReady(ds)
		err = fakeKubeClient.Tracker().Update(daemonSetGVR, ds, ds.Namespace)

		return true, ds, err
	})

	fakeOperatorClient := fakeclientset.NewSimpleClientset(&runoncedurationoverridev1.RunOnceDurationOverride{
		ObjectMeta: metav1.ObjectMeta{
			Name: crName,
		},
		Spec: runoncedurationoverridev1.RunOnceDurationOverrideSpec{
			RunOnceDurationOverrideConfig: runoncedurationoverridev1.RunOnceDurationOverrideConfig{
				Spec: runoncedurationoverridev1.RunOnceDurationOverrideConfigSpec{
					ActiveDeadlineSeconds: 3600,
				},
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	return &testOperatorSetup{
		kubeClient:     fakeKubeClient,
		operatorClient: fakeOperatorClient,
		expectedNames: &expectedResourceNames{
			deployment:                        operatorName,
			daemonSet:                         operatorName,
			service:                           operatorName,
			serviceAccount:                    operatorName,
			secret:                            "server-serving-cert-" + operatorName,
			configMapConfiguration:            operatorName + "-configuration",
			configMapCABundle:                 operatorName + "-service-serving",
			mutatingWebhookConfiguration:      "runoncedurationoverrides.admission.runoncedurationoverride.openshift.io",
			apiService:                        "v1.admission.runoncedurationoverride.openshift.io",
			roleKubeSystem:                    "extension-apiserver-authentication-reader",
			roleBindingKubeSystem:             "extension-server-authentication-reader-" + operatorName,
			roleSCCHostNetwork:                operatorName + "-scc-hostnetwork-use",
			roleBindingSCCHostNetwork:         operatorName + "-scc-hostnetwork-use",
			clusterRoleRequester:              "system:" + operatorName + "-requester",
			clusterRoleDefault:                "default-aggregated-apiserver-" + operatorName,
			clusterRoleAnonymousAccess:        operatorName + "-anonymous-access",
			clusterRoleBindingDefault:         "default-aggregated-apiserver-" + operatorName,
			clusterRoleBindingAuthDelegator:   "auth-delegator-" + operatorName,
			clusterRoleBindingAnonymousAccess: operatorName + "-anonymous-access",
		},
		ctx:       ctx,
		cancel:    cancel,
		namespace: namespace,
		kubeInformerFactory: informers.NewSharedInformerFactoryWithOptions(
			fakeKubeClient,
			10*time.Minute,
			informers.WithNamespace(namespace),
		),
		operatorInformerFactory: operatorinformers.NewSharedInformerFactory(
			fakeOperatorClient,
			DefaultResyncPeriodPrimaryResource,
		),
		runtimeContext: operatorruntime.NewOperandContext(operatorName, namespace, crName, "test-image:latest", "v1.0.0"),
		recorder:       events.NewLoggingEventRecorder(operatorName, clock.RealClock{}),
	}
}

func TestOperatorReconciliation(t *testing.T) {
	// Initialize klog flags
	// klog.InitFlags(nil)

	// Set verbosity level (higher number = more verbose)
	// 0 = errors only, 1-4 = info, 5-9 = debug, 10+ = trace
	// flag.Set("v", "4")

	// Set required environment variables for RunOperator
	os.Setenv(OperandImageEnvName, "test-image:latest")
	os.Setenv(OperandVersionEnvName, "v1.0.0")
	defer os.Unsetenv(OperandImageEnvName)
	defer os.Unsetenv(OperandVersionEnvName)

	setup := setupTestOperator(t)
	defer setup.cancel()

	// Create controller first - this registers informers via factory.New().WithFilteredEventsInformers
	controller := targetconfigcontroller.NewTargetConfigController(
		&operatorclient.RunOnceDurationOverrideClient{
			Ctx:                             setup.ctx,
			RunOnceDurationOverrideInformer: setup.operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides(),
			OperatorClient:                  setup.operatorClient.RunOnceDurationOverrideV1(),
		},
		setup.kubeClient,
		setup.runtimeContext,
		setup.kubeInformerFactory,
		setup.operatorInformerFactory,
		setup.recorder,
	)

	// Start informers and wait for caches to sync
	setup.kubeInformerFactory.Start(setup.ctx.Done())
	setup.operatorInformerFactory.Start(setup.ctx.Done())
	setup.kubeInformerFactory.WaitForCacheSync(setup.ctx.Done())
	setup.operatorInformerFactory.WaitForCacheSync(setup.ctx.Done())

	verifyResources(t, setup.ctx, setup.kubeClient, setup.namespace, setup.expectedNames, true)

	for i := 0; i < 2; i++ {
		if err := controller.Sync(setup.ctx, &fakeSyncContext{recorder: setup.recorder}); err != nil {
			t.Logf("Sync %d returned error: %v", i+1, err)
		} else {
			t.Logf("Sync %d succeeded", i+1)
		}

		// Give watch events time to propagate to informers
		time.Sleep(100 * time.Millisecond)
	}

	verifyResources(t, setup.ctx, setup.kubeClient, setup.namespace, setup.expectedNames, false)

	t.Log("Controller sync sequence completed")
}

type expectedResourceNames struct {
	deployment                        string
	daemonSet                         string
	service                           string
	serviceAccount                    string
	secret                            string
	configMapConfiguration            string
	configMapCABundle                 string
	mutatingWebhookConfiguration      string
	apiService                        string
	roleKubeSystem                    string
	roleBindingKubeSystem             string
	roleSCCHostNetwork                string
	roleBindingSCCHostNetwork         string
	clusterRoleRequester              string
	clusterRoleDefault                string
	clusterRoleAnonymousAccess        string
	clusterRoleBindingDefault         string
	clusterRoleBindingAuthDelegator   string
	clusterRoleBindingAnonymousAccess string
}

func verifyResources(t *testing.T, ctx context.Context, client *kubefake.Clientset, namespace string, expected *expectedResourceNames, expectZero bool) {
	checkResource := func(name, resourceType string, exists bool) {
		if expectZero && exists {
			t.Errorf("expected no %s named %q, but it exists", resourceType, name)
		} else if !expectZero {
			t.Logf("%s %q: exists=%v", resourceType, name, exists)
		}
	}

	if ds, err := client.AppsV1().DaemonSets(namespace).Get(ctx, expected.daemonSet, metav1.GetOptions{}); err == nil {
		checkResource(expected.daemonSet, "DaemonSet", ds != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected DaemonSet %q to exist, but it was not found", expected.daemonSet)
		} else {
			t.Errorf("error getting DaemonSet %q: %v", expected.daemonSet, err)
		}
	}

	if sa, err := client.CoreV1().ServiceAccounts(namespace).Get(ctx, expected.serviceAccount, metav1.GetOptions{}); err == nil {
		checkResource(expected.serviceAccount, "ServiceAccount", sa != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ServiceAccount %q to exist, but it was not found", expected.serviceAccount)
		} else {
			t.Errorf("error getting ServiceAccount %q: %v", expected.serviceAccount, err)
		}
	}

	if sec, err := client.CoreV1().Secrets(namespace).Get(ctx, expected.secret, metav1.GetOptions{}); err == nil {
		checkResource(expected.secret, "Secret", sec != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected Secret %q to exist, but it was not found", expected.secret)
		} else {
			t.Errorf("error getting Secret %q: %v", expected.secret, err)
		}
	}

	if cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, expected.configMapConfiguration, metav1.GetOptions{}); err == nil {
		checkResource(expected.configMapConfiguration, "ConfigMap", cm != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ConfigMap %q to exist, but it was not found", expected.configMapConfiguration)
		} else {
			t.Errorf("error getting ConfigMap %q: %v", expected.configMapConfiguration, err)
		}
	}

	if cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, expected.configMapCABundle, metav1.GetOptions{}); err == nil {
		checkResource(expected.configMapCABundle, "ConfigMap", cm != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ConfigMap %q to exist, but it was not found", expected.configMapCABundle)
		} else {
			t.Errorf("error getting ConfigMap %q: %v", expected.configMapCABundle, err)
		}
	}

	if wh, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, expected.mutatingWebhookConfiguration, metav1.GetOptions{}); err == nil {
		checkResource(expected.mutatingWebhookConfiguration, "MutatingWebhookConfiguration", wh != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected MutatingWebhookConfiguration %q to exist, but it was not found", expected.mutatingWebhookConfiguration)
		} else {
			t.Errorf("error getting MutatingWebhookConfiguration %q: %v", expected.mutatingWebhookConfiguration, err)
		}
	}

	if rb, err := client.RbacV1().RoleBindings("kube-system").Get(ctx, expected.roleBindingKubeSystem, metav1.GetOptions{}); err == nil {
		checkResource(expected.roleBindingKubeSystem, "RoleBinding(kube-system)", rb != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected RoleBinding %q (kube-system) to exist, but it was not found", expected.roleBindingKubeSystem)
		} else {
			t.Errorf("error getting RoleBinding %q (kube-system): %v", expected.roleBindingKubeSystem, err)
		}
	}

	if role, err := client.RbacV1().Roles(namespace).Get(ctx, expected.roleSCCHostNetwork, metav1.GetOptions{}); err == nil {
		checkResource(expected.roleSCCHostNetwork, "Role", role != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected Role %q to exist, but it was not found", expected.roleSCCHostNetwork)
		} else {
			t.Errorf("error getting Role %q: %v", expected.roleSCCHostNetwork, err)
		}
	}

	if rb, err := client.RbacV1().RoleBindings(namespace).Get(ctx, expected.roleBindingSCCHostNetwork, metav1.GetOptions{}); err == nil {
		checkResource(expected.roleBindingSCCHostNetwork, "RoleBinding", rb != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected RoleBinding %q to exist, but it was not found", expected.roleBindingSCCHostNetwork)
		} else {
			t.Errorf("error getting RoleBinding %q: %v", expected.roleBindingSCCHostNetwork, err)
		}
	}

	if cr, err := client.RbacV1().ClusterRoles().Get(ctx, expected.clusterRoleRequester, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleRequester, "ClusterRole", cr != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ClusterRole %q to exist, but it was not found", expected.clusterRoleRequester)
		} else {
			t.Errorf("error getting ClusterRole %q: %v", expected.clusterRoleRequester, err)
		}
	}

	if cr, err := client.RbacV1().ClusterRoles().Get(ctx, expected.clusterRoleDefault, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleDefault, "ClusterRole", cr != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ClusterRole %q to exist, but it was not found", expected.clusterRoleDefault)
		} else {
			t.Errorf("error getting ClusterRole %q: %v", expected.clusterRoleDefault, err)
		}
	}

	if cr, err := client.RbacV1().ClusterRoles().Get(ctx, expected.clusterRoleAnonymousAccess, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleAnonymousAccess, "ClusterRole", cr != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ClusterRole %q to exist, but it was not found", expected.clusterRoleAnonymousAccess)
		} else {
			t.Errorf("error getting ClusterRole %q: %v", expected.clusterRoleAnonymousAccess, err)
		}
	}

	if crb, err := client.RbacV1().ClusterRoleBindings().Get(ctx, expected.clusterRoleBindingDefault, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleBindingDefault, "ClusterRoleBinding", crb != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ClusterRoleBinding %q to exist, but it was not found", expected.clusterRoleBindingDefault)
		} else {
			t.Errorf("error getting ClusterRoleBinding %q: %v", expected.clusterRoleBindingDefault, err)
		}
	}

	if crb, err := client.RbacV1().ClusterRoleBindings().Get(ctx, expected.clusterRoleBindingAuthDelegator, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleBindingAuthDelegator, "ClusterRoleBinding", crb != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ClusterRoleBinding %q to exist, but it was not found", expected.clusterRoleBindingAuthDelegator)
		} else {
			t.Errorf("error getting ClusterRoleBinding %q: %v", expected.clusterRoleBindingAuthDelegator, err)
		}
	}

	if crb, err := client.RbacV1().ClusterRoleBindings().Get(ctx, expected.clusterRoleBindingAnonymousAccess, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleBindingAnonymousAccess, "ClusterRoleBinding", crb != nil)
	} else if !expectZero {
		if k8serrors.IsNotFound(err) {
			t.Errorf("expected ClusterRoleBinding %q to exist, but it was not found", expected.clusterRoleBindingAnonymousAccess)
		} else {
			t.Errorf("error getting ClusterRoleBinding %q: %v", expected.clusterRoleBindingAnonymousAccess, err)
		}
	}
}
