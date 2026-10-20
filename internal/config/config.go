package config

import (
	"flag"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	MetricsAddr          string `mapstructure:"metrics-bind-address"`
	HealthProbeAddr      string `mapstructure:"health-probe-bind-address"`
	EnableLeaderElection bool   `mapstructure:"leader-elect"`
	SecureMetrics        bool   `mapstructure:"metrics-secure"`
	EnableHTTP2          bool   `mapstructure:"enable-http2"`

	LogLevel  string `mapstructure:"log-level"`
	LogFormat string `mapstructure:"log-format"`

	APIPort int    `mapstructure:"api-port"`
	APIHost string `mapstructure:"api-host"`
	WebDir  string `mapstructure:"web-dir"`

	AuthEnabled  bool   `mapstructure:"auth-enabled"`
	AuthAudience string `mapstructure:"auth-audience"`

	DatabaseHost     string `mapstructure:"database-host"`
	DatabasePort     int    `mapstructure:"database-port"`
	DatabaseUser     string `mapstructure:"database-user"`
	DatabasePassword string `mapstructure:"database-password"`
	DatabaseName     string `mapstructure:"database-name"`
	DatabaseSSLMode  string `mapstructure:"database-sslmode"`

	ConfigFile string
}

// LoadConfig 读取 YAML 和环境变量，并仅用显式传入的认证 flag 覆盖它们。
// flags 可为 nil；其余既有 flag 的加载行为保持不变。
func LoadConfig(configFile string, flags *flag.FlagSet) (*Config, error) {
	v := viper.New()

	v.SetDefault("metrics-bind-address", ":8443")
	v.SetDefault("health-probe-bind-address", ":8081")
	v.SetDefault("leader-elect", true)
	v.SetDefault("metrics-secure", true)
	v.SetDefault("enable-http2", false)

	v.SetDefault("log-level", "info")
	v.SetDefault("log-format", "console")

	v.SetDefault("api-port", 8080)
	v.SetDefault("api-host", "0.0.0.0")
	v.SetDefault("web-dir", "/web/dist")

	v.SetDefault("auth-enabled", false)
	v.SetDefault("auth-audience", "")

	v.SetDefault("database-host", "localhost")
	v.SetDefault("database-port", 5432)
	v.SetDefault("database-user", "kubetask")
	v.SetDefault("database-password", "kubetask")
	v.SetDefault("database-name", "kubetask")
	v.SetDefault("database-sslmode", "disable")

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("kubetask")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	v.SetEnvPrefix("KUBETASK")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	if flags != nil {
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "auth-enabled", "auth-audience":
				v.Set(f.Name, f.Value.String())
			}
		})
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.AuthAudience = strings.TrimSpace(cfg.AuthAudience)
	if cfg.AuthEnabled && cfg.AuthAudience == "" {
		return nil, fmt.Errorf("auth-audience is required when auth-enabled is true")
	}

	cfg.ConfigFile = v.ConfigFileUsed()
	return cfg, nil
}
