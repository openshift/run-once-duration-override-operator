package operatorclient

import (
	"context"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	applyconfiguration "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	fakeclientset "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned/fake"
	informers "github.com/openshift/run-once-duration-override-operator/pkg/generated/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// Test helper functions

func setupTestClient(ctx context.Context, t *testing.T, initialObjects ...runtime.Object) (*RunOnceDurationOverrideClient, *fakeclientset.Clientset) {
	fakeClient := fakeclientset.NewSimpleClientset(initialObjects...)
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	runOnceDurationOverrideInformer := informerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides()

	go runOnceDurationOverrideInformer.Informer().Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), runOnceDurationOverrideInformer.Informer().HasSynced) {
		t.Fatal("Failed to sync informer cache")
	}

	return &RunOnceDurationOverrideClient{
		Ctx:                             ctx,
		RunOnceDurationOverrideInformer: runOnceDurationOverrideInformer,
		OperatorClient:                  fakeClient.RunOnceDurationOverrideV1(),
	}, fakeClient
}

func verifyCustomFieldsPreserved(t *testing.T, status *runoncedurationoverridev1.RunOnceDurationOverrideStatus, expectedHash, expectedServingCert, expectedImage string) {
	t.Helper()

	if status.Hash.Configuration != expectedHash {
		t.Errorf("Configuration hash not preserved: got %s, want %s", status.Hash.Configuration, expectedHash)
	}
	if expectedServingCert != "" && status.Hash.ServingCert != expectedServingCert {
		t.Errorf("ServingCert hash not preserved: got %s, want %s", status.Hash.ServingCert, expectedServingCert)
	}
	if status.Image != expectedImage {
		t.Errorf("Image not preserved: got %s, want %s", status.Image, expectedImage)
	}
	if status.CertsRotateAt.IsZero() {
		t.Errorf("CertsRotateAt should not be zero")
	}
}

func createTestCR(name string, applyFunc func(*runoncedurationoverridev1.RunOnceDurationOverride)) *runoncedurationoverridev1.RunOnceDurationOverride {
	cr := &runoncedurationoverridev1.RunOnceDurationOverride{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: "1",
		},
		Spec: runoncedurationoverridev1.RunOnceDurationOverrideSpec{
			OperatorSpec: operatorv1.OperatorSpec{
				ManagementState: operatorv1.Managed,
			},
		},
		Status: runoncedurationoverridev1.RunOnceDurationOverrideStatus{},
	}

	if applyFunc != nil {
		applyFunc(cr)
	}

	return cr
}

func TestUpdateOperatorStatus(t *testing.T) {
	ctx := context.Background()
	now := metav1.Now()
	initialCertsRotateAt := metav1.NewTime(time.Now().Add(24 * time.Hour))
	updatedCertsRotateAt := metav1.NewTime(time.Now().Add(48 * time.Hour))

	tests := []struct {
		name           string
		initialStatus  runoncedurationoverridev1.RunOnceDurationOverrideStatus
		updatedStatus  runoncedurationoverridev1.RunOnceDurationOverrideStatus
		validateStatus func(t *testing.T, status *runoncedurationoverridev1.RunOnceDurationOverrideStatus)
	}{
		{
			name: "update all custom fields",
			initialStatus: runoncedurationoverridev1.RunOnceDurationOverrideStatus{
				OperatorStatus: operatorv1.OperatorStatus{
					Version: "1.0.0",
				},
				Hash: runoncedurationoverridev1.RunOnceDurationOverrideResourceHash{
					Configuration: "old-config-hash",
					ServingCert:   "old-cert-hash",
				},
				Resources: runoncedurationoverridev1.RunOnceDurationOverrideResources{
					ConfigurationRef: &corev1.ObjectReference{
						Kind:      "ConfigMap",
						Namespace: "test-ns",
						Name:      "old-config",
					},
					DeploymentRef: &corev1.ObjectReference{
						Kind:      "DaemonSet",
						Namespace: "test-ns",
						Name:      "old-deployment",
					},
				},
				Image:         "old-image:v1",
				CertsRotateAt: initialCertsRotateAt,
			},
			updatedStatus: runoncedurationoverridev1.RunOnceDurationOverrideStatus{
				OperatorStatus: operatorv1.OperatorStatus{
					Version: "2.0.0",
					Conditions: []operatorv1.OperatorCondition{
						{
							Type:               "Available",
							Status:             operatorv1.ConditionTrue,
							LastTransitionTime: now,
						},
					},
				},
				Hash: runoncedurationoverridev1.RunOnceDurationOverrideResourceHash{
					Configuration: "new-config-hash",
					ServingCert:   "new-cert-hash",
				},
				Resources: runoncedurationoverridev1.RunOnceDurationOverrideResources{
					ConfigurationRef: &corev1.ObjectReference{
						Kind:      "ConfigMap",
						Namespace: "test-ns",
						Name:      "new-config",
					},
					DeploymentRef: &corev1.ObjectReference{
						Kind:      "DaemonSet",
						Namespace: "test-ns",
						Name:      "new-deployment",
					},
					ServiceCertSecretRef: &corev1.ObjectReference{
						Kind:      "Secret",
						Namespace: "test-ns",
						Name:      "new-cert-secret",
					},
					ServiceCAConfigMapRef: &corev1.ObjectReference{
						Kind:      "ConfigMap",
						Namespace: "test-ns",
						Name:      "new-ca-configmap",
					},
				},
				Image:         "new-image:v2",
				CertsRotateAt: updatedCertsRotateAt,
			},
			validateStatus: func(t *testing.T, status *runoncedurationoverridev1.RunOnceDurationOverrideStatus) {
				if status.Version != "2.0.0" {
					t.Errorf("Expected Version 2.0.0, got %s", status.Version)
				}
				if status.Hash.Configuration != "new-config-hash" {
					t.Errorf("Expected Hash.Configuration new-config-hash, got %s", status.Hash.Configuration)
				}
				if status.Hash.ServingCert != "new-cert-hash" {
					t.Errorf("Expected Hash.ServingCert new-cert-hash, got %s", status.Hash.ServingCert)
				}
				if status.Image != "new-image:v2" {
					t.Errorf("Expected Image new-image:v2, got %s", status.Image)
				}
				if status.Resources.ConfigurationRef == nil || status.Resources.ConfigurationRef.Name != "new-config" {
					t.Errorf("Expected ConfigurationRef.Name new-config, got %v", status.Resources.ConfigurationRef)
				}
				if status.Resources.DeploymentRef == nil || status.Resources.DeploymentRef.Name != "new-deployment" {
					t.Errorf("Expected DeploymentRef.Name new-deployment, got %v", status.Resources.DeploymentRef)
				}
				if status.Resources.ServiceCertSecretRef == nil || status.Resources.ServiceCertSecretRef.Name != "new-cert-secret" {
					t.Errorf("Expected ServiceCertSecretRef.Name new-cert-secret, got %v", status.Resources.ServiceCertSecretRef)
				}
				if status.Resources.ServiceCAConfigMapRef == nil || status.Resources.ServiceCAConfigMapRef.Name != "new-ca-configmap" {
					t.Errorf("Expected ServiceCAConfigMapRef.Name new-ca-configmap, got %v", status.Resources.ServiceCAConfigMapRef)
				}
				if !status.CertsRotateAt.Equal(&updatedCertsRotateAt) {
					t.Errorf("Expected CertsRotateAt to be updated to %v, got %v", updatedCertsRotateAt, status.CertsRotateAt)
				}
			},
		},
		{
			name: "update only hash fields",
			initialStatus: runoncedurationoverridev1.RunOnceDurationOverrideStatus{
				OperatorStatus: operatorv1.OperatorStatus{
					Version: "1.0.0",
				},
				Hash: runoncedurationoverridev1.RunOnceDurationOverrideResourceHash{
					Configuration: "hash1",
					ServingCert:   "hash2",
				},
				Image:         "image:v1",
				CertsRotateAt: initialCertsRotateAt,
			},
			updatedStatus: runoncedurationoverridev1.RunOnceDurationOverrideStatus{
				OperatorStatus: operatorv1.OperatorStatus{
					Version: "1.0.0",
				},
				Hash: runoncedurationoverridev1.RunOnceDurationOverrideResourceHash{
					Configuration: "hash1-updated",
					ServingCert:   "hash2-updated",
				},
				Image:         "image:v1",
				CertsRotateAt: initialCertsRotateAt,
			},
			validateStatus: func(t *testing.T, status *runoncedurationoverridev1.RunOnceDurationOverrideStatus) {
				if status.Hash.Configuration != "hash1-updated" {
					t.Errorf("Expected Hash.Configuration hash1-updated, got %s", status.Hash.Configuration)
				}
				if status.Hash.ServingCert != "hash2-updated" {
					t.Errorf("Expected Hash.ServingCert hash2-updated, got %s", status.Hash.ServingCert)
				}
				if status.Image != "image:v1" {
					t.Errorf("Expected Image to remain image:v1, got %s", status.Image)
				}
			},
		},
		{
			name: "clear resource references",
			initialStatus: runoncedurationoverridev1.RunOnceDurationOverrideStatus{
				OperatorStatus: operatorv1.OperatorStatus{
					Version: "1.0.0",
				},
				Resources: runoncedurationoverridev1.RunOnceDurationOverrideResources{
					ConfigurationRef: &corev1.ObjectReference{
						Kind:      "ConfigMap",
						Namespace: "test-ns",
						Name:      "config",
					},
					ServiceCertSecretRef: &corev1.ObjectReference{
						Kind:      "Secret",
						Namespace: "test-ns",
						Name:      "secret",
					},
				},
			},
			updatedStatus: runoncedurationoverridev1.RunOnceDurationOverrideStatus{
				OperatorStatus: operatorv1.OperatorStatus{
					Version: "1.0.0",
				},
				Resources: runoncedurationoverridev1.RunOnceDurationOverrideResources{
					// All fields cleared
				},
			},
			validateStatus: func(t *testing.T, status *runoncedurationoverridev1.RunOnceDurationOverrideStatus) {
				if status.Resources.ConfigurationRef != nil {
					t.Errorf("Expected ConfigurationRef to be nil, got %v", status.Resources.ConfigurationRef)
				}
				if status.Resources.ServiceCertSecretRef != nil {
					t.Errorf("Expected ServiceCertSecretRef to be nil, got %v", status.Resources.ServiceCertSecretRef)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingCR := createTestCR(OperatorConfigName, func(cr *runoncedurationoverridev1.RunOnceDurationOverride) {
				cr.Status = tt.initialStatus
			})

			client, fakeClient := setupTestClient(ctx, t, existingCR)

			returnedStatus, err := client.UpdateOperatorStatus(ctx, "1", &tt.updatedStatus)
			if err != nil {
				t.Fatalf("UpdateOperatorStatus failed: %v", err)
			}

			// Validate returned status
			tt.validateStatus(t, returnedStatus)

			// Fetch and validate persisted status
			updatedCR, err := fakeClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Get(ctx, OperatorConfigName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Failed to get updated CR: %v", err)
			}

			tt.validateStatus(t, &updatedCR.Status)
		})
	}
}

func TestApplyOperatorStatus(t *testing.T) {
	ctx := context.Background()
	now := metav1.Now()

	existingCR := createTestCR(OperatorConfigName, func(cr *runoncedurationoverridev1.RunOnceDurationOverride) {
		cr.Status.Version = "1.0.0"
		cr.Status.Hash.Configuration = "config-hash-789"
		cr.Status.Image = "test-image:v2"
		cr.Status.CertsRotateAt = metav1.NewTime(time.Now().Add(24 * time.Hour))
	})

	client, fakeClient := setupTestClient(ctx, t, existingCR)

	applyConfig := applyconfiguration.OperatorStatus().
		WithConditions(
			applyconfiguration.OperatorCondition().
				WithType("Available").
				WithStatus(operatorv1.ConditionTrue).
				WithLastTransitionTime(now),
		).
		WithVersion("1.2.0")

	err := client.ApplyOperatorStatus(ctx, "test-manager", applyConfig)
	if err != nil {
		t.Fatalf("ApplyOperatorStatus failed: %v", err)
	}

	updatedCR, err := fakeClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Get(ctx, OperatorConfigName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}

	if updatedCR.Status.Version != "1.2.0" {
		t.Errorf("Expected version 1.2.0, got %s", updatedCR.Status.Version)
	}
	verifyCustomFieldsPreserved(t, &updatedCR.Status, "config-hash-789", "", "test-image:v2")
}

func TestUpdateOperatorSpec(t *testing.T) {
	ctx := context.Background()

	existingCR := createTestCR(OperatorConfigName, func(cr *runoncedurationoverridev1.RunOnceDurationOverride) {
		cr.Spec.LogLevel = operatorv1.Normal
	})

	client, fakeClient := setupTestClient(ctx, t, existingCR)

	updatedSpec := &operatorv1.OperatorSpec{
		ManagementState: operatorv1.Managed,
		LogLevel:        operatorv1.Debug,
	}

	returnedSpec, newVersion, err := client.UpdateOperatorSpec(ctx, "1", updatedSpec)
	if err != nil {
		t.Fatalf("UpdateOperatorSpec failed: %v", err)
	}

	if returnedSpec.LogLevel != operatorv1.Debug {
		t.Errorf("Expected LogLevel Debug, got %s", returnedSpec.LogLevel)
	}
	if newVersion == "" {
		t.Errorf("ResourceVersion should be set")
	}

	updatedCR, err := fakeClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Get(ctx, OperatorConfigName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}

	if updatedCR.Spec.LogLevel != operatorv1.Debug {
		t.Errorf("Spec not updated: expected Debug, got %s", updatedCR.Spec.LogLevel)
	}
}

func TestApplyOperatorSpec(t *testing.T) {
	ctx := context.Background()

	existingCR := createTestCR(OperatorConfigName, nil)

	client, fakeClient := setupTestClient(ctx, t, existingCR)

	applyConfig := applyconfiguration.OperatorSpec().
		WithManagementState(operatorv1.Managed).
		WithLogLevel(operatorv1.TraceAll)

	err := client.ApplyOperatorSpec(ctx, "test-manager", applyConfig)
	if err != nil {
		t.Fatalf("ApplyOperatorSpec failed: %v", err)
	}

	updatedCR, err := fakeClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Get(ctx, OperatorConfigName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}

	if updatedCR.Spec.LogLevel != operatorv1.TraceAll {
		t.Errorf("Expected LogLevel TraceAll, got %s", updatedCR.Spec.LogLevel)
	}
}

func TestGetOperatorState(t *testing.T) {
	ctx := context.Background()

	existingCR := createTestCR(OperatorConfigName, func(cr *runoncedurationoverridev1.RunOnceDurationOverride) {
		cr.Status.Version = "1.0.0"
		cr.ResourceVersion = "5"
	})

	client, _ := setupTestClient(ctx, t, existingCR)

	spec, status, resourceVersion, err := client.GetOperatorState()
	if err != nil {
		t.Fatalf("GetOperatorState failed: %v", err)
	}

	if spec.ManagementState != operatorv1.Managed {
		t.Errorf("Expected ManagementState Managed, got %s", spec.ManagementState)
	}
	if status.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", status.Version)
	}
	if resourceVersion != "5" {
		t.Errorf("Expected resourceVersion 5, got %s", resourceVersion)
	}
}

func TestUpdateStatus(t *testing.T) {
	ctx := context.Background()
	now := metav1.Now()
	certsRotateAt := metav1.NewTime(time.Now().Add(24 * time.Hour))

	existingCR := createTestCR(OperatorConfigName, func(cr *runoncedurationoverridev1.RunOnceDurationOverride) {
		cr.Status.Conditions = []operatorv1.OperatorCondition{
			{
				Type:               "Available",
				Status:             operatorv1.ConditionFalse,
				LastTransitionTime: now,
			},
		}
		cr.Status.Version = "1.0.0"
		cr.Status.Hash = runoncedurationoverridev1.RunOnceDurationOverrideResourceHash{
			Configuration: "old-config-hash",
			ServingCert:   "old-cert-hash",
		}
		cr.Status.Resources.ConfigurationRef = &corev1.ObjectReference{
			Kind:      "ConfigMap",
			Namespace: "test-ns",
			Name:      "old-config",
		}
		cr.Status.Image = "old-image:v1"
		cr.Status.CertsRotateAt = certsRotateAt
	})

	client, fakeClient := setupTestClient(ctx, t, existingCR)

	// Use the new UpdateStatus function to update both standard and custom fields
	_, _, err := UpdateStatus(ctx, client, func(status *runoncedurationoverridev1.RunOnceDurationOverrideStatus) error {
		// Update standard OperatorStatus fields
		v1helpers.SetOperatorCondition(&status.OperatorStatus.Conditions, operatorv1.OperatorCondition{
			Type:   "Available",
			Status: operatorv1.ConditionTrue,
			Reason: "AsExpected",
		})
		status.OperatorStatus.Version = "1.1.0"

		// Update custom fields
		status.Hash.Configuration = "new-config-hash"
		status.Hash.ServingCert = "new-cert-hash"
		status.Resources.ConfigurationRef = &corev1.ObjectReference{
			Kind:      "ConfigMap",
			Namespace: "test-ns",
			Name:      "new-config",
		}
		status.Image = "new-image:v2"

		return nil
	})
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify the update persisted correctly
	updatedCR, err := fakeClient.RunOnceDurationOverrideV1().RunOnceDurationOverrides().Get(ctx, OperatorConfigName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}

	// Verify standard OperatorStatus fields
	if updatedCR.Status.Version != "1.1.0" {
		t.Errorf("Expected version 1.1.0, got %s", updatedCR.Status.Version)
	}
	if len(updatedCR.Status.Conditions) == 0 {
		t.Fatalf("Expected conditions to be set")
	}
	var availableCondition *operatorv1.OperatorCondition
	for i := range updatedCR.Status.Conditions {
		if updatedCR.Status.Conditions[i].Type == "Available" {
			availableCondition = &updatedCR.Status.Conditions[i]
			break
		}
	}
	if availableCondition == nil {
		t.Fatalf("Available condition not found")
	}
	if availableCondition.Status != operatorv1.ConditionTrue {
		t.Errorf("Expected Available condition to be True, got %s", availableCondition.Status)
	}

	// Verify custom fields were updated
	verifyCustomFieldsPreserved(t, &updatedCR.Status, "new-config-hash", "new-cert-hash", "new-image:v2")

	if updatedCR.Status.Resources.ConfigurationRef == nil || updatedCR.Status.Resources.ConfigurationRef.Name != "new-config" {
		t.Errorf("ConfigurationRef not updated correctly")
	}
}
