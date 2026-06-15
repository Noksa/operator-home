package operatorbootstrap

import (
	"context"
	"fmt"

	"github.com/Noksa/operator-home/internal/operatorbootstrapinternal"
	"github.com/Noksa/operator-home/internal/operatorconfiginternal"
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/samber/lo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
)

// MultiClusterBootstrapper extends Bootstrapper with multicluster-runtime support.
type MultiClusterBootstrapper struct {
	Bootstrapper
	mcMgr    mcmanager.Manager
	provider multicluster.Provider
}

// NewMultiClusterBootstrapper creates a bootstrapper that wraps the controller-runtime
// manager with multicluster-runtime's mcmanager.Manager. The provider is used to
// discover and engage remote clusters dynamically.
//
// IMPORTANT: Call MustInitConfig() before this if you need config values to construct
// the provider. If config is not yet initialized, this function will initialize it.
//
// Usage:
//
//	operatorbootstrap.MustInitConfig(&cfg.Cfg) // populate config first
//	provider := kubeconfig.New(kubeconfig.Options{Namespace: cfg.Cfg.Namespace, ...})
//	b := operatorbootstrap.NewMultiClusterBootstrapper(ctx, &cfg.Cfg, provider, optsModifier, nil)
//	b.SetupSignalHandler(0)
//	provider.SetupWithManager(b.Context(), b.GetMultiClusterMgr())
//	b.Run()
func NewMultiClusterBootstrapper(
	ctx context.Context,
	operatorCfg operatorconfig.OperatorConfig,
	provider multicluster.Provider,
	optsModifier func(ctrl.Options) ctrl.Options,
	mgrFunc operatorbootstrapinternal.ManagerFunc,
) *MultiClusterBootstrapper {
	// Config init is idempotent — safe to call even if MustInitConfig was already called
	operatorconfiginternal.InstantiateConfiguration(operatorCfg)

	dc := operatorCfg.GetDefaultConfig()
	opts := ctrl.Options{
		Metrics:                metricsserver.Options{BindAddress: dc.MetricsAddr},
		HealthProbeBindAddress: dc.ProbeAddr,
		LeaderElection:         dc.EnableLeaderElection,
	}
	if optsModifier != nil {
		opts = optsModifier(opts)
	}

	ctx, cancel := context.WithCancel(ctx)
	opts.BaseContext = func() context.Context { return ctx }

	// Create the base controller-runtime manager
	baseMgr := operatorbootstrapinternal.NewManager(opts, mgrFunc)

	// Wrap with multicluster support
	mcMgr, err := mcmanager.WithMultiCluster(baseMgr, provider)
	if err != nil {
		panic(fmt.Sprintf("failed to create multicluster manager: %v", err))
	}

	return &MultiClusterBootstrapper{
		Bootstrapper: Bootstrapper{mgr: baseMgr, ctx: ctx, cancel: cancel},
		mcMgr:        mcMgr,
		provider:     provider,
	}
}

// GetMultiClusterMgr returns the multicluster-aware manager.
// Use this for provider.SetupWithManager() and mgr.Add() of Aware runnables.
func (b *MultiClusterBootstrapper) GetMultiClusterMgr() mcmanager.Manager {
	return b.mcMgr
}

// GetLocalMgr returns the underlying controller-runtime manager for the local cluster.
// Use this for controller.SetupWithManager() and other local-only operations.
func (b *MultiClusterBootstrapper) GetLocalMgr() manager.Manager {
	return b.mcMgr.GetLocalManager()
}

// Run starts health/readiness probes and the multicluster manager. Blocks until done.
func (b *MultiClusterBootstrapper) Run() {
	lo.Must0(b.mcMgr.AddHealthzCheck("healthz", healthz.Ping), "unable to setup healthz")
	lo.Must0(b.mcMgr.AddReadyzCheck("readyz", healthz.Ping), "unable to setup readyz")
	lo.Must0(b.mcMgr.Start(b.ctx))
}
