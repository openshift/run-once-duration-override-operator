package operator

import (
	"context"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/operator/events"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	fakeclientset "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/fake"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
	fakeaggregator "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/fake"
)

var daemonSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				Name:            "test-operator",
				Namespace:       "test-namespace",
				ShutdownContext: context.Background(),
				RestConfig:      &rest.Config{},
				OperandImage:    "test-image:latest",
				OperandVersion:  "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "missing namespace",
			config: &Config{
				Name:            "test-operator",
				Namespace:       "",
				ShutdownContext: context.Background(),
				RestConfig:      &rest.Config{},
				OperandImage:    "test-image:latest",
				OperandVersion:  "v1.0.0",
			},
			wantErr: true,
			errMsg:  "operator namespace must be specified",
		},
		{
			name: "missing name",
			config: &Config{
				Name:            "",
				Namespace:       "test-namespace",
				ShutdownContext: context.Background(),
				RestConfig:      &rest.Config{},
				OperandImage:    "test-image:latest",
				OperandVersion:  "v1.0.0",
			},
			wantErr: true,
			errMsg:  "operator name must be specified",
		},
		{
			name: "missing rest config",
			config: &Config{
				Name:            "test-operator",
				Namespace:       "test-namespace",
				ShutdownContext: context.Background(),
				RestConfig:      nil,
				OperandImage:    "test-image:latest",
				OperandVersion:  "v1.0.0",
			},
			wantErr: true,
			errMsg:  "no rest.Config has been specified",
		},
		{
			name: "missing operand image",
			config: &Config{
				Name:            "test-operator",
				Namespace:       "test-namespace",
				ShutdownContext: context.Background(),
				RestConfig:      &rest.Config{},
				OperandImage:    "",
				OperandVersion:  "v1.0.0",
			},
			wantErr: true,
			errMsg:  "no operand image has been specified",
		},
		{
			name: "missing operand version",
			config: &Config{
				Name:            "test-operator",
				Namespace:       "test-namespace",
				ShutdownContext: context.Background(),
				RestConfig:      &rest.Config{},
				OperandImage:    "test-image:latest",
				OperandVersion:  "",
			},
			wantErr: true,
			errMsg:  "no operand version has been specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err.Error() != tt.errMsg {
				t.Errorf("Config.Validate() error = %v, expected %v", err.Error(), tt.errMsg)
			}
		})
	}
}

type testOperatorSetup struct {
	kubeClient              *kubefake.Clientset
	aggregatorClient        *fakeaggregator.Clientset
	expectedNames           *expectedResourceNames
	ctx                     context.Context
	cancel                  context.CancelFunc
	namespace               string
	kubeInformerFactory     informers.SharedInformerFactory
	operatorInformerFactory operatorinformers.SharedInformerFactory
	runtimeContext          operatorruntime.OperandContext
	mockClient              *operatorruntime.Client
	recorder                events.Recorder
}

func setupTestOperator(t *testing.T) *testOperatorSetup {
	namespace := "test-namespace"
	operatorName := "test-operator"
	crName := DefaultCR

	rodoo := &runoncedurationoverridev1.RunOnceDurationOverride{
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
	}

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

	fakeOperatorClient := fakeclientset.NewSimpleClientset(rodoo)
	fakeAggregatorClient := fakeaggregator.NewSimpleClientset()

	expectedNames := &expectedResourceNames{
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
	}

	scheme := runtime.NewScheme()
	_ = runoncedurationoverridev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = admissionregistrationv1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)

	kubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		fakeKubeClient,
		10*time.Minute,
		informers.WithNamespace(namespace),
	)

	operatorInformerFactory := operatorinformers.NewSharedInformerFactory(
		fakeOperatorClient,
		DefaultResyncPeriodPrimaryResource,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	runtimeContext := operatorruntime.NewOperandContext(operatorName, namespace, crName, "test-image:latest", "v1.0.0")

	mockClient := &operatorruntime.Client{
		Operator:        fakeOperatorClient,
		Kubernetes:      fakeKubeClient,
		APIRegistration: fakeAggregatorClient,
	}

	// create recorder for tests
	recorder := events.NewLoggingEventRecorder(operatorName, clock.RealClock{})

	return &testOperatorSetup{
		kubeClient:              fakeKubeClient,
		aggregatorClient:        fakeAggregatorClient,
		expectedNames:           expectedNames,
		ctx:                     ctx,
		cancel:                  cancel,
		namespace:               namespace,
		kubeInformerFactory:     kubeInformerFactory,
		operatorInformerFactory: operatorInformerFactory,
		runtimeContext:          runtimeContext,
		mockClient:              mockClient,
		recorder:                recorder,
	}
}

func TestOperatorReconciliation(t *testing.T) {
	// Initialize klog flags
	// klog.InitFlags(nil)

	// Set verbosity level (higher number = more verbose)
	// 0 = errors only, 1-4 = info, 5-9 = debug, 10+ = trace
	// flag.Set("v", "4")

	setup := setupTestOperator(t)
	defer setup.cancel()

	c, err := runoncedurationoverride.New(
		DefaultWorkerCount,
		setup.mockClient,
		setup.runtimeContext,
		setup.kubeInformerFactory,
		setup.operatorInformerFactory,
		setup.recorder,
	)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	setup.kubeInformerFactory.Start(setup.ctx.Done())
	setup.operatorInformerFactory.Start(setup.ctx.Done())

	for _, synced := range setup.kubeInformerFactory.WaitForCacheSync(setup.ctx.Done()) {
		if !synced {
			t.Fatal("failed to sync kube informer caches")
		}
	}
	for _, synced := range setup.operatorInformerFactory.WaitForCacheSync(setup.ctx.Done()) {
		if !synced {
			t.Fatal("failed to sync operator informer caches")
		}
	}

	verifyResources(t, setup.ctx, setup.kubeClient, setup.aggregatorClient, setup.namespace, setup.expectedNames, true)

	runnerErrorCh := make(chan error, 1)
	go c.Run(setup.ctx, runnerErrorCh)

	if err := <-runnerErrorCh; err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	time.Sleep(1 * time.Second)

	verifyResources(t, setup.ctx, setup.kubeClient, setup.aggregatorClient, setup.namespace, setup.expectedNames, false)

	setup.cancel()

	select {
	case <-c.Done():
		t.Log("Controller stopped successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for controller to stop")
	}
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

func verifyResources(t *testing.T, ctx context.Context, client *kubefake.Clientset, aggregatorClient *fakeaggregator.Clientset, namespace string, expected *expectedResourceNames, expectZero bool) {
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

// mockEnqueuer is a mock implementation of runtime.Enqueuer for testing
type mockEnqueuer struct{}

func (m *mockEnqueuer) Enqueue(owned interface{}) error {
	// No-op for testing
	return nil
}
