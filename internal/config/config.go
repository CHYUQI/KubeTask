package config

import (
	"flag"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

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

	AuthEnabled            bool   `mapstructure:"auth-enabled"`
	AuthAudience           string `mapstructure:"auth-audience"`
	AuthAPIKeysFile        string `mapstructure:"auth-api-keys-file"`
	TenantIsolationEnabled bool   `mapstructure:"tenant-isolation-enabled"`

	WebhookEnabled    bool          `mapstructure:"webhook-enabled"`
	WebhookURL        string        `mapstructure:"webhook-url"`
	WebhookTimeout    time.Duration `mapstructure:"webhook-timeout"`
	WebhookMaxRetries int           `mapstructure:"webhook-max-retries"`

	SMTPEnabled         bool          `mapstructure:"smtp-enabled"`
	SMTPAddress         string        `mapstructure:"smtp-address"`
	SMTPTLS             string        `mapstructure:"smtp-tls"`
	SMTPUsername        string        `mapstructure:"smtp-username"`
	SMTPPassword        string        `mapstructure:"smtp-password"`
	SMTPFrom            string        `mapstructure:"smtp-from"`
	SMTPTo              []string      `mapstructure:"smtp-to"`
	SMTPTimeout         time.Duration `mapstructure:"smtp-timeout"`
	SMTPMaxRetries      int           `mapstructure:"smtp-max-retries"`
	SMTPSubjectTemplate string        `mapstructure:"smtp-subject-template"`
	SMTPBodyTemplate    string        `mapstructure:"smtp-body-template"`

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
	v.SetDefault("auth-api-keys-file", "")
	v.SetDefault("tenant-isolation-enabled", false)

	v.SetDefault("webhook-enabled", false)
	v.SetDefault("webhook-url", "")
	v.SetDefault("webhook-timeout", "5s")
	v.SetDefault("webhook-max-retries", 3)

	v.SetDefault("smtp-enabled", false)
	v.SetDefault("smtp-address", "")
	v.SetDefault("smtp-tls", "starttls")
	v.SetDefault("smtp-username", "")
	v.SetDefault("smtp-password", "")
	v.SetDefault("smtp-from", "")
	v.SetDefault("smtp-to", []string{})
	v.SetDefault("smtp-timeout", "10s")
	v.SetDefault("smtp-max-retries", 3)
	v.SetDefault("smtp-subject-template", "")
	v.SetDefault("smtp-body-template", "")

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

	for _, key := range []string{"webhook-url", "smtp-username", "smtp-password"} {
		if _, ok := v.Get(key).(string); !ok {
			// Do not let mapstructure include a secret-bearing value in its error.
			return nil, fmt.Errorf("%s must be a string", key)
		}
	}
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.AuthAudience = strings.TrimSpace(cfg.AuthAudience)
	if cfg.AuthEnabled && cfg.AuthAudience == "" {
		return nil, fmt.Errorf("auth-audience is required when auth-enabled is true")
	}
	if cfg.TenantIsolationEnabled && !cfg.AuthEnabled {
		return nil, fmt.Errorf("tenant-isolation-enabled requires auth-enabled")
	}
	if cfg.AuthAPIKeysFile != "" && !cfg.AuthEnabled {
		return nil, fmt.Errorf("auth-api-keys-file requires auth-enabled")
	}
	if err := cfg.validateWebhook(); err != nil {
		return nil, err
	}
	if err := cfg.validateSMTP(); err != nil {
		return nil, err
	}

	cfg.ConfigFile = v.ConfigFileUsed()
	return cfg, nil
}

func (c *Config) validateSMTP() error {
	if !c.SMTPEnabled {
		return nil
	}
	c.SMTPAddress = strings.TrimSpace(c.SMTPAddress)
	host, portText, err := net.SplitHostPort(c.SMTPAddress)
	port, portErr := strconv.Atoi(portText)
	if err != nil || portErr != nil || host == "" || strings.ContainsAny(host, "/\\@ \t\r\n") || port < 1 || port > 65535 {
		return fmt.Errorf("smtp-address must contain a host and a port between 1 and 65535")
	}
	if c.SMTPTLS != "starttls" && c.SMTPTLS != "implicit" {
		return fmt.Errorf("smtp-tls must be starttls or implicit")
	}
	if (c.SMTPUsername == "") != (c.SMTPPassword == "") || strings.ContainsAny(c.SMTPUsername+c.SMTPPassword, "\x00\r\n") {
		return fmt.Errorf("smtp-username and smtp-password must both be set or both be empty, without NUL or line breaks")
	}
	c.SMTPFrom = strings.TrimSpace(c.SMTPFrom)
	if !validSMTPAddress(c.SMTPFrom) {
		return fmt.Errorf("smtp-from must be a bare ASCII email address")
	}
	if len(c.SMTPTo) == 0 {
		return fmt.Errorf("smtp-to must contain at least one email address")
	}
	seen := make(map[string]bool)
	recipients := make([]string, 0, len(c.SMTPTo))
	for _, recipient := range c.SMTPTo {
		recipient = strings.TrimSpace(recipient)
		if !validSMTPAddress(recipient) {
			return fmt.Errorf("smtp-to must contain bare ASCII email addresses")
		}
		if !seen[recipient] {
			seen[recipient] = true
			recipients = append(recipients, recipient)
		}
	}
	c.SMTPTo = recipients
	if c.SMTPTimeout < time.Second || c.SMTPTimeout > time.Minute {
		return fmt.Errorf("smtp-timeout must be between 1s and 60s")
	}
	if c.SMTPMaxRetries < 0 || c.SMTPMaxRetries > 5 {
		return fmt.Errorf("smtp-max-retries must be between 0 and 5")
	}
	return nil
}

func validSMTPAddress(value string) bool {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Name != "" || address.Address != value {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool { return r <= ' ' || r >= 127 }) == -1
}

func (c *Config) validateWebhook() error {
	if !c.WebhookEnabled {
		return nil
	}
	c.WebhookURL = strings.TrimSpace(c.WebhookURL)
	u, err := url.Parse(c.WebhookURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || strings.Contains(c.WebhookURL, "#") {
		// URL parsing errors can contain credentials from the original address.
		return fmt.Errorf("webhook-url must be an HTTP(S) URL with a host and without userinfo or fragment")
	}
	if c.WebhookTimeout < time.Second || c.WebhookTimeout > 30*time.Second {
		return fmt.Errorf("webhook-timeout must be between 1s and 30s")
	}
	if c.WebhookMaxRetries < 0 || c.WebhookMaxRetries > 5 {
		return fmt.Errorf("webhook-max-retries must be between 0 and 5")
	}
	return nil
}
