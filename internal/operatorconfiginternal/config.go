package operatorconfiginternal

import (
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/go-logr/zapr"
	goflags "github.com/jessevdk/go-flags"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/yaml"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
)

func InstantiateConfiguration(cfg operatorconfig.OperatorConfig) {
	flagParser := goflags.NewParser(cfg, goflags.IgnoreUnknown|goflags.PassDoubleDash|goflags.HelpFlag)
	_, firstError := flagParser.Parse()

	lo.Must0(firstError)
	var zapCfg zap.Config
	if cfg.GetDefaultConfig().LoggingType == "prod" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
	}
	if cfg.GetDefaultConfig().LoggingLevel == "debug" {
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
	if cfg.GetDefaultConfig().ConfigPath != "" {
		b, err := os.ReadFile(cfg.GetDefaultConfig().ConfigPath)
		if err != nil {
			mainLogger.Error(err, "Couldn't read the additional operator config file")
			panic(err)
		}
		err = yaml.Unmarshal(b, &cfg)
		if err != nil {
			mainLogger.Error(err, "Couldn't unmarshal the additional operator config file. Check that the config is yaml and correct")
			panic(err)
		}
	}
}
