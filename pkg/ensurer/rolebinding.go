package ensurer

import (
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/run-once-duration-override-operator/pkg/dynamic"
)

type RoleBindingEnsurer struct {
	client dynamic.Ensurer
}

func (r *RoleBindingEnsurer) Ensure(role *rbacv1.RoleBinding) (current *rbacv1.RoleBinding, err error) {
	unstructured, errGot := r.client.Ensure("rolebindings", role)
	if errGot != nil {
		err = errGot
		return
	}

	current = &rbacv1.RoleBinding{}
	if conversionErr := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.UnstructuredContent(), current); conversionErr != nil {
		err = conversionErr
		return
	}

	return
}
