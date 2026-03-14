package operatorconfiginternal

import (
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/Noksa/operator-home/pkg/operatorconfig"
	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	goflags "github.com/jessevdk/go-flags"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	yamlv2 "gopkg.in/yaml.v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

// InstantiateConfiguration parses flags, sets up logging, and optionally
// reads an additional YAML config file into cfg.
func InstantiateConfiguration(cfg operatorconfig.OperatorConfig) {
	kind := reflect.ValueOf(cfg).Kind()
	if kind != reflect.Ptr {
		panic(fmt.Sprintf("InstantiateConfiguration requires a pointer. Got %v", kind))
	}
	flagParser := goflags.NewParser(cfg, goflags.IgnoreUnknown|goflags.PassDoubleDash|goflags.HelpFlag)
	_, firstError := flagParser.Parse()
	lo.Must0(firstError)

	dc := cfg.GetDefaultConfig()
	var mainLogger logr.Logger
	if dc.CustomLoggerSetup != nil {
		mainLogger = dc.CustomLoggerSetup()
	} else {
		mainLogger = defaultLogger(dc)
	}

	commitSha := envOrDefault("GIT_COMMIT_SHA", "local-build")
	buildDate := envOrDefault("BUILD_DATE", "unknown")
	mainLogger.WithValues("Commit sha", commitSha, "Build date", buildDate, "Log level", dc.LoggingLevel, "Log type", dc.LoggingType).Info("Operator info")
	mainLogger.V(1).Info("Debug logging activated")
	ctrl.SetLogger(mainLogger)

	if dc.ConfigPath != "" {
		b, err := os.ReadFile(dc.ConfigPath)
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

func defaultLogger(dc operatorconfig.DefaultConfig) logr.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zerologr.NameFieldName = "source"
	zerologr.VerbosityFieldName = ""
	zerologr.NameSeparator = "/"
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.StampMilli,
	}
	zl := zerolog.New(output).With().Timestamp().Logger()
	if dc.LoggingType == "prod" {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		zl = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	if dc.LoggingLevel == "debug" {
		zerologr.SetMaxV(1)
	} else {
		zerologr.SetMaxV(0)
	}
	return zerologr.New(&zl)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
