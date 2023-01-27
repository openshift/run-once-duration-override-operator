package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RunOnceDurationOverrideKind = "RunOnceDurationOverride"
)

type RunOnceDurationOverrideConditionType string

const (
	InstallReadinessFailure RunOnceDurationOverrideConditionType = "InstallReadinessFailure"
	Available               RunOnceDurationOverrideConditionType = "Available"
)

const (
	InvalidParameters            = "InvalidParameters"
	ConfigurationCheckFailed     = "ConfigurationCheckFailed"
	CertNotAvailable             = "CertNotAvailable"
	CannotSetReference           = "CannotSetReference"
	CannotGenerateCert           = "CannotGenerateCert"
	InternalError                = "InternalError"
	AdmissionWebhookNotAvailable = "AdmissionWebhookNotAvailable"
	DeploymentNotReady           = "DeploymentNotReady"
)

type RunOnceDurationOverrideCondition struct {
	// Type is the type of RunOnceDurationOverride condition.
	Type RunOnceDurationOverrideConditionType `json:"type" description:"type of RunOnceDurationOverride condition"`

	// Status is the status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status" description:"status of the condition, one of True, False, Unknown"`

	// Reason is a one-word CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" description:"one-word CamelCase reason for the condition's last transition"`

	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty" description:"human-readable message indicating details about last transition"`

	// LastTransitionTime is the last time the condition transit from one status to another
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" description:"last time the condition transit from one status to another" hash:"ignore"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunOnceDurationOverride struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunOnceDurationOverrideSpec   `json:"spec,omitempty"`
	Status RunOnceDurationOverrideStatus `json:"status,omitempty"`
}

type RunOnceDurationOverrideSpec struct {
	PodResourceOverride PodResourceOverride `json:"podResourceOverride"`
}

type RunOnceDurationOverrideStatus struct {
	// Resources is a set of resources associated with the operand.
	Resources  RunOnceDurationOverrideResources    `json:"resources,omitempty"`
	Hash       RunOnceDurationOverrideResourceHash `json:"hash,omitempty"`
	Conditions []RunOnceDurationOverrideCondition  `json:"conditions,omitempty" hash:"set"`
	Version    string                              `json:"version,omitempty"`
	Image      string                              `json:"image,omitempty"`

	// CertsRotateAt is the time the serving certs will be rotated at.
	// +optional
	CertsRotateAt metav1.Time `json:"certsRotateAt,omitempty"`
}

type RunOnceDurationOverrideResourceHash struct {
	Configuration string `json:"configuration,omitempty"`
	ServingCert   string `json:"servingCert,omitempty"`
}

type RunOnceDurationOverrideResources struct {
	// ConfigurationRef points to the ConfigMap that contains the parameters for
	// RunOnceDurationOverride admission webhook.
	ConfigurationRef *corev1.ObjectReference `json:"configurationRef,omitempty"`

	// ServiceCAConfigMapRef points to the ConfigMap that is injected with a
	// data item (key "service-ca.crt") containing the PEM-encoded CA signing bundle.
	ServiceCAConfigMapRef *corev1.ObjectReference `json:"serviceCAConfigMapRef,omitempty"`

	// ServiceRef points to the Service object that exposes the RunOnceDurationOverride
	// webhook admission server to the cluster.
	// This service is annotated with `service.beta.openshift.io/serving-cert-secret-name`
	// so that service-ca operator can issue a signed serving certificate/key pair.
	ServiceRef *corev1.ObjectReference `json:"serviceRef,omitempty"`

	// ServiceCertSecretRef points to the Secret object which is created by the
	// service-ca operator and contains the signed serving certificate/key pair.
	ServiceCertSecretRef *corev1.ObjectReference `json:"serviceCertSecretRef,omitempty"`

	// DeploymentRef points to the Deployment object of the RunOnceDurationOverride
	// admission webhook server.
	DeploymentRef *corev1.ObjectReference `json:"deploymentRef,omitempty"`

	// APiServiceRef points to the APIService object related to the RunOnceDurationOverride
	// admission webhook server.
	APiServiceRef *corev1.ObjectReference `json:"apiServiceRef,omitempty"`

	// APiServiceRef points to the APIService object related to the RunOnceDurationOverride
	// admission webhook server.
	MutatingWebhookConfigurationRef *corev1.ObjectReference `json:"mutatingWebhookConfigurationRef,omitempty"`
}

// PodResourceOverride is the configuration for the admission controller which
// overrides user-provided container request/limit values.
type PodResourceOverride struct {
	metav1.TypeMeta `json:",inline"`
	Spec            PodResourceOverrideSpec `json:"spec,omitempty"`
}

// PodResourceOverrideSpec is the configuration for the RunOnceDurationOverride
// admission controller which overrides user-provided container request/limit values.
type PodResourceOverrideSpec struct {
	// ActiveDeadlineSeconds (if > 0) overrides activeDeadlineSeconds field of pod;
	// if pod's restartPolicy is set to Never or OnFailure.
	ActiveDeadlineSeconds int64 `json:"activeDeadlineSeconds"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RunOnceDurationOverrideList contains a list of IngressControllers.
type RunOnceDurationOverrideList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RunOnceDurationOverride `json:"items"`
}
