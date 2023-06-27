package operatorconfig

type OperatorConfig interface {
	LogLevel() string
	LogType() string
}

type DefaultConfig struct {
	MetricsAddr          string `long:"metrics-bind-address" description:"The address the metrics endpoint binds to." default:":8080"`
	ProbeAddr            string `long:"health-probe-bind-address" description:"The address the probe endpoint binds to." default:":8081"`
	EnableLeaderElection bool   `long:"enable-leader-election" description:"LeaderElection configMap name"`
	LoggingLevel         string `long:"loglevel" description:"Can be debug or info" default:"info"`
	LoggingType          string `long:"logtype" description:"Can be prod or dev" default:"dev"`
	ConfigPath           string `long:"config" description:"The path to the operator's config'" default:"/etc/kazoo-operator/config.yaml"`
	LocalEnv             bool   `long:"localEnv" description:"DEBUG ONLY!"`
	Kubeconfig           string `long:"kubeconfig" description:"used locally to find and use an approptiate kubeconfig file when you have a lot of them. Optional"`
}
