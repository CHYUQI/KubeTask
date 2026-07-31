package config

import (
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

	DatabaseHost     string `mapstructure:"database-host"`
	DatabasePort     int    `mapstructure:"database-port"`
	DatabaseUser     string `mapstructure:"database-user"`
	DatabasePassword string `mapstructure:"database-password"`
	DatabaseName     string `mapstructure:"database-name"`
	DatabaseSSLMode  string `mapstructure:"database-sslmode"`

	ConfigFile string
}

func LoadConfig(configFile string) (*Config, error) {
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

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.ConfigFile = v.ConfigFileUsed()
	return cfg, nil
}
