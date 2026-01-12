package targetconfigcontroller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"

	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/cert"
	fakeclientset "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/fake"
	operatorinformers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator/operatorclient"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

// TestHandlerConditions tests all conditions produced by various handlers using table-driven tests
func TestHandlerConditions(t *testing.T) {
	tests := []struct {
		name            string
		rodoo           *runoncedurationoverridev1.RunOnceDurationOverride
		setupFunc       func(*kubefake.Clientset)
		expectCondition string
		expectStatus    operatorv1.ConditionStatus
		expectReason    string
	}{
		// Availability handler conditions
		{
			name:  "AvailabilityHandler - Success",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
			},
			expectCondition: "Available",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    "",
		},
		{
			name:  "AvailabilityHandler - DeploymentNotComplete",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createNotReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
			},
			expectCondition: "Available",
			expectStatus:    operatorv1.ConditionFalse,
			expectReason:    string(runoncedurationoverridev1.InternalError),
		},
		{
			name:  "AvailabilityHandler - DeploymentNotFound",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				// Don't create any DaemonSet
			},
			expectCondition: "Available",
			expectStatus:    operatorv1.ConditionFalse,
			expectReason:    string(runoncedurationoverridev1.AdmissionWebhookNotAvailable),
		},

		// Validation handler conditions
		{
			name:  "ValidationHandler - Success",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionFalse,
			expectReason:    "",
		},
		{
			name:  "ValidationHandler - InvalidParameters",
			rodoo: createTestRodoo(-1, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.InvalidParameters),
		},

		// Configuration handler conditions - focus on errors
		{
			name:  "ConfigurationHandler - ConfigMapGetError",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
				// Only fail for the configuration configmap
				fakeKubeClient.PrependReactor("get", "configmaps", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					getAction := action.(kubetesting.GetAction)
					if getAction.GetName() == "test-operator-configuration" {
						return true, nil, fmt.Errorf("simulated get error")
					}
					return false, nil, nil
				})
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.InternalError),
		},
		{
			name:  "ConfigurationHandler - CreateConfigMapError",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
				// Only fail when creating the configuration configmap
				fakeKubeClient.PrependReactor("create", "configmaps", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					createAction := action.(kubetesting.CreateAction)
					obj := createAction.GetObject().(*corev1.ConfigMap)
					if obj.Name == "test-operator-configuration" {
						return true, nil, fmt.Errorf("simulated create error")
					}
					return false, nil, nil
				})
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.InternalError),
		},

		// Cert generation handler conditions
		{
			name:  "CertGenerationHandler - SecretGetError",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
				// Only fail when getting the server-serving-cert secret
				// Note: This reactor affects the Get inside ApplySecret, not the lister Get
				fakeKubeClient.PrependReactor("get", "secrets", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					getAction := action.(kubetesting.GetAction)
					if getAction.GetName() == "server-serving-cert-test-operator" {
						return true, nil, fmt.Errorf("simulated get secret error")
					}
					return false, nil, nil
				})
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.CannotGenerateCert),
		},
		{
			name:  "CertGenerationHandler - CreateSecretError",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
				// Only fail when creating the server-serving-cert secret
				fakeKubeClient.PrependReactor("create", "secrets", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					createAction := action.(kubetesting.CreateAction)
					obj := createAction.GetObject().(*corev1.Secret)
					if obj.Name == "server-serving-cert-test-operator" {
						return true, nil, fmt.Errorf("simulated create secret error")
					}
					return false, nil, nil
				})
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.CannotGenerateCert),
		},
		{
			name:  "CertGenerationHandler - CreateConfigMapError",
			rodoo: createTestRodoo(3600, nil),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
				// Only fail when creating the service-serving configmap
				fakeKubeClient.PrependReactor("create", "configmaps", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					createAction := action.(kubetesting.CreateAction)
					obj := createAction.GetObject().(*corev1.ConfigMap)
					if obj.Name == "test-operator-service-serving" {
						return true, nil, fmt.Errorf("simulated create configmap error")
					}
					return false, nil, nil
				})
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.CannotGenerateCert),
		},

		// Note: CertReadyHandler is too complex to test in this table-driven test framework because:
		// 1. It only runs when cert_generation doesn't set the bundle (i.e., when certs already exist and don't need rotation)
		// 2. To test validation failures, we'd need certs with invalid data that pass cert.IsPopulated()
		// 3. But if certs pass IsPopulated(), cert_generation sets ensure=false and doesn't set the bundle
		// 4. Then cert_ready loads the same certs from listers and validates them with Bundle.Validate()
		// 5. However, Validate() only checks non-empty fields, not actual PEM validity
		// 6. If we use empty data to fail Validate(), cert_generation triggers ensure=true due to !IsPopulated()
		// This circular dependency makes isolated testing impractical in the current framework.
		// The cert creation/validation paths are already well-tested by cert_generation handler tests.

		// DaemonSetHandler conditions
		{
			name:  "DaemonSetHandler - GetDeploymentError",
			rodoo: createTestRodoo(3600, withDaemonSetHandlerStatus),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
				// Cause error when getting daemonset in DaemonSetHandler
				fakeKubeClient.PrependReactor("get", "daemonsets", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					getAction := action.(kubetesting.GetAction)
					if getAction.GetName() == "test-operator" && getAction.GetNamespace() == "test-namespace" {
						return true, nil, fmt.Errorf("simulated get daemonset error")
					}
					return false, nil, nil
				})
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.InternalError),
		},

		// Note: DeploymentReadyHandler is not tested separately because it uses the same
		// deploy.IsAvailable() logic as AvailabilityHandler and returns the same condition
		// (Available=False/AdmissionWebhookNotAvailable). This would be duplicate test coverage.

		// WebhookConfigurationHandler conditions
		{
			name:  "WebhookConfigurationHandler - GetWebhookError",
			rodoo: createTestRodoo(3600, withWebhookHandlerStatus),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
				// Fail when getting webhook configuration
				fakeKubeClient.PrependReactor("get", "mutatingwebhookconfigurations", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					getAction := action.(kubetesting.GetAction)
					if getAction.GetName() == "runoncedurationoverrides.admission.runoncedurationoverride.openshift.io" {
						return true, nil, fmt.Errorf("simulated get webhook error")
					}
					return false, nil, nil
				})
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.CertNotAvailable),
		},
		{
			name:  "WebhookConfigurationHandler - CreateWebhookError",
			rodoo: createTestRodoo(3600, withWebhookHandlerStatus),
			setupFunc: func(fakeKubeClient *kubefake.Clientset) {
				createReadyDaemonSet(fakeKubeClient, "test-operator", "test-namespace")
				// Fail when creating webhook configuration
				fakeKubeClient.PrependReactor("create", "mutatingwebhookconfigurations", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, fmt.Errorf("simulated create webhook error")
				})
			},
			expectCondition: "InstallReadinessFailure",
			expectStatus:    operatorv1.ConditionTrue,
			expectReason:    string(runoncedurationoverridev1.CertNotAvailable),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeKubeClient := kubefake.NewSimpleClientset()
			fakeOperatorClient := fakeclientset.NewSimpleClientset(tt.rodoo)

			tt.setupFunc(fakeKubeClient)

			kubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
				fakeKubeClient,
				0,
				informers.WithNamespace("test-namespace"),
			)

			operatorInformerFactory := operatorinformers.NewSharedInformerFactory(
				fakeOperatorClient,
				0,
			)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create controller first - this registers informers via factory.New().WithFilteredEventsInformers
			c := NewTargetConfigController(
				&operatorclient.RunOnceDurationOverrideClient{
					Ctx:                             ctx,
					RunOnceDurationOverrideInformer: operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides(),
					OperatorClient:                  fakeOperatorClient.RunOnceDurationOverrideV1(),
				},
				fakeKubeClient,
				createTestOperandContext(),
				kubeInformerFactory,
				operatorInformerFactory,
				events.NewLoggingEventRecorder("test-operator", clock.RealClock{}),
			)

			// Start informers and wait for caches to sync
			kubeInformerFactory.Start(ctx.Done())
			operatorInformerFactory.Start(ctx.Done())
			kubeInformerFactory.WaitForCacheSync(ctx.Done())
			operatorInformerFactory.WaitForCacheSync(ctx.Done())

			// Call Sync directly instead of Run
			err := c.Sync(ctx, &fakeSyncContext{recorder: events.NewLoggingEventRecorder("test", clock.RealClock{})})
			if err != nil {
				t.Logf("Sync returned error (may be expected): %v", err)
			}

			// Get the updated status
			updated, err := fakeOperatorClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Get(ctx, "cluster", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Failed to get updated CR: %v", err)
			}

			verifyCondition(t, updated, tt.expectCondition, tt.expectStatus, tt.expectReason)
		})
	}
}

// Helper functions

func createTestRodoo(activeDeadlineSeconds int64, applyFunc func(*runoncedurationoverridev1.RunOnceDurationOverride)) *runoncedurationoverridev1.RunOnceDurationOverride {
	rodoo := &runoncedurationoverridev1.RunOnceDurationOverride{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: runoncedurationoverridev1.RunOnceDurationOverrideSpec{
			RunOnceDurationOverrideConfig: runoncedurationoverridev1.RunOnceDurationOverrideConfig{
				Spec: runoncedurationoverridev1.RunOnceDurationOverrideConfigSpec{
					ActiveDeadlineSeconds: activeDeadlineSeconds,
				},
			},
		},
	}
	if applyFunc != nil {
		applyFunc(rodoo)
	}
	return rodoo
}

func createTestOperandContext() operatorruntime.OperandContext {
	return operatorruntime.NewOperandContext("test-operator", "test-namespace", "cluster", "test-image:latest", "v1.0.0")
}

func withCertReadyStatus(rodoo *runoncedurationoverridev1.RunOnceDurationOverride) {
	rodoo.Status.Resources.ConfigurationRef = &corev1.ObjectReference{
		Name:            "test-operator-configuration",
		ResourceVersion: "1",
	}
	rodoo.Status.Resources.ServiceCertSecretRef = &corev1.ObjectReference{
		Name:            "server-serving-cert-test-operator",
		ResourceVersion: "1",
	}
	rodoo.Status.Resources.ServiceCAConfigMapRef = &corev1.ObjectReference{
		Name:            "test-operator-service-serving",
		ResourceVersion: "1",
	}
	// Set CertsRotateAt to a future time to prevent cert_generation from rotating certs
	rodoo.Status.CertsRotateAt = metav1.NewTime(time.Now().Add(24 * time.Hour))
}

func withDaemonSetHandlerStatus(rodoo *runoncedurationoverridev1.RunOnceDurationOverride) {
	withCertReadyStatus(rodoo)
	rodoo.Status.Hash.Configuration = "test-config-hash"
	rodoo.Status.Hash.ServingCert = "test-cert-hash"
}

func withWebhookHandlerStatus(rodoo *runoncedurationoverridev1.RunOnceDurationOverride) {
	withDaemonSetHandlerStatus(rodoo)
	rodoo.Status.Resources.DeploymentRef = &corev1.ObjectReference{
		Name:            "test-operator",
		Namespace:       "test-namespace",
		ResourceVersion: "1",
	}
}

func createReadyDaemonSet(fakeKubeClient *kubefake.Clientset, name, namespace string) {
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 1,
			CurrentNumberScheduled: 1,
			NumberAvailable:        1,
			UpdatedNumberScheduled: 1,
		},
	}
	fakeKubeClient.AppsV1().DaemonSets(namespace).Create(context.TODO(), daemonSet, metav1.CreateOptions{})
}

func createNotReadyDaemonSet(fakeKubeClient *kubefake.Clientset, name, namespace string) {
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"operator.openshift.io/spec-hash": "test-hash",
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 1,
			CurrentNumberScheduled: 0,
			NumberAvailable:        0,
			UpdatedNumberScheduled: 0,
		},
	}
	fakeKubeClient.AppsV1().DaemonSets(namespace).Create(context.TODO(), daemonSet, metav1.CreateOptions{})
}

func verifyCondition(t *testing.T, rodoo *runoncedurationoverridev1.RunOnceDurationOverride, conditionType string, expectedStatus operatorv1.ConditionStatus, expectedReason string) {
	t.Helper()

	if len(rodoo.Status.Conditions) == 0 {
		t.Fatalf("No conditions set at all")
	}

	cond := findCondition(rodoo.Status.Conditions, conditionType)
	if cond == nil {
		t.Fatalf("Condition %s not found. Available conditions: %+v", conditionType, rodoo.Status.Conditions)
	}

	if cond.Status != expectedStatus {
		t.Errorf("Expected %s=%s, got %s=%s", conditionType, expectedStatus, conditionType, cond.Status)
	}

	if expectedReason != "" && cond.Reason != expectedReason {
		t.Errorf("Expected reason=%s, got reason=%s", expectedReason, cond.Reason)
	}
}

// fakeSyncContext implements factory.SyncContext for testing
type fakeSyncContext struct {
	recorder events.Recorder
}

func (f *fakeSyncContext) Recorder() events.Recorder {
	return f.recorder
}

func (f *fakeSyncContext) Queue() workqueue.RateLimitingInterface {
	return nil
}

func (f *fakeSyncContext) QueueKey() string {
	return ""
}

// TestCertReadyHandler tests the CertReadyHandler in isolation
func TestCertReadyHandler(t *testing.T) {
	tests := []struct {
		name             string
		bundleAlreadySet bool
		objects          []runtime.Object
		err              error
		expectReason     string
	}{
		{
			name:             "BundleAlreadySet - Success",
			bundleAlreadySet: true,
		},
		{
			name: "ValidCertsExist - Success",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "server-serving-cert-test-operator",
						Namespace: "test-namespace",
					},
					Data: map[string][]byte{
						"tls.key": []byte("test-key-data"),
						"tls.crt": []byte("test-cert-data"),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-operator-service-serving",
						Namespace: "test-namespace",
					},
					Data: map[string]string{
						"service-ca.crt": "test-ca-data",
					},
				},
			},
		},
		{
			name: "SecretNotFound - Error",
			objects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-operator-service-serving",
						Namespace: "test-namespace",
					},
					Data: map[string]string{
						"service-ca.crt": "test-ca-data",
					},
				},
			},
			err:          errors.New("not found"),
			expectReason: string(runoncedurationoverridev1.CertNotAvailable),
		},
		{
			name: "ConfigMapNotFound - Error",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "server-serving-cert-test-operator",
						Namespace: "test-namespace",
					},
					Data: map[string][]byte{
						"tls.key": []byte("test-key-data"),
						"tls.crt": []byte("test-cert-data"),
					},
				},
			},
			err:          errors.New("not found"),
			expectReason: string(runoncedurationoverridev1.CertNotAvailable),
		},
		{
			name: "InvalidCertData_EmptyKey - Error",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "server-serving-cert-test-operator",
						Namespace: "test-namespace",
					},
					Data: map[string][]byte{
						"tls.key": []byte(""), // Empty key
						"tls.crt": []byte("test-cert-data"),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-operator-service-serving",
						Namespace: "test-namespace",
					},
					Data: map[string]string{
						"service-ca.crt": "test-ca-data",
					},
				},
			},
			err:          errors.New("must be specified"),
			expectReason: string(runoncedurationoverridev1.CertNotAvailable),
		},
		{
			name: "InvalidCertData_EmptyCert - Error",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "server-serving-cert-test-operator",
						Namespace: "test-namespace",
					},
					Data: map[string][]byte{
						"tls.key": []byte("test-key-data"),
						"tls.crt": []byte(""), // Empty cert
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-operator-service-serving",
						Namespace: "test-namespace",
					},
					Data: map[string]string{
						"service-ca.crt": "test-ca-data",
					},
				},
			},
			err:          errors.New("must be specified"),
			expectReason: string(runoncedurationoverridev1.CertNotAvailable),
		},
		{
			name: "InvalidCertData_EmptyCA - Error",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "server-serving-cert-test-operator",
						Namespace: "test-namespace",
					},
					Data: map[string][]byte{
						"tls.key": []byte("test-key-data"),
						"tls.crt": []byte("test-cert-data"),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-operator-service-serving",
						Namespace: "test-namespace",
					},
					Data: map[string]string{
						"service-ca.crt": "", // Empty CA
					},
				},
			},
			err:          errors.New("must be specified"),
			expectReason: string(runoncedurationoverridev1.CertNotAvailable),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client without pre-populated objects
			fakeKubeClient := kubefake.NewSimpleClientset()

			// Create informer factory with namespace filtering
			kubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
				fakeKubeClient,
				0,
				informers.WithNamespace("test-namespace"),
			)

			// Register informers BEFORE starting the factory
			_ = kubeInformerFactory.Core().V1().Secrets().Informer()
			_ = kubeInformerFactory.Core().V1().ConfigMaps().Informer()

			// Start informers and wait for caches to sync
			stopCh := make(chan struct{})
			defer close(stopCh)
			kubeInformerFactory.Start(stopCh)
			kubeInformerFactory.WaitForCacheSync(stopCh)

			// Add objects to tracker after informers started
			// This triggers watch events to the registered informers
			for _, obj := range tt.objects {
				err := fakeKubeClient.Tracker().Add(obj)
				if err != nil {
					t.Fatalf("Failed to add object to tracker: %v", err)
				}
			}

			// Give watch events time to propagate
			time.Sleep(100 * time.Millisecond)

			// Create the handler
			handler := NewCertReadyHandler(
				fakeKubeClient,
				kubeInformerFactory.Core().V1().Secrets().Lister(),
				kubeInformerFactory.Core().V1().ConfigMaps().Lister(),
			)

			// Create reconcile context
			reconcileContext := NewReconcileRequestContext(createTestOperandContext())

			// Set bundle if the test requires it
			if tt.bundleAlreadySet {
				reconcileContext.SetBundle(&cert.Bundle{
					Serving: cert.Serving{
						ServiceKey:  []byte("test-key"),
						ServiceCert: []byte("test-cert"),
					},
					ServingCertCA: []byte("test-ca"),
				})
			}

			// Call the handler
			result, _, err := handler.Handle(reconcileContext, createTestRodoo(3600, withCertReadyStatus))

			// Verify results
			if tt.err != nil {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				if !strings.Contains(err.Error(), tt.err.Error()) {
					t.Errorf("expected error to contain %q, but got: %v", tt.err.Error(), err)
				}
				if tt.expectReason != "" {
					reason := GetReason(err)
					if reason != tt.expectReason {
						t.Errorf("expected reason %q, but got %q", tt.expectReason, reason)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// Verify bundle is set in context
				if reconcileContext.GetBundle() == nil {
					t.Fatalf("expected bundle to be set in context")
				}
				// Verify hash is set in status
				if result.Status.Hash.ServingCert == "" {
					t.Errorf("expected hash to be set in status")
				}
			}
		})
	}
}

// findCondition finds a condition by type in the conditions slice
func findCondition(conditions []operatorv1.OperatorCondition, conditionType string) *operatorv1.OperatorCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
