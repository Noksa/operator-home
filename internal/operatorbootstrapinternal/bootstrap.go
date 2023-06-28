package operatorbootstrapinternal

import (
	"github.com/Noksa/operator-home/pkg/operatorkclient"
	"github.com/samber/lo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func NewManager(opts ctrl.Options, mgrFunc ManagerFunc) ctrl.Manager {
	mgr, err := ctrl.NewManager(operatorkclient.GetClientConfig(), opts)
	if mgrFunc != nil {
		mgrFunc(mgr)
	}
	mgr = lo.Must(mgr, err)
	return mgr
}

type ManagerFunc func(mgr manager.Manager)
