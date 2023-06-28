package operatorconfiginternal

import (
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/go-logr/zapr"
	goflags "github.com/jessevdk/go-flags"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
)

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
