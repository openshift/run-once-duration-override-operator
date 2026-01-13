package operator

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/crypto"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	fakeclientset "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/fake"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/configobservation/configobservercontroller"
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
	kubeClient                 *kubefake.Clientset
	operatorClient             *fakeclientset.Clientset
	operatorClientWrapper      *operatorclient.RunOnceDurationOverrideClient
	expectedNames              *expectedResourceNames
	ctx                        context.Context
	cancel                     context.CancelFunc
	namespace                  string
	kubeInformerFactory        informers.SharedInformerFactory
	operatorInformerFactory    operatorinformers.SharedInformerFactory
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces
	configClient               *configfake.Clientset
	configInformers            configinformers.SharedInformerFactory
	runtimeContext             operatorruntime.OperandContext
	recorder                   events.Recorder
}

type tlsTestControllers struct {
	configObserver         *configobservercontroller.ConfigObserver
	targetConfigController factory.Controller
}

// setupTLSTestControllers creates and initializes controllers for TLS configuration tests
func setupTLSTestControllers(t *testing.T, setup *testOperatorSetup) *tlsTestControllers {
	t.Helper()

	wrappedOperatorClient := operatorclient.NewOperatorClientWrapper(setup.operatorClientWrapper)

	configObserver := configobservercontroller.NewConfigObserver(
		wrappedOperatorClient,
		setup.configInformers,
		resourcesynccontroller.NewResourceSyncController(
			"RunOnceDurationOverrideOperator",
			wrappedOperatorClient,
			setup.kubeInformersForNamespaces,
			setup.kubeClient.CoreV1(),
			setup.kubeClient.CoreV1(),
			setup.recorder,
		),
		setup.recorder,
	)

	targetConfigController := targetconfigcontroller.NewTargetConfigController(
		setup.operatorClientWrapper,
		setup.kubeClient,
		setup.runtimeContext,
		setup.kubeInformerFactory,
		setup.operatorInformerFactory,
		setup.recorder,
	)

	// Start all informers
	setup.kubeInformersForNamespaces.Start(setup.ctx.Done())
	setup.configInformers.Start(setup.ctx.Done())
	setup.kubeInformerFactory.Start(setup.ctx.Done())
	setup.operatorInformerFactory.Start(setup.ctx.Done())

	// Wait for cache sync
	setup.kubeInformersForNamespaces.WaitForCacheSync(setup.ctx.Done())
	setup.configInformers.WaitForCacheSync(setup.ctx.Done())
	setup.kubeInformerFactory.WaitForCacheSync(setup.ctx.Done())
	setup.operatorInformerFactory.WaitForCacheSync(setup.ctx.Done())

	return &tlsTestControllers{
		configObserver:         configObserver,
		targetConfigController: targetConfigController,
	}
}

// syncControllers runs config observer sync followed by target config controller sync
func syncControllers(t *testing.T, setup *testOperatorSetup, controllers *tlsTestControllers, iterations int) {
	t.Helper()

	if err := controllers.configObserver.Sync(setup.ctx, &fakeSyncContext{recorder: setup.recorder}); err != nil {
		t.Fatalf("config observer sync failed: %v", err)
	}

	for i := 0; i < iterations; i++ {
		if err := controllers.targetConfigController.Sync(setup.ctx, &fakeSyncContext{recorder: setup.recorder}); err != nil {
			t.Logf("Sync iteration %d returned error: %v", i+1, err)
		}
	}
}

type testOperatorOptions struct {
	apiServer                *configv1.APIServer
	createInitialResources   bool
	addDaemonSetReadyReactor bool
}

func setupTestOperator(t *testing.T) *testOperatorSetup {
	return setupTestOperatorWithOptions(t, testOperatorOptions{
		addDaemonSetReadyReactor: true,
	})
}

func setupTestOperatorWithTLSConfig(t *testing.T, apiServer *configv1.APIServer) *testOperatorSetup {
	return setupTestOperatorWithOptions(t, testOperatorOptions{
		apiServer:              apiServer,
		createInitialResources: true,
	})
}

func setupTestOperatorWithOptions(t *testing.T, opts testOperatorOptions) *testOperatorSetup {
	// Use real operator constants for TLS tests, test constants for reconciliation tests
	var namespace, operatorName, crName string
	if opts.createInitialResources {
		namespace = operatorclient.OperatorNamespace
		operatorName = operatorclient.OperatorName
		crName = operatorclient.OperatorConfigName
	} else {
		namespace = "test-namespace"
		operatorName = "test-operator"
		crName = DefaultCR
	}

	// Create initial Kubernetes resources if requested
	var initialResources []runtime.Object
	if opts.createInitialResources {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            operatorName + "-configuration",
				Namespace:       namespace,
				ResourceVersion: "1",
			},
			Data: map[string]string{
				"override.yaml": "activeDeadlineSeconds: 3600",
			},
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "server-serving-cert-" + operatorName,
				Namespace:       namespace,
				ResourceVersion: "1",
			},
			Data: map[string][]byte{
				"tls.crt": []byte("cert-data"),
				"tls.key": []byte("key-data"),
			},
		}

		caConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            operatorName + "-service-serving",
				Namespace:       namespace,
				ResourceVersion: "1",
			},
			Data: map[string]string{
				"service-ca.crt": "ca-data",
			},
		}

		daemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorName,
				Namespace: namespace,
			},
			Status: appsv1.DaemonSetStatus{
				ObservedGeneration:     1,
				DesiredNumberScheduled: 1,
				CurrentNumberScheduled: 1,
				NumberAvailable:        1,
				UpdatedNumberScheduled: 1,
				NumberUnavailable:      0,
			},
		}

		initialResources = []runtime.Object{configMap, secret, caConfigMap, daemonSet}
	}

	fakeKubeClient := kubefake.NewSimpleClientset(initialResources...)

	// Add DaemonSet reactor if requested
	if opts.addDaemonSetReadyReactor {
		setDaemonSetReady := func(ds *appsv1.DaemonSet) {
			ds.Status.ObservedGeneration = ds.Generation
			ds.Status.DesiredNumberScheduled = 1
			ds.Status.CurrentNumberScheduled = 1
			ds.Status.NumberAvailable = 1
			ds.Status.UpdatedNumberScheduled = 1
			ds.Status.NumberUnavailable = 0
		}

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
	}

	// Create operator CR
	runOnceDurationOverride := &runoncedurationoverridev1.RunOnceDurationOverride{
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

	fakeOperatorClient := fakeclientset.NewSimpleClientset(runOnceDurationOverride)
	operatorInformerFactory := operatorinformers.NewSharedInformerFactory(fakeOperatorClient, DefaultResyncPeriodPrimaryResource)

	// Add to informer cache if we're creating initial resources
	if opts.createInitialResources {
		operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides().Informer().GetIndexer().Add(runOnceDurationOverride)
	}

	// Create operator client wrapper
	operatorClientWrapper := &operatorclient.RunOnceDurationOverrideClient{
		Ctx:                             context.TODO(),
		RunOnceDurationOverrideInformer: operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides(),
		OperatorClient:                  fakeOperatorClient.RunOnceDurationOverrideV1(),
	}

	// Create kubeInformersForNamespaces (needed for TLS tests)
	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(
		fakeKubeClient,
		"",
		namespace,
	)

	// Add initial resources to informer cache if created
	if opts.createInitialResources && len(initialResources) > 0 {
		for _, res := range initialResources {
			switch obj := res.(type) {
			case *corev1.ConfigMap:
				kubeInformersForNamespaces.InformersFor(namespace).Core().V1().ConfigMaps().Informer().GetIndexer().Add(obj)
			case *corev1.Secret:
				kubeInformersForNamespaces.InformersFor(namespace).Core().V1().Secrets().Informer().GetIndexer().Add(obj)
			}
		}
	}

	// Create config informers
	var configObjects []runtime.Object
	if opts.apiServer != nil {
		configObjects = append(configObjects, opts.apiServer)
	}
	fakeConfigClient := configfake.NewSimpleClientset(configObjects...)
	configInformers := configinformers.NewSharedInformerFactory(fakeConfigClient, 0)

	if opts.apiServer != nil {
		configInformers.Config().V1().APIServers().Informer().GetIndexer().Add(opts.apiServer)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	// Select recorder type
	var recorder events.Recorder
	if opts.createInitialResources {
		recorder = events.NewInMemoryRecorder("", clock.RealClock{})
	} else {
		recorder = events.NewLoggingEventRecorder(operatorName, clock.RealClock{})
	}

	setup := &testOperatorSetup{
		kubeClient:                 fakeKubeClient,
		operatorClient:             fakeOperatorClient,
		operatorClientWrapper:      operatorClientWrapper,
		ctx:                        ctx,
		cancel:                     cancel,
		namespace:                  namespace,
		kubeInformerFactory:        informers.NewSharedInformerFactoryWithOptions(fakeKubeClient, 10*time.Minute, informers.WithNamespace(namespace)),
		operatorInformerFactory:    operatorInformerFactory,
		kubeInformersForNamespaces: kubeInformersForNamespaces,
		configClient:               fakeConfigClient,
		configInformers:            configInformers,
		runtimeContext:             operatorruntime.NewOperandContext(operatorName, namespace, crName, "test-image:latest", "v1.0.0"),
		recorder:                   recorder,
	}

	// Only set expectedNames for non-TLS tests
	if !opts.createInitialResources {
		setup.expectedNames = &expectedResourceNames{
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
	}

	return setup
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

func TestDaemonSetTLSConfiguration(t *testing.T) {
	intermediateProfile := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	intermediateCiphers := crypto.OpenSSLToIANACipherSuites(intermediateProfile.Ciphers)

	tests := []struct {
		name                 string
		apiServer            *configv1.APIServer
		expectedCipherSuites string
		expectedMinTLSVer    string
	}{
		{
			name:                 "no APIServer config",
			apiServer:            nil,
			expectedCipherSuites: fmt.Sprintf("--tls-cipher-suites=%s", strings.Join(intermediateCiphers, ",")),
			expectedMinTLSVer:    fmt.Sprintf("--tls-min-version=%s", intermediateProfile.MinTLSVersion),
		},
		{
			name: "APIServer with TLS security profile",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers: []string{
									"ECDHE-ECDSA-AES128-GCM-SHA256",
									"ECDHE-RSA-AES128-GCM-SHA256",
								},
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					},
				},
			},
			expectedCipherSuites: "--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			expectedMinTLSVer:    "--tls-min-version=VersionTLS12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := setupTestOperatorWithTLSConfig(t, tt.apiServer)
			defer setup.cancel()

			controllers := setupTLSTestControllers(t, setup)

			// Run initial sync to populate ObservedConfig and create DaemonSet
			syncControllers(t, setup, controllers, 3)

			// Verify TLS configuration in DaemonSet
			actualDaemonSet, err := setup.kubeClient.AppsV1().DaemonSets(setup.namespace).Get(setup.ctx, setup.runtimeContext.WebhookName(), metav1.GetOptions{})
			if err != nil {
				t.Fatalf("failed to get daemonset: %v", err)
			}

			if len(actualDaemonSet.Spec.Template.Spec.Containers) == 0 {
				t.Fatalf("DaemonSet has no containers")
			}

			foundCipherSuites := false
			foundMinTLSVersion := false

			for _, arg := range actualDaemonSet.Spec.Template.Spec.Containers[0].Args {
				if strings.HasPrefix(arg, "--tls-cipher-suites=") {
					foundCipherSuites = true
					if arg != tt.expectedCipherSuites {
						t.Errorf("Expected cipher suites arg %q, got %q", tt.expectedCipherSuites, arg)
					}
				}
				if strings.HasPrefix(arg, "--tls-min-version=") {
					foundMinTLSVersion = true
					if arg != tt.expectedMinTLSVer {
						t.Errorf("Expected min TLS version arg %q, got %q", tt.expectedMinTLSVer, arg)
					}
				}
			}

			if !foundCipherSuites {
				t.Errorf("Expected to find --tls-cipher-suites arg but didn't")
			}
			if !foundMinTLSVersion {
				t.Errorf("Expected to find --tls-min-version arg but didn't")
			}
		})
	}
}

func TestDaemonSetTLSConfiguration_ObservedConfigUpdate(t *testing.T) {
	// This test verifies that the DaemonSet is updated when ObservedConfig changes
	// Start with TLS12 config, then update to TLS13, and verify the DaemonSet is updated
	initialAPIServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers: []string{
							"ECDHE-ECDSA-AES128-GCM-SHA256",
						},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
		},
	}

	setup := setupTestOperatorWithTLSConfig(t, initialAPIServer)
	defer setup.cancel()

	controllers := setupTLSTestControllers(t, setup)

	// Initial sync - populate ObservedConfig and create DaemonSet
	syncControllers(t, setup, controllers, 3)

	// Verify initial TLS configuration
	initialDS, err := setup.kubeClient.AppsV1().DaemonSets(setup.namespace).Get(setup.ctx, setup.runtimeContext.WebhookName(), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get initial daemonset: %v", err)
	}

	initialArgs := initialDS.Spec.Template.Spec.Containers[0].Args
	foundInitialCipher := false
	foundInitialMinTLS := false
	for _, arg := range initialArgs {
		if arg == "--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256" {
			foundInitialCipher = true
		}
		if arg == "--tls-min-version=VersionTLS12" {
			foundInitialMinTLS = true
		}
	}
	if !foundInitialCipher || !foundInitialMinTLS {
		t.Fatalf("Initial TLS config not found in DaemonSet. Args: %v", initialArgs)
	}

	// Update APIServer TLS config
	updatedAPIServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "cluster",
			ResourceVersion: "2",
		},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers: []string{
							"ECDHE-RSA-AES256-GCM-SHA384",
						},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
		},
	}

	_, err = setup.configClient.ConfigV1().APIServers().Update(setup.ctx, updatedAPIServer, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update APIServer: %v", err)
	}

	// Give informer time to see the update
	time.Sleep(100 * time.Millisecond)

	// Sync controllers - this should detect ObservedConfig change and update DaemonSet
	syncControllers(t, setup, controllers, 3)

	// Verify updated TLS configuration in DaemonSet
	updatedDS, err := setup.kubeClient.AppsV1().DaemonSets(setup.namespace).Get(setup.ctx, setup.runtimeContext.WebhookName(), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get updated daemonset: %v", err)
	}

	updatedArgs := updatedDS.Spec.Template.Spec.Containers[0].Args
	foundUpdatedCipher := false
	foundUpdatedMinTLS := false
	stillHasOldCipher := false
	stillHasOldMinTLS := false

	for _, arg := range updatedArgs {
		if arg == "--tls-cipher-suites=TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384" {
			foundUpdatedCipher = true
		}
		if arg == "--tls-min-version=VersionTLS13" {
			foundUpdatedMinTLS = true
		}
		if arg == "--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256" {
			stillHasOldCipher = true
		}
		if arg == "--tls-min-version=VersionTLS12" {
			stillHasOldMinTLS = true
		}
	}

	if !foundUpdatedCipher {
		t.Errorf("Expected updated cipher suites '--tls-cipher-suites=TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384' not found. Args: %v", updatedArgs)
	}
	if !foundUpdatedMinTLS {
		t.Errorf("Expected updated min TLS version '--tls-min-version=VersionTLS13' not found. Args: %v", updatedArgs)
	}
	if stillHasOldCipher {
		t.Errorf("Old cipher suites still present in DaemonSet. Args: %v", updatedArgs)
	}
	if stillHasOldMinTLS {
		t.Errorf("Old min TLS version still present in DaemonSet. Args: %v", updatedArgs)
	}
}

func TestDaemonSetTLSConfiguration_NoDuplicateArgs(t *testing.T) {
	// This test verifies that TLS args are not duplicated when ObservedConfig changes multiple times
	initialAPIServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers: []string{
							"ECDHE-ECDSA-AES128-GCM-SHA256",
						},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
		},
	}

	setup := setupTestOperatorWithTLSConfig(t, initialAPIServer)
	defer setup.cancel()

	controllers := setupTLSTestControllers(t, setup)

	// Initial sync - populate ObservedConfig and create DaemonSet with TLS12
	syncControllers(t, setup, controllers, 3)

	// Verify initial TLS12 configuration
	initialDS, err := setup.kubeClient.AppsV1().DaemonSets(setup.namespace).Get(setup.ctx, setup.runtimeContext.WebhookName(), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get initial daemonset: %v", err)
	}

	initialArgs := initialDS.Spec.Template.Spec.Containers[0].Args
	verifySingleTLSArg(t, initialArgs, "initial",
		"--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
		"--tls-min-version=VersionTLS12")

	// First update: Change to TLS13 with different cipher
	updatedAPIServer1 := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "cluster",
			ResourceVersion: "2",
		},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers: []string{
							"ECDHE-RSA-AES256-GCM-SHA384",
						},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
		},
	}

	_, err = setup.configClient.ConfigV1().APIServers().Update(setup.ctx, updatedAPIServer1, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update APIServer (first update): %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	syncControllers(t, setup, controllers, 3)

	// Verify first update TLS13 configuration
	var firstUpdateDS *appsv1.DaemonSet
	firstUpdateDS, err = setup.kubeClient.AppsV1().DaemonSets(setup.namespace).Get(setup.ctx, setup.runtimeContext.WebhookName(), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get daemonset after first update: %v", err)
	}

	firstUpdateArgs := firstUpdateDS.Spec.Template.Spec.Containers[0].Args
	verifySingleTLSArg(t, firstUpdateArgs, "first update",
		"--tls-cipher-suites=TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		"--tls-min-version=VersionTLS13")

	// Second update: Change back to TLS12 with yet another cipher
	updatedAPIServer2 := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "cluster",
			ResourceVersion: "3",
		},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers: []string{
							"ECDHE-ECDSA-AES256-GCM-SHA384",
							"ECDHE-RSA-AES128-GCM-SHA256",
						},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
		},
	}

	_, err = setup.configClient.ConfigV1().APIServers().Update(setup.ctx, updatedAPIServer2, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update APIServer (second update): %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	syncControllers(t, setup, controllers, 3)

	// Verify second update TLS12 configuration with multiple ciphers
	var secondUpdateDS *appsv1.DaemonSet
	secondUpdateDS, err = setup.kubeClient.AppsV1().DaemonSets(setup.namespace).Get(setup.ctx, setup.runtimeContext.WebhookName(), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get daemonset after second update: %v", err)
	}

	secondUpdateArgs := secondUpdateDS.Spec.Template.Spec.Containers[0].Args
	verifySingleTLSArg(t, secondUpdateArgs, "second update",
		"--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		"--tls-min-version=VersionTLS12")
}

// verifySingleTLSArg verifies that TLS args appear exactly once with expected values
func verifySingleTLSArg(t *testing.T, args []string, stage string, expectedCipherSuites, expectedMinTLSVersion string) {
	t.Helper()

	cipherSuiteCount := 0
	minTLSVersionCount := 0
	var actualCipherSuiteArg string
	var actualMinTLSVersionArg string

	for _, arg := range args {
		if strings.HasPrefix(arg, "--tls-cipher-suites=") {
			cipherSuiteCount++
			actualCipherSuiteArg = arg
		}
		if strings.HasPrefix(arg, "--tls-min-version=") {
			minTLSVersionCount++
			actualMinTLSVersionArg = arg
		}
	}

	// Verify each TLS arg appears exactly once
	if cipherSuiteCount != 1 {
		t.Errorf("[%s] Expected --tls-cipher-suites to appear exactly once, but found %d occurrences. Args: %v",
			stage, cipherSuiteCount, args)
	}
	if minTLSVersionCount != 1 {
		t.Errorf("[%s] Expected --tls-min-version to appear exactly once, but found %d occurrences. Args: %v",
			stage, minTLSVersionCount, args)
	}

	// Verify the values match expectations
	if actualCipherSuiteArg != expectedCipherSuites {
		t.Errorf("[%s] Expected cipher suites to be %q, got %q", stage, expectedCipherSuites, actualCipherSuiteArg)
	}
	if actualMinTLSVersionArg != expectedMinTLSVersion {
		t.Errorf("[%s] Expected min TLS version to be %q, got %q", stage, expectedMinTLSVersion, actualMinTLSVersionArg)
	}
}
