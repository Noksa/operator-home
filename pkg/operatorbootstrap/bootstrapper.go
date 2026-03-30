package operatorbootstrap

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/Noksa/operator-home/internal/operatorbootstrapinternal"
	"github.com/Noksa/operator-home/internal/operatorconfiginternal"
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/samber/lo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Bootstrapper wires up configuration, manager, and controllers for an operator.
type Bootstrapper struct {
	mgr       manager.Manager
	ctx       context.Context
	cancelled atomic.Bool
}

// Cancelled reports whether a shutdown signal has been received.
func (b *Bootstrapper) Cancelled() bool {
	return b.cancelled.Load()
}

// NewBootstrapper creates a Bootstrapper, instantiating configuration and the
// controller-runtime manager. Call SetupSignalHandler to establish the lifecycle context.
func NewBootstrapper(operatorCfg operatorconfig.OperatorConfig, newOpts func() ctrl.Options, mgrFunc operatorbootstrapinternal.ManagerFunc) *Bootstrapper {
	operatorconfiginternal.InstantiateConfiguration(operatorCfg)
	mgr := operatorbootstrapinternal.NewManager(newOpts(), mgrFunc)
	return &Bootstrapper{mgr: mgr, ctx: context.Background()}
}

// GetMgr returns the underlying controller-runtime manager.
func (b *Bootstrapper) GetMgr() manager.Manager {
	return b.mgr
}

// Context returns the context used by this bootstrapper.
func (b *Bootstrapper) Context() context.Context {
	return b.ctx
}

// WithControllers registers one or more controllers with the manager.
func (b *Bootstrapper) WithControllers(controllers ...KubernetesOperator) *Bootstrapper {
	for _, controller := range controllers {
		lo.Must0(controller.SetupWithManager(b.mgr))
	}
	return b
}

// Run starts health/readiness probes and the manager. Blocks until the context is done.
func (b *Bootstrapper) Run() {
	lo.Must0(b.GetMgr().AddHealthzCheck("healthz", healthz.Ping), "unable to setup healthz")
	lo.Must0(b.GetMgr().AddReadyzCheck("readyz", healthz.Ping), "unable to setup readyz")
	lo.Must0(b.mgr.Start(b.ctx))
}

// SetupSignalHandler installs OS signal handling (SIGINT, SIGTERM) and returns
// a context that is cancelled on the first signal. A second signal exits immediately.
// The optional callback runs before cancellation.
func (b *Bootstrapper) SetupSignalHandler(additionalActionBeforeCancel func()) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	b.ctx = ctx
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-c
		logger := log.Log.WithName("SignalHandler")
		if s != nil {
			logger.WithValues("Signal", s.String()).Info("Received the signal")
		}
		b.cancelled.Store(true)
		if additionalActionBeforeCancel != nil {
			logger.Info("Running additional actions before exit")
			additionalActionBeforeCancel()
		}
		logger.Info("Cancelling context")
		cancel()
		<-c
		os.Exit(999) // second signal — exit immediately
	}()
	return ctx
}
