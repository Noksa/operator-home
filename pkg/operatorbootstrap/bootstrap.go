package operatorbootstrap

import (
	"context"
	"fmt"
	"github.com/Noksa/operator-home/internal/operatorbootstrapinternal"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	AddPodIndexersToManager operatorbootstrapinternal.ManagerFunc = func(mgr manager.Manager) {
		if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			return []string{o.(*v1.Pod).Spec.NodeName}
		}); err != nil {
			panic(fmt.Sprintf("Failed to setup pod indexer, %s", err))
		}
	}
)

func MustSetupController(err error) {
	lo.Must0(err, "couldn't create controller")
}
