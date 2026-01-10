package deploy

import (
	gocontext "context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	listersappsv1 "k8s.io/client-go/listers/apps/v1"

	operatorsv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/run-once-duration-override-operator/pkg/asset"
	operatorruntime "github.com/openshift/run-once-duration-override-operator/pkg/runtime"
)

func NewDaemonSetInstall(lister listersappsv1.DaemonSetLister, oc operatorruntime.OperandContext, asset *asset.Asset, client kubernetes.Interface, recorder events.Recorder) Interface {
	return &daemonset{
		lister:   lister,
		context:  oc,
		asset:    asset,
		client:   client,
		recorder: recorder,
	}
}

type daemonset struct {
	lister   listersappsv1.DaemonSetLister
	context  operatorruntime.OperandContext
	asset    *asset.Asset
	client   kubernetes.Interface
	recorder events.Recorder
}

func (d *daemonset) Name() string {
	return d.asset.DaemonSet().Name()
}

func (d *daemonset) IsAvailable() (available bool, err error) {
	name := d.asset.DaemonSet().Name()
	current, err := d.lister.DaemonSets(d.context.WebhookNamespace()).Get(name)
	if err != nil {
		return
	}

	available, err = GetDaemonSetStatus(current)
	return
}

func (d *daemonset) Get() (object runtime.Object, accessor metav1.Object, err error) {
	name := d.asset.DaemonSet().Name()
	object, err = d.lister.DaemonSets(d.context.WebhookNamespace()).Get(name)
	if err != nil {
		return
	}

	accessor, err = meta.Accessor(object)
	return
}

func (d *daemonset) Ensure(parent, child Applier, generations []operatorsv1.GenerationStatus) (current runtime.Object, accessor metav1.Object, err error) {
	desired := d.asset.DaemonSet().New()

	if parent != nil {
		parent.Apply(desired)
	}
	if child != nil {
		child.Apply(&desired.Spec.Template)
	}

	current, _, err = resourceapply.ApplyDaemonSet(gocontext.TODO(), d.client.AppsV1(), d.recorder, desired, resourcemerge.ExpectedDaemonSetGeneration(desired, generations))
	if err != nil {
		return
	}

	accessor, err = meta.Accessor(current)
	return
}
