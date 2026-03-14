package operatorconfig

import "github.com/go-logr/logr"

// OperatorConfig is implemented by operator-specific configuration structs.
type OperatorConfig interface {
	GetDefaultConfig() DefaultConfig
	// Initialize may help to do additional initialization during configuration instantiation.
	Initialize() error
}

// DefaultConfig holds common operator configuration fields.
type DefaultConfig struct {
	MetricsAddr          string `long:"metrics-bind-address" description:"The address the metrics endpoint binds to." default:":8080"`
	ProbeAddr            string `long:"health-probe-bind-address" description:"The address the probe endpoint binds to." default:":8081"`
	EnableLeaderElection bool   `long:"enable-leader-election" description:"LeaderElection configMap name"`
	LoggingLevel         string `long:"loglevel" description:"Can be debug or info" default:"info"`
	LoggingType          string `long:"logtype" description:"Can be prod or dev" default:"dev"`
	ConfigPath           string `long:"config" description:"The path to an additional custom operator's config'" default:""`
	LocalEnv             bool   `long:"localEnv" description:"DEBUG ONLY!"`
	Kubeconfig           string `long:"kubeconfig" description:"used locally to find and use an appropriate kubeconfig file when you have a lot of them. Optional"`
	// CustomLoggerSetup, when set, overrides the default zerolog-based logger.
	CustomLoggerSetup func() logr.Logger `long:"-" no-flag:"true"`
}

func (d DefaultConfig) GetDefaultConfig() DefaultConfig {
	return d
}
