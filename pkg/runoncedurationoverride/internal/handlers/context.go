package handlers

import (
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/cert"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	runoncedurationoverridev1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/runoncedurationoverride/v1"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
	"github.com/openshift/run-once-duration-override-operator/pkg/secondarywatch"
)

func NewReconcileRequestContext(oc operatorruntime.OperandContext) *ReconcileRequestContext {
	return &ReconcileRequestContext{
		OperandContext: oc,
	}
}

type Options struct {
	OperandContext  operatorruntime.OperandContext
	Client          *operatorruntime.Client
	PrimaryLister   runoncedurationoverridev1listers.RunOnceDurationOverrideLister
	SecondaryLister *secondarywatch.Lister
	Asset           *asset.Asset
	Deploy          deploy.Interface
	Recorder        events.Recorder
}

type ReconcileRequestContext struct {
	operatorruntime.OperandContext
	bundle *cert.Bundle
}

func (r *ReconcileRequestContext) SetBundle(bundle *cert.Bundle) {
	r.bundle = bundle
}

func (r *ReconcileRequestContext) GetBundle() *cert.Bundle {
	return r.bundle
}

func (r *ReconcileRequestContext) ControllerSetter() operatorruntime.SetControllerFunc {
	return operatorruntime.SetController
}
