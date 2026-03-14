package operatorbootstrapinternal

import (
	"github.com/samber/lo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ManagerFunc is an optional callback invoked after the manager is created
// but before it is returned, allowing callers to register indexers, etc.
type ManagerFunc func(mgr manager.Manager)

// NewManager creates a controller-runtime manager using the in-cluster or
// kubeconfig-based rest.Config from ctrl.GetConfigOrDie().
func NewManager(opts ctrl.Options, mgrFunc ManagerFunc) ctrl.Manager {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opts)
	if mgrFunc != nil {
		mgrFunc(mgr)
	}
	mgr = lo.Must(mgr, err)
	return mgr
}
