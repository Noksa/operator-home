package operatorbootstrap

import (
	"context"
	"fmt"
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/Noksa/operator-home/pkg/operatorkclient"
	"github.com/go-logr/zapr"
	goflags "github.com/jessevdk/go-flags"
	"github.com/samber/lo"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"time"
)

var (
	StartTime                           = time.Now()
	AddPodIndexersToManager ManagerFunc = func(mgr manager.Manager) {
		if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			return []string{o.(*v1.Pod).Spec.NodeName}
		}); err != nil {
			panic(fmt.Sprintf("Failed to setup pod indexer, %s", err))
		}
	}
)

type ManagerFunc func(mgr manager.Manager)

func MustSetupController(err error) {
	lo.Must0(err, "couldn't create controller")
}

func NewManager(opts ctrl.Options, mgrFunc ManagerFunc) ctrl.Manager {
	mgr, err := ctrl.NewManager(operatorkclient.GetClientConfig(), opts)
	if mgrFunc != nil {
		mgrFunc(mgr)
	}
	mgr = lo.Must(mgr, err)
	if opts.LeaderElection {
		StartTime = time.Now().Add(time.Second * 30)
	}
	return mgr
}

func InstantiateConfiguration(cfg operatorconfig.OperatorConfig) {
	flagParser := goflags.NewParser(cfg, goflags.IgnoreUnknown|goflags.PassDoubleDash|goflags.HelpFlag)
	_, firstError := flagParser.Parse()
	lo.Must0(firstError)
	var zapCfg zap.Config
	if cfg.LogType() == "prod" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
	}
	if cfg.LogLevel() == "debug" {
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	} else {
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	zapCfg.DisableCaller = true
	zapLogger, err := zapCfg.Build()
	lo.Must0(err)
	mainLogger := zapr.NewLogger(zapLogger)
	commitSha := os.Getenv("GIT_COMMIT_SHA")
	if commitSha == "" {
		commitSha = "local-build"
	}
	buildDate := os.Getenv("BUILD_DATE")
	if buildDate == "" {
		buildDate = "unknown"
	}
	mainLogger.WithValues("Commit sha", commitSha, "Build date", buildDate).Info("Operator info")
	ctrl.SetLogger(mainLogger)
}
