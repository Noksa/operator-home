package operatorbootstrap

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type KubernetesOperator interface {
	reconcile.Reconciler
	SetupWithManager(mgr ctrl.Manager) error
}
