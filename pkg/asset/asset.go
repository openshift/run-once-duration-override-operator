package asset

import (
	"fmt"

	appsv1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
	"github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

func New(context runtime.OperandContext) *Asset {
	values := &Values{
		Name:                            context.WebhookName(),
		Namespace:                       context.WebhookNamespace(),
		ServiceAccountName:              context.WebhookName(),
		OperandImage:                    context.OperandImage(),
		OperandVersion:                  context.OperandVersion(),
		AdmissionAPIGroup:               "admission.runoncedurationoverride.openshift.io",
		AdmissionAPIVersion:             "v1",
		AdmissionAPIResource:            "runoncedurationoverrides",
		OwnerLabelKey:                   "operator.apps.openshift.io/runoncedurationoverride",
		OwnerLabelValue:                 "true",
		SelectorLabelKey:                "runoncedurationoverride",
		SelectorLabelValue:              "true",
		ConfigurationKey:                "configuration.yaml",
		ConfigurationHashAnnotationKey:  fmt.Sprintf("%s.%s/configuration.hash", context.WebhookName(), appsv1.GroupName),
		ServingCertHashAnnotationKey:    fmt.Sprintf("%s.%s/servingcert.hash", context.WebhookName(), appsv1.GroupName),
		ObservedConfigHashAnnotationKey: fmt.Sprintf("%s.%s/observedconfig.hash", context.WebhookName(), appsv1.GroupName),
		OwnerAnnotationKey:              fmt.Sprintf("%s.%s/owner", context.WebhookName(), appsv1.GroupName),
	}

	return &Asset{
		values: values,
	}
}

type Asset struct {
	values *Values
}

func (a *Asset) Values() *Values {
	return a.values
}

type Values struct {
	Name                 string
	Namespace            string
	ServiceAccountName   string
	OperandImage         string
	OperandVersion       string
	AdmissionAPIGroup    string
	AdmissionAPIVersion  string
	AdmissionAPIResource string
	OwnerLabelKey        string
	OwnerLabelValue      string
	SelectorLabelKey     string
	SelectorLabelValue   string
	ConfigurationKey     string

	ConfigurationHashAnnotationKey  string
	ServingCertHashAnnotationKey    string
	ObservedConfigHashAnnotationKey string
	OwnerAnnotationKey              string
}
