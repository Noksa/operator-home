package operatorbootstrap

import (
	"context"
	"github.com/samber/lo"
	"os"
	"os/signal"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"syscall"
)

type Bootstrapper struct {
	mgr manager.Manager
}

var cancelled = false

func Cancelled() bool {
	return cancelled
}

func NewBootstrapper(mgr manager.Manager) *Bootstrapper {
	b := &Bootstrapper{mgr: mgr}
	return b
}

func (b *Bootstrapper) GetMgr() manager.Manager {
	return b.mgr
}

func (b *Bootstrapper) WithControllers(controllers ...KubernetesOperator) *Bootstrapper {
	for _, controller := range controllers {
		lo.Must0(controller.SetupWithManager(b.mgr))
	}
	return b
}

func (b *Bootstrapper) Run(ctx context.Context) {
	lo.Must0(b.GetMgr().AddHealthzCheck("healthz", healthz.Ping), "unable to setup healthz")
	lo.Must0(b.GetMgr().AddReadyzCheck("readyz", healthz.Ping), "unable to setup readyz")
	lo.Must0(b.mgr.Start(ctx))
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
