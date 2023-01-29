package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/apps/v1"
	"github.com/openshift/run-once-duration-override-operator/test/helper"
)

func TestRunOnceDurationWithOptIn(t *testing.T) {
	tests := []struct {
		name         string
		request      *corev1.PodSpec
		resourceWant *int64
	}{
		{
			name: "WithMultipleContainers",
			request: &corev1.PodSpec{
				ActiveDeadlineSeconds: pointer.Int64Ptr(2400),
				Containers: []corev1.Container{
					{
						Name:  "db",
						Image: "openshift/hello-openshift",
						Ports: []corev1.ContainerPort{
							{
								Name:          "db",
								ContainerPort: 60000,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: pointer.BoolPtr(true),
							SeccompProfile: &corev1.SeccompProfile{
								Type: "RuntimeDefault",
							},
						},
					},
					{
						Name:  "app",
						Image: "openshift/hello-openshift",
						Ports: []corev1.ContainerPort{
							{
								Name:          "app",
								ContainerPort: 60100,
							},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
								corev1.ResourceCPU:    resource.MustParse("500m"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: pointer.BoolPtr(true),
							SeccompProfile: &corev1.SeccompProfile{
								Type: "RuntimeDefault",
							},
						},
					},
				},
			},
			resourceWant: pointer.Int64Ptr(2400),
		},
		{
			name: "WithInitContainer",
			request: &corev1.PodSpec{
				ActiveDeadlineSeconds: pointer.Int64Ptr(1200),
				InitContainers: []corev1.Container{
					{
						Name:  "init",
						Image: "busybox:latest",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1024Mi"),
								corev1.ResourceCPU:    resource.MustParse("1000m"),
							},
						},
						Command: []string{
							"sh",
							"-c",
							"echo The app is running! && sleep 1",
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: pointer.BoolPtr(true),
							SeccompProfile: &corev1.SeccompProfile{
								Type: "RuntimeDefault",
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "openshift/hello-openshift",
						Ports: []corev1.ContainerPort{
							{
								Name:          "app",
								ContainerPort: 60100,
							},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
								corev1.ResourceCPU:    resource.MustParse("500m")},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: pointer.BoolPtr(true),
							SeccompProfile: &corev1.SeccompProfile{
								Type: "RuntimeDefault",
							},
						},
					},
				},
			},
			resourceWant: pointer.Int64Ptr(1200),
		},
		{
			name: "WithLimitRangeWithDefaultLimitForCPUAndMemory",
			request: &corev1.PodSpec{
				ActiveDeadlineSeconds: pointer.Int64Ptr(800),
				RestartPolicy:         corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "openshift/hello-openshift",
						Ports: []corev1.ContainerPort{
							{
								Name:          "app",
								ContainerPort: 60100,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: pointer.BoolPtr(true),
							SeccompProfile: &corev1.SeccompProfile{
								Type: "RuntimeDefault",
							},
						},
					},
				},
			},
			resourceWant: pointer.Int64Ptr(800),
		},
		{
			name: "WithLimitRangeWithDefaultLimitForCPUAndMemory",
			request: &corev1.PodSpec{
				ActiveDeadlineSeconds: pointer.Int64Ptr(800),
				RestartPolicy:         corev1.RestartPolicyAlways,
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "openshift/hello-openshift",
						Ports: []corev1.ContainerPort{
							{
								Name:          "app",
								ContainerPort: 60100,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: pointer.BoolPtr(true),
							SeccompProfile: &corev1.SeccompProfile{
								Type: "RuntimeDefault",
							},
						},
					},
				},
			},
			resourceWant: nil,
		},
		{
			name: "WithLimitRangeWithDefaultLimitForCPUAndMemory",
			request: &corev1.PodSpec{
				ActiveDeadlineSeconds: pointer.Int64Ptr(800),
				RestartPolicy:         corev1.RestartPolicyOnFailure,
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "openshift/hello-openshift",
						Ports: []corev1.ContainerPort{
							{
								Name:          "app",
								ContainerPort: 60100,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: pointer.BoolPtr(true),
							SeccompProfile: &corev1.SeccompProfile{
								Type: "RuntimeDefault",
							},
						},
					},
				},
			},
			resourceWant: pointer.Int64Ptr(800),
		},
		{
			name: "WithLimitRangeWithMaximumForCPU",
			request: &corev1.PodSpec{
				ActiveDeadlineSeconds: pointer.Int64Ptr(1100),
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "openshift/hello-openshift",
						Ports: []corev1.ContainerPort{
							{
								Name:          "app",
								ContainerPort: 60100,
							},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1024Mi"),
								corev1.ResourceCPU:    resource.MustParse("1000m")},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: pointer.BoolPtr(true),
							SeccompProfile: &corev1.SeccompProfile{
								Type: "RuntimeDefault",
							},
						},
					},
				},
			},
			resourceWant: pointer.Int64Ptr(1100),
		},
	}

	client := helper.NewClient(t, options.config)

	f := &helper.PreCondition{Client: client.Kubernetes}
	f.MustHaveAdmissionRegistrationV1(t)

	// ensure we have the webhook up and running with the desired config
	configuration := appsv1.PodResourceOverrideSpec{
		ActiveDeadlineSeconds: 200,
	}
	override := appsv1.PodResourceOverride{
		Spec: configuration,
	}

	t.Logf("setting webhook configuration - %s", configuration.String())
	current, changed := helper.EnsureAdmissionWebhook(t, client.Operator, "cluster", override)
	defer helper.RemoveAdmissionWebhook(t, client.Operator, current.GetName())

	t.Log("waiting for webhook configuration to take effect")
	current = helper.Wait(t, client.Operator, "cluster", helper.GetAvailableConditionFunc(current, changed))

	f.MustHaveRunOnceDurationOverrideConfiguration(t)
	t.Log("webhook configuration has been set successfully")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			func() {
				// ensure a namespace that is properly labeled
				ns, disposer := helper.NewNamespace(t, client.Kubernetes, "croe2e", true)
				defer disposer.Dispose()
				namespace := ns.GetName()

				podGot, disposer := helper.NewPod(t, client.Kubernetes, namespace, *test.request)
				defer disposer.Dispose()

				require.Equal(t, podGot.Spec.ActiveDeadlineSeconds, test.resourceWant)
			}()
		})
	}
}

func TestRunOnceDurationOverrideWithConfigurationChange(t *testing.T) {
	client := helper.NewClient(t, options.config)

	f := &helper.PreCondition{Client: client.Kubernetes}
	f.MustHaveAdmissionRegistrationV1(t)

	before := appsv1.PodResourceOverrideSpec{
		ActiveDeadlineSeconds: 1200,
	}
	override := appsv1.PodResourceOverride{
		Spec: before,
	}

	t.Logf("initial configuration - %s", before.String())

	current, changed := helper.EnsureAdmissionWebhook(t, client.Operator, "cluster", override)
	defer helper.RemoveAdmissionWebhook(t, client.Operator, current.GetName())

	current = helper.Wait(t, client.Operator, "cluster", helper.GetAvailableConditionFunc(current, changed))
	require.Equal(t, override.Spec.Hash(), current.Status.Hash.Configuration)

	after := appsv1.PodResourceOverrideSpec{
		ActiveDeadlineSeconds: 1200,
	}
	override = appsv1.PodResourceOverride{
		Spec: after,
	}

	t.Logf("final configuration - %s", after.String())

	current, changed = helper.EnsureAdmissionWebhook(t, client.Operator, "cluster", override)
	current = helper.Wait(t, client.Operator, "cluster", helper.GetAvailableConditionFunc(current, changed))
	require.Equal(t, override.Spec.Hash(), current.Status.Hash.Configuration)

	// create a new Pod, we expect the Pod resources to be overridden based of the new configuration.
	ns, disposer := helper.NewNamespace(t, client.Kubernetes, "croe2e", true)
	defer disposer.Dispose()

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{
			{
				Name:  "app",
				Image: "openshift/hello-openshift",
				Ports: []corev1.ContainerPort{
					{
						Name:          "app",
						ContainerPort: 60100,
					},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1024Mi"),
						corev1.ResourceCPU:    resource.MustParse("1000m")},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: pointer.BoolPtr(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
					RunAsNonRoot: pointer.BoolPtr(true),
					SeccompProfile: &corev1.SeccompProfile{
						Type: "RuntimeDefault",
					},
				},
			},
		},
	}

	podGot, disposer := helper.NewPod(t, client.Kubernetes, ns.GetName(), podSpec)
	defer disposer.Dispose()

	require.Equal(t, t, podGot.Spec.ActiveDeadlineSeconds, pointer.Int64Ptr(1100))
}

func TestRunOnceDurationOverrideWithCertRotation(t *testing.T) {
	client := helper.NewClient(t, options.config)

	f := &helper.PreCondition{Client: client.Kubernetes}
	f.MustHaveAdmissionRegistrationV1(t)

	configuration := appsv1.PodResourceOverrideSpec{
		ActiveDeadlineSeconds: 900,
	}
	override := appsv1.PodResourceOverride{
		Spec: configuration,
	}

	current, changed := helper.EnsureAdmissionWebhook(t, client.Operator, "cluster", override)
	defer helper.RemoveAdmissionWebhook(t, client.Operator, current.GetName())

	current = helper.Wait(t, client.Operator, "cluster", helper.GetAvailableConditionFunc(current, changed))

	originalCertHash := current.Status.Hash.ServingCert
	require.NotEmpty(t, originalCertHash)

	current.Status.CertsRotateAt = metav1.NewTime(time.Now())
	current, err := client.Operator.AppsV1().RunOnceDurationOverrides().UpdateStatus(context.TODO(), current, metav1.UpdateOptions{})
	require.NoError(t, err)

	current = helper.Wait(t, client.Operator, "cluster", helper.GetAvailableConditionFunc(current, true))
	newCertHash := current.Status.Hash.ServingCert
	require.NotEmpty(t, originalCertHash)
	require.NotEqual(t, originalCertHash, newCertHash)

	// make sure everything works after cert is regenerated
	ns, disposer := helper.NewNamespace(t, client.Kubernetes, "croe2e", true)
	defer disposer.Dispose()

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyOnFailure,
		Containers: []corev1.Container{
			{
				Name:  "app",
				Image: "openshift/hello-openshift",
				Ports: []corev1.ContainerPort{
					{
						Name:          "app",
						ContainerPort: 60100,
					},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1024Mi"),
						corev1.ResourceCPU:    resource.MustParse("1000m")},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: pointer.BoolPtr(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
					RunAsNonRoot: pointer.BoolPtr(true),
					SeccompProfile: &corev1.SeccompProfile{
						Type: "RuntimeDefault",
					},
				},
			},
		},
	}

	podGot, disposer := helper.NewPod(t, client.Kubernetes, ns.GetName(), podSpec)
	defer disposer.Dispose()

	require.Equal(t, podGot.Spec.ActiveDeadlineSeconds, pointer.Int64Ptr(900))
}

func TestRunOnceDurationOverrideWithNoOptIn(t *testing.T) {
	client := helper.NewClient(t, options.config)

	f := &helper.PreCondition{Client: client.Kubernetes}
	f.MustHaveAdmissionRegistrationV1(t)

	configuration := appsv1.PodResourceOverrideSpec{
		ActiveDeadlineSeconds: 750,
	}
	override := appsv1.PodResourceOverride{
		Spec: configuration,
	}

	current, changed := helper.EnsureAdmissionWebhook(t, client.Operator, "cluster", override)
	defer helper.RemoveAdmissionWebhook(t, client.Operator, current.GetName())

	current = helper.Wait(t, client.Operator, "cluster", helper.GetAvailableConditionFunc(current, changed))

	// make sure everything works after cert is regenerated
	ns, disposer := helper.NewNamespace(t, client.Kubernetes, "croe2e", false)
	defer disposer.Dispose()

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyOnFailure,
		Containers: []corev1.Container{
			{
				Name:  "app",
				Image: "openshift/hello-openshift",
				Ports: []corev1.ContainerPort{
					{
						Name:          "app",
						ContainerPort: 60100,
					},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1024Mi"),
						corev1.ResourceCPU:    resource.MustParse("1000m")},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: pointer.BoolPtr(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
					RunAsNonRoot: pointer.BoolPtr(true),
					SeccompProfile: &corev1.SeccompProfile{
						Type: "RuntimeDefault",
					},
				},
			},
		},
	}

	podGot, disposer := helper.NewPod(t, client.Kubernetes, ns.GetName(), podSpec)
	defer disposer.Dispose()

	require.Equal(t, podGot.Spec.ActiveDeadlineSeconds, pointer.Int64Ptr(750))
}

func TestRunOnceDurationOverrideWithNoOptInNoChange(t *testing.T) {
	client := helper.NewClient(t, options.config)

	f := &helper.PreCondition{Client: client.Kubernetes}
	f.MustHaveAdmissionRegistrationV1(t)

	configuration := appsv1.PodResourceOverrideSpec{
		ActiveDeadlineSeconds: 750,
	}
	override := appsv1.PodResourceOverride{
		Spec: configuration,
	}

	current, changed := helper.EnsureAdmissionWebhook(t, client.Operator, "cluster", override)
	defer helper.RemoveAdmissionWebhook(t, client.Operator, current.GetName())

	current = helper.Wait(t, client.Operator, "cluster", helper.GetAvailableConditionFunc(current, changed))

	// make sure everything works after cert is regenerated
	ns, disposer := helper.NewNamespace(t, client.Kubernetes, "croe2e", false)
	defer disposer.Dispose()

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyAlways,
		Containers: []corev1.Container{
			{
				Name:  "app",
				Image: "openshift/hello-openshift",
				Ports: []corev1.ContainerPort{
					{
						Name:          "app",
						ContainerPort: 60100,
					},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1024Mi"),
						corev1.ResourceCPU:    resource.MustParse("1000m")},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: pointer.BoolPtr(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
					RunAsNonRoot: pointer.BoolPtr(true),
					SeccompProfile: &corev1.SeccompProfile{
						Type: "RuntimeDefault",
					},
				},
			},
		},
	}

	podGot, disposer := helper.NewPod(t, client.Kubernetes, ns.GetName(), podSpec)
	defer disposer.Dispose()

	require.Equal(t, podGot.Spec.ActiveDeadlineSeconds, nil)
}
