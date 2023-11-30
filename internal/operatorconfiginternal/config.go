package operatorconfiginternal

import (
	"fmt"
	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	goflags "github.com/jessevdk/go-flags"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	yamlv2 "gopkg.in/yaml.v2"
	"os"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
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
	var mainLogger logr.Logger
	if operatorconfig.CustomLoggerSetup != nil {
		mainLogger = operatorconfig.CustomLoggerSetup()
	} else {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
		zerologr.NameFieldName = "source"
		zerologr.VerbosityFieldName = ""
		zerologr.NameSeparator = "/"
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.StampMilli,
		}
		zl := zerolog.New(output).With().Timestamp().Logger()
		if cfg.GetDefaultConfig().LoggingType == "prod" {
			zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
			zl = zerolog.New(os.Stdout).With().Timestamp().Logger()
		}
		if cfg.GetDefaultConfig().LoggingLevel == "debug" {
			zerologr.SetMaxV(1)
		} else {
			zerologr.SetMaxV(0)
		}
		mainLogger = zerologr.New(&zl)
	}
	commitSha := os.Getenv("GIT_COMMIT_SHA")
	if commitSha == "" {
		commitSha = "local-build"
	}
	buildDate := os.Getenv("BUILD_DATE")
	if buildDate == "" {
		buildDate = "unknown"
	}
	mainLogger.WithValues("Commit sha", commitSha, "Build date", buildDate, "Log level", cfg.GetDefaultConfig().LoggingLevel, "Log type", cfg.GetDefaultConfig().LoggingType).Info("Operator info")
	mainLogger.V(1).Info("Debug logging activated")
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
