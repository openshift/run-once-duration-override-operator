package handlers

import (
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	"github.com/openshift/run-once-duration-override-operator/pkg/cert"
	"github.com/openshift/run-once-duration-override-operator/pkg/deploy"
	appsv1listers "github.com/openshift/run-once-duration-override-operator/pkg/generated/listers/apps/v1"
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
	PrimaryLister   appsv1listers.RunOnceDurationOverrideLister
	SecondaryLister *secondarywatch.Lister
	Asset           *asset.Asset
	Deploy          deploy.Interface
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
