package operatorbootstrap

import (
	"context"
	"fmt"

	"github.com/Noksa/operator-home/internal/operatorbootstrapinternal"
	"github.com/Noksa/operator-home/internal/operatorconfiginternal"
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddPodIndexersToManager is a ManagerFunc that registers a field indexer
// for spec.nodeName on Pods.
func AddPodIndexersToManager(mgr manager.Manager) {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Pod{}, "spec.nodeName", func(o client.Object) []string {
		return []string{o.(*v1.Pod).Spec.NodeName}
	}); err != nil {
		panic(fmt.Sprintf("Failed to setup pod indexer, %s", err))
	}
}

// MustSetupController panics if err is non-nil.
func MustSetupController(err error) {
	lo.Must0(err, "couldn't create controller")
}

// MustInitConfig initializes the operator config (CLI flags, YAML file, defaults)
// without creating a manager. Use this when you need config values before constructing
// a provider or other components that depend on config.
func MustInitConfig(operatorCfg operatorconfig.OperatorConfig) {
	operatorconfiginternal.InstantiateConfiguration(operatorCfg)
}

// Ensure AddPodIndexersToManager satisfies the ManagerFunc type.
var _ operatorbootstrapinternal.ManagerFunc = AddPodIndexersToManager
