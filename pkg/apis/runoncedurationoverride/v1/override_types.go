package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorsv1 "github.com/openshift/api/operator/v1"
)

const (
	RunOnceDurationOverrideKind = "RunOnceDurationOverride"
)

const (
	InstallReadinessFailure      = "InstallReadinessFailure"
	InvalidParameters            = "InvalidParameters"
	ConfigurationCheckFailed     = "ConfigurationCheckFailed"
	CertNotAvailable             = "CertNotAvailable"
	CannotSetReference           = "CannotSetReference"
	CannotGenerateCert           = "CannotGenerateCert"
	InternalError                = "InternalError"
	AdmissionWebhookNotAvailable = "AdmissionWebhookNotAvailable"
	DeploymentNotReady           = "DeploymentNotReady"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rodoo,scope=Cluster
type RunOnceDurationOverride struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +required
	Spec RunOnceDurationOverrideSpec `json:"spec,omitempty"`
	// status holds observed values from the cluster. They may not be overridden.
	// +optional
	Status RunOnceDurationOverrideStatus `json:"status,omitempty"`
}

type RunOnceDurationOverrideSpec struct {
	operatorsv1.OperatorSpec `json:",inline"`

	RunOnceDurationOverrideConfig RunOnceDurationOverrideConfig `json:"runOnceDurationOverride"`
}

// RunOnceDurationOverrideConfig is the configuration for the admission controller which
// overrides activeDeadlineSeconds for pods with restartPolicy set to Never or OnFailure.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RunOnceDurationOverrideConfig struct {
	metav1.TypeMeta `json:",inline"`
	Spec            RunOnceDurationOverrideConfigSpec `json:"spec,omitempty"`
}

type RunOnceDurationOverrideConfigSpec struct {
	// ActiveDeadlineSeconds (if > 0) overrides activeDeadlineSeconds field of pod;
	// if pod's restartPolicy is set to Never or OnFailure.
	ActiveDeadlineSeconds int64 `json:"activeDeadlineSeconds"`
}

type RunOnceDurationOverrideStatus struct {
	operatorsv1.OperatorStatus `json:",inline"`

	// Resources is a set of resources associated with the operand.
	Resources RunOnceDurationOverrideResources    `json:"resources,omitempty"`
	Hash      RunOnceDurationOverrideResourceHash `json:"hash,omitempty"`
	Image     string                              `json:"image,omitempty"`

	// CertsRotateAt is the time the serving certs will be rotated at.
	// +optional
	CertsRotateAt metav1.Time `json:"certsRotateAt,omitempty"`
}

type RunOnceDurationOverrideResourceHash struct {
	Configuration  string `json:"configuration,omitempty"`
	ServingCert    string `json:"servingCert,omitempty"`
	ObservedConfig string `json:"observedConfig,omitempty"`
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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RunOnceDurationOverrideList contains a list of IngressControllers.
type RunOnceDurationOverrideList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RunOnceDurationOverride `json:"items"`
}
