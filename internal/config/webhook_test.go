package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kubetask.io/kubetask/internal/config"
)

const (
	testWebhookURL    = "https://example.com/hook?token=private-token"
	testWebhookURLKey = "webhook-url"
)

func loadWebhookConfig(t *testing.T, yaml string, env map[string]string) (*config.Config, error) {
	t.Helper()
	for _, key := range []string{"AUTH_ENABLED", "AUTH_AUDIENCE", "WEBHOOK_ENABLED", "WEBHOOK_URL", "WEBHOOK_TIMEOUT", "WEBHOOK_MAX_RETRIES"} {
		t.Setenv("KUBETASK_"+key, env[key])
	}
	path := filepath.Join(t.TempDir(), "kubetask.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return config.LoadConfig(path, nil)
}

func TestWebhookDefaultsAndEnvironmentOverrides(t *testing.T) {
	cfg, err := loadWebhookConfig(t, "{}", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WebhookEnabled || cfg.WebhookURL != "" || cfg.WebhookTimeout != 5*time.Second || cfg.WebhookMaxRetries != 3 {
		t.Fatal("unexpected webhook defaults")
	}
	cfg, err = loadWebhookConfig(t, "webhook-enabled: false\nwebhook-timeout: 2s\nwebhook-max-retries: 1", map[string]string{
		"WEBHOOK_ENABLED": "true", "WEBHOOK_URL": testWebhookURL, "WEBHOOK_TIMEOUT": "10s", "WEBHOOK_MAX_RETRIES": "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WebhookEnabled || cfg.WebhookURL != testWebhookURL || cfg.WebhookTimeout != 10*time.Second || cfg.WebhookMaxRetries != 0 {
		t.Fatal("webhook environment did not override YAML")
	}
}

func TestWebhookValidationAndURLRedaction(t *testing.T) {
	for _, tt := range []struct{ name, yaml, errorKey string }{
		{name: "missing URL", errorKey: testWebhookURLKey},
		{name: "relative URL", yaml: "webhook-url: /private-token", errorKey: testWebhookURLKey},
		{name: "wrong protocol", yaml: "webhook-url: ftp://example.com/private-token", errorKey: testWebhookURLKey},
		{name: "userinfo", yaml: "webhook-url: https://user:private-token@example.com", errorKey: testWebhookURLKey},
		{name: "fragment", yaml: "webhook-url: https://example.com/#private-token", errorKey: testWebhookURLKey},
		{name: "invalid escape", yaml: "webhook-url: https://example.com/%private-token", errorKey: testWebhookURLKey},
		{name: "non-string URL", yaml: "webhook-url: {token: private-token}", errorKey: testWebhookURLKey},
		{name: "timeout too short", yaml: "webhook-url: " + testWebhookURL + "\nwebhook-timeout: 0s", errorKey: "webhook-timeout"},
		{name: "timeout too long", yaml: "webhook-url: " + testWebhookURL + "\nwebhook-timeout: 31s", errorKey: "webhook-timeout"},
		{name: "negative retry count", yaml: "webhook-url: " + testWebhookURL + "\nwebhook-max-retries: -1", errorKey: "webhook-max-retries"},
		{name: "too many retries", yaml: "webhook-url: " + testWebhookURL + "\nwebhook-max-retries: 6", errorKey: "webhook-max-retries"},
		{name: "valid YAML", yaml: "webhook-url: " + testWebhookURL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadWebhookConfig(t, "webhook-enabled: true\n"+tt.yaml, nil)
			if tt.errorKey == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errorKey) {
				t.Fatalf("expected error for %s, got %v", tt.errorKey, err)
			}
			if strings.Contains(err.Error(), "private-token") {
				t.Fatal("configuration error leaked credentials")
			}
		})
	}
}
