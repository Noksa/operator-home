package operatorbootstrap

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Noksa/operator-home/internal/operatorbootstrapinternal"
	"github.com/Noksa/operator-home/internal/operatorconfiginternal"
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/samber/lo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Bootstrapper wires up configuration, manager, and controllers for an operator.
type Bootstrapper struct {
	mgr       manager.Manager
	ctx       context.Context
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

// Cancelled reports whether a shutdown signal has been received.
func (b *Bootstrapper) Cancelled() bool {
	return b.cancelled.Load()
}

// NewBootstrapper creates a Bootstrapper. It initializes the operator config
// first (parses CLI flags, loads the YAML config file), then calls optsModifier
// with a ctrl.Options pre-populated from DefaultConfig:
//
//   - Metrics.BindAddress  ← MetricsAddr
//   - HealthProbeBindAddress ← ProbeAddr
//   - LeaderElection       ← EnableLeaderElection
//   - BaseContext           ← bootstrapper's internal context (always set)
//
// optsModifier receives these defaults and returns the final options. Only set
// operator-specific fields inside it: Scheme, LeaderElectionID, WebhookServer, etc.
// Config values that live outside DefaultConfig (e.g. cfg.Cfg.Namespaces.System)
// are safe to access inside the modifier because config is already initialised.
//
// Pass nil for optsModifier to use the defaults unchanged.
//
// The parent ctx controls the manager lifetime and is wrapped internally, so
// both OS signals (SetupSignalHandler) and external cancellation stop everything.
func NewBootstrapper(ctx context.Context, operatorCfg operatorconfig.OperatorConfig, optsModifier func(ctrl.Options) ctrl.Options, mgrFunc operatorbootstrapinternal.ManagerFunc) *Bootstrapper {
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

	mgr := operatorbootstrapinternal.NewManager(opts, mgrFunc)
	return &Bootstrapper{mgr: mgr, ctx: ctx, cancel: cancel}
}

// GetMgr returns the underlying controller-runtime manager.
func (b *Bootstrapper) GetMgr() manager.Manager {
	return b.mgr
}

// Context returns the context that governs the manager and all goroutines that
// should stop when the operator shuts down.
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

// Run starts health/readiness probes and the manager. Blocks until the
// bootstrapper's context is done.
func (b *Bootstrapper) Run() {
	lo.Must0(b.GetMgr().AddHealthzCheck("healthz", healthz.Ping), "unable to setup healthz")
	lo.Must0(b.GetMgr().AddReadyzCheck("readyz", healthz.Ping), "unable to setup readyz")
	lo.Must0(b.mgr.Start(b.ctx))
}

// SetupSignalHandler installs OS signal handling (SIGINT, SIGTERM). On the
// first signal the bootstrapper's context is cancelled after gracefulShutdownDelay,
// stopping the manager and any goroutines using Context(). A second signal exits
// immediately. Pass 0 for no delay.
//
// Returns Context() for convenience — use the returned value wherever a
// signal-aware context is needed without calling Context() separately.
func (b *Bootstrapper) SetupSignalHandler(gracefulShutdownDelay time.Duration) context.Context {
	setupSignalHandler(gracefulShutdownDelay, &b.cancelled, b.cancel)
	return b.ctx
}

// CustomSignalsHandler is a standalone signal handler for code that needs a
// cancellable context before constructing a Bootstrapper. On the first
// SIGINT/SIGTERM the returned context is cancelled after gracefulShutdownDelay.
// A second signal exits immediately. Pass 0 for no delay.
func CustomSignalsHandler(gracefulShutdownDelay time.Duration) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	var dummy atomic.Bool
	setupSignalHandler(gracefulShutdownDelay, &dummy, cancel)
	return ctx
}

func setupSignalHandler(gracefulShutdownDelay time.Duration, cancelled *atomic.Bool, cancel context.CancelFunc) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-c
		logger := log.Log.WithName("SignalHandler")
		if s != nil {
			logger.WithValues("Signal", s.String()).Info("Received the signal")
		}
		cancelled.Store(true)
		if gracefulShutdownDelay > 0 {
			logger.Info("Waiting before shutdown", "delay", gracefulShutdownDelay)
			time.Sleep(gracefulShutdownDelay)
		}
		logger.Info("Cancelling context")
		cancel()
		<-c
		os.Exit(999) // second signal — exit immediately
	}()
}
