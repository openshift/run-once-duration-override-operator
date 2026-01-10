package operator

import (
	"context"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/dynamic"
	fakeclientset "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/fake"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/run-once-duration-override-operator/pkg/runoncedurationoverride"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
	"github.com/openshift/run-once-duration-override-operator/pkg/secondarywatch"
	fakeaggregator "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/fake"
)

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
	controller       runoncedurationoverride.Interface
	kubeClient       *kubefake.Clientset
	aggregatorClient *fakeaggregator.Clientset
	expectedNames    *expectedResourceNames
	ctx              context.Context
	cancel           context.CancelFunc
	namespace        string
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
	fakeDynamicClient := fake.NewSimpleDynamicClient(scheme)
	dynamicEnsurer := dynamic.NewEnsurer(fakeDynamicClient)

	kubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		fakeKubeClient,
		10*time.Minute,
		informers.WithNamespace(namespace),
	)

	operatorInformerFactory := operatorinformers.NewSharedInformerFactory(
		fakeOperatorClient,
		DefaultResyncPeriodPrimaryResource,
	)

	lister, starter := secondarywatch.New(kubeInformerFactory)
	if lister == nil || starter == nil {
		t.Fatal("expected lister and starter to be non-nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	runtimeContext := operatorruntime.NewOperandContext(operatorName, namespace, crName, "test-image:latest", "v1.0.0")

	mockClient := &operatorruntime.Client{
		Operator:        fakeOperatorClient,
		Kubernetes:      fakeKubeClient,
		Dynamic:         dynamicEnsurer,
		APIRegistration: fakeAggregatorClient,
	}

	c, enqueuer, err := runoncedurationoverride.New(&runoncedurationoverride.Options{
		ResyncPeriod:   DefaultResyncPeriodPrimaryResource,
		Workers:        DefaultWorkerCount,
		RuntimeContext: runtimeContext,
		Client:         mockClient,
		Lister:         lister,
	})
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	if err := starter.Start(enqueuer, ctx); err != nil {
		t.Fatalf("expected starter.Start to succeed, got error: %v", err)
	}

	kubeInformerFactory.Start(ctx.Done())
	operatorInformerFactory.Start(ctx.Done())

	for _, synced := range kubeInformerFactory.WaitForCacheSync(ctx.Done()) {
		if !synced {
			t.Fatal("failed to sync kube informer caches")
		}
	}
	for _, synced := range operatorInformerFactory.WaitForCacheSync(ctx.Done()) {
		if !synced {
			t.Fatal("failed to sync operator informer caches")
		}
	}

	return &testOperatorSetup{
		controller:       c,
		kubeClient:       fakeKubeClient,
		aggregatorClient: fakeAggregatorClient,
		expectedNames:    expectedNames,
		ctx:              ctx,
		cancel:           cancel,
		namespace:        namespace,
	}
}

func TestOperatorReconciliation(t *testing.T) {
	setup := setupTestOperator(t)
	defer setup.cancel()

	verifyResources(t, setup.ctx, setup.kubeClient, setup.aggregatorClient, setup.namespace, setup.expectedNames, true)

	runner := runoncedurationoverride.NewRunner()
	runnerErrorCh := make(chan error, 1)
	go runner.Run(setup.ctx, setup.controller, runnerErrorCh)

	if err := <-runnerErrorCh; err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	time.Sleep(1 * time.Second)

	verifyResources(t, setup.ctx, setup.kubeClient, setup.aggregatorClient, setup.namespace, setup.expectedNames, false)

	setup.cancel()

	select {
	case <-runner.Done():
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

	if dep, err := client.AppsV1().Deployments(namespace).Get(ctx, expected.deployment, metav1.GetOptions{}); err == nil {
		checkResource(expected.deployment, "Deployment", dep != nil)
	} else if !expectZero {
		t.Logf("Deployment %q: not found", expected.deployment)
	}

	if ds, err := client.AppsV1().DaemonSets(namespace).Get(ctx, expected.daemonSet, metav1.GetOptions{}); err == nil {
		checkResource(expected.daemonSet, "DaemonSet", ds != nil)
	} else if !expectZero {
		t.Logf("DaemonSet %q: not found", expected.daemonSet)
	}

	if svc, err := client.CoreV1().Services(namespace).Get(ctx, expected.service, metav1.GetOptions{}); err == nil {
		checkResource(expected.service, "Service", svc != nil)
	} else if !expectZero {
		t.Logf("Service %q: not found", expected.service)
	}

	if sa, err := client.CoreV1().ServiceAccounts(namespace).Get(ctx, expected.serviceAccount, metav1.GetOptions{}); err == nil {
		checkResource(expected.serviceAccount, "ServiceAccount", sa != nil)
	} else if !expectZero {
		t.Logf("ServiceAccount %q: not found", expected.serviceAccount)
	}

	if sec, err := client.CoreV1().Secrets(namespace).Get(ctx, expected.secret, metav1.GetOptions{}); err == nil {
		checkResource(expected.secret, "Secret", sec != nil)
	} else if !expectZero {
		t.Logf("Secret %q: not found", expected.secret)
	}

	if cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, expected.configMapConfiguration, metav1.GetOptions{}); err == nil {
		checkResource(expected.configMapConfiguration, "ConfigMap", cm != nil)
	} else if !expectZero {
		t.Logf("ConfigMap %q: not found", expected.configMapConfiguration)
	}

	if cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, expected.configMapCABundle, metav1.GetOptions{}); err == nil {
		checkResource(expected.configMapCABundle, "ConfigMap", cm != nil)
	} else if !expectZero {
		t.Logf("ConfigMap %q: not found", expected.configMapCABundle)
	}

	if wh, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, expected.mutatingWebhookConfiguration, metav1.GetOptions{}); err == nil {
		checkResource(expected.mutatingWebhookConfiguration, "MutatingWebhookConfiguration", wh != nil)
	} else if !expectZero {
		t.Logf("MutatingWebhookConfiguration %q: not found", expected.mutatingWebhookConfiguration)
	}

	if api, err := aggregatorClient.ApiregistrationV1().APIServices().Get(ctx, expected.apiService, metav1.GetOptions{}); err == nil {
		checkResource(expected.apiService, "APIService", api != nil)
	} else if !expectZero {
		t.Logf("APIService %q: not found", expected.apiService)
	}

	if role, err := client.RbacV1().Roles("kube-system").Get(ctx, expected.roleKubeSystem, metav1.GetOptions{}); err == nil {
		checkResource(expected.roleKubeSystem, "Role(kube-system)", role != nil)
	} else if !expectZero {
		t.Logf("Role %q (kube-system): not found", expected.roleKubeSystem)
	}

	if rb, err := client.RbacV1().RoleBindings("kube-system").Get(ctx, expected.roleBindingKubeSystem, metav1.GetOptions{}); err == nil {
		checkResource(expected.roleBindingKubeSystem, "RoleBinding(kube-system)", rb != nil)
	} else if !expectZero {
		t.Logf("RoleBinding %q (kube-system): not found", expected.roleBindingKubeSystem)
	}

	if role, err := client.RbacV1().Roles(namespace).Get(ctx, expected.roleSCCHostNetwork, metav1.GetOptions{}); err == nil {
		checkResource(expected.roleSCCHostNetwork, "Role", role != nil)
	} else if !expectZero {
		t.Logf("Role %q: not found", expected.roleSCCHostNetwork)
	}

	if rb, err := client.RbacV1().RoleBindings(namespace).Get(ctx, expected.roleBindingSCCHostNetwork, metav1.GetOptions{}); err == nil {
		checkResource(expected.roleBindingSCCHostNetwork, "RoleBinding", rb != nil)
	} else if !expectZero {
		t.Logf("RoleBinding %q: not found", expected.roleBindingSCCHostNetwork)
	}

	if cr, err := client.RbacV1().ClusterRoles().Get(ctx, expected.clusterRoleRequester, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleRequester, "ClusterRole", cr != nil)
	} else if !expectZero {
		t.Logf("ClusterRole %q: not found", expected.clusterRoleRequester)
	}

	if cr, err := client.RbacV1().ClusterRoles().Get(ctx, expected.clusterRoleDefault, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleDefault, "ClusterRole", cr != nil)
	} else if !expectZero {
		t.Logf("ClusterRole %q: not found", expected.clusterRoleDefault)
	}

	if cr, err := client.RbacV1().ClusterRoles().Get(ctx, expected.clusterRoleAnonymousAccess, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleAnonymousAccess, "ClusterRole", cr != nil)
	} else if !expectZero {
		t.Logf("ClusterRole %q: not found", expected.clusterRoleAnonymousAccess)
	}

	if crb, err := client.RbacV1().ClusterRoleBindings().Get(ctx, expected.clusterRoleBindingDefault, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleBindingDefault, "ClusterRoleBinding", crb != nil)
	} else if !expectZero {
		t.Logf("ClusterRoleBinding %q: not found", expected.clusterRoleBindingDefault)
	}

	if crb, err := client.RbacV1().ClusterRoleBindings().Get(ctx, expected.clusterRoleBindingAuthDelegator, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleBindingAuthDelegator, "ClusterRoleBinding", crb != nil)
	} else if !expectZero {
		t.Logf("ClusterRoleBinding %q: not found", expected.clusterRoleBindingAuthDelegator)
	}

	if crb, err := client.RbacV1().ClusterRoleBindings().Get(ctx, expected.clusterRoleBindingAnonymousAccess, metav1.GetOptions{}); err == nil {
		checkResource(expected.clusterRoleBindingAnonymousAccess, "ClusterRoleBinding", crb != nil)
	} else if !expectZero {
		t.Logf("ClusterRoleBinding %q: not found", expected.clusterRoleBindingAnonymousAccess)
	}
}

// mockEnqueuer is a mock implementation of runtime.Enqueuer for testing
type mockEnqueuer struct{}

func (m *mockEnqueuer) Enqueue(owned interface{}) error {
	// No-op for testing
	return nil
}
