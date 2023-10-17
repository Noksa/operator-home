package operatorconfiginternal

import (
	"fmt"
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/go-logr/zapr"
	goflags "github.com/jessevdk/go-flags"
	"github.com/samber/lo"
	"go.uber.org/zap"
	yamlv2 "gopkg.in/yaml.v2"
	"os"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
)

func InstantiateConfiguration(cfg operatorconfig.OperatorConfig) {
	// pointer check
	kind := reflect.ValueOf(cfg).Kind()
	if reflect.ValueOf(cfg).Kind() != reflect.Ptr {
		panic(fmt.Sprintf("InstantiateConfiguration required pointer. Got %v", kind))
	}
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
		err = yamlv2.Unmarshal(b, cfg)
		if err != nil {
			mainLogger.Error(err, "Couldn't unmarshal the additional operator config file. Check that the config is yaml and correct")
			panic(err)
		}
		err = cfg.Initialize()
		if err != nil {
			panic(fmt.Sprintf("couldn't initialize config: %v", err))
		}
	}
}
