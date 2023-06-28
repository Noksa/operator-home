package operatorbootstrap

import (
	"context"
	"github.com/Noksa/operator-home/internal/operatorbootstrapinternal"
	"github.com/Noksa/operator-home/internal/operatorconfiginternal"
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/samber/lo"
	"os"
	"os/signal"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"syscall"
)

type Bootstrapper struct {
	mgr manager.Manager
	ctx context.Context
}

var cancelled = false

func Cancelled() bool {
	return cancelled
}

func NewBootstrapper(ctx context.Context, operatorCfg operatorconfig.OperatorConfig, newOpts func() ctrl.Options, mgrFunc operatorbootstrapinternal.ManagerFunc) *Bootstrapper {
	operatorconfiginternal.InstantiateConfiguration(operatorCfg)
	mgr := operatorbootstrapinternal.NewManager(newOpts(), mgrFunc)
	b := &Bootstrapper{mgr: mgr, ctx: ctx}
	return b
}

func (b *Bootstrapper) GetMgr() manager.Manager {
	return b.mgr
}

func (b *Bootstrapper) Context() context.Context {
	return b.ctx
}

func (b *Bootstrapper) WithControllers(controllers ...KubernetesOperator) *Bootstrapper {
	for _, controller := range controllers {
		lo.Must0(controller.SetupWithManager(b.mgr))
	}
	return b
}

func (b *Bootstrapper) Run() {
	lo.Must0(b.GetMgr().AddHealthzCheck("healthz", healthz.Ping), "unable to setup healthz")
	lo.Must0(b.GetMgr().AddReadyzCheck("readyz", healthz.Ping), "unable to setup readyz")
	lo.Must0(b.mgr.Start(b.ctx))
}

func CustomSignalsHandler(additionalActionBeforeCancel func()) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancelled = true
		additionalActionBeforeCancel()
		cancel()
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return ctx
}
