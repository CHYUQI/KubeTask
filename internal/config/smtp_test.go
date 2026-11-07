package config_test

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

const validSMTPYAML = `smtp-enabled: true
smtp-address: smtp.example.com:587
smtp-from: alerts@example.com
smtp-to: [ops@example.com, ops@example.com]
`

const testSMTPEnabledYAML = "smtp-enabled: true"

func TestSMTPDefaultsAndEnvironment(t *testing.T) {
	for _, key := range []string{"SMTP_ENABLED", "SMTP_ADDRESS", "SMTP_TLS", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_TO", "SMTP_TIMEOUT", "SMTP_MAX_RETRIES"} {
		t.Setenv("KUBETASK_"+key, "")
	}
	cfg, err := loadWebhookConfig(t, "{}", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTPEnabled || cfg.SMTPTLS != "starttls" || cfg.SMTPTimeout != 10*time.Second || cfg.SMTPMaxRetries != 3 {
		t.Fatal("unexpected email defaults")
	}
	t.Setenv("KUBETASK_SMTP_TO", "first@example.com,second@example.com")
	t.Setenv("KUBETASK_SMTP_USERNAME", "sender")
	t.Setenv("KUBETASK_SMTP_PASSWORD", "private-password")
	cfg, err = loadWebhookConfig(t, validSMTPYAML, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.SMTPTo, []string{"first@example.com", "second@example.com"}) || cfg.SMTPPassword != "private-password" {
		t.Fatal("SMTP environment did not override YAML")
	}
	t.Setenv("KUBETASK_SMTP_TO", "")
	cfg, err = loadWebhookConfig(t, validSMTPYAML, nil)
	if err != nil || len(cfg.SMTPTo) != 1 {
		t.Fatalf("recipients were not deduplicated: %v", err)
	}
}

func TestSMTPValidationAndCredentialRedaction(t *testing.T) {
	for _, tt := range []struct{ name, from, to string }{
		{name: "missing host", from: "smtp.example.com:587", to: ":587"},
		{name: "invalid port", from: "smtp.example.com:587", to: "smtp.example.com:70000"},
		{name: "plaintext TLS mode", from: testSMTPEnabledYAML, to: "smtp-enabled: true\nsmtp-tls: none"},
		{name: "invalid sender", from: "alerts@example.com", to: "bad-sender"},
		{name: "header injection", from: "alerts@example.com", to: "\"alerts@example.com\\r\\nBcc: extra@example.com\""},
		{name: "empty recipients", from: "[ops@example.com, ops@example.com]", to: "[]"},
		{name: "invalid recipient", from: "[ops@example.com, ops@example.com]", to: "[invalid]"},
		{name: "partial credentials", from: testSMTPEnabledYAML, to: "smtp-enabled: true\nsmtp-password: private-password"},
		{name: "non-string password", from: testSMTPEnabledYAML, to: "smtp-enabled: true\nsmtp-password: {token: private-password}"},
		{name: "timeout too long", from: testSMTPEnabledYAML, to: "smtp-enabled: true\nsmtp-timeout: 61s"},
		{name: "too many retries", from: testSMTPEnabledYAML, to: "smtp-enabled: true\nsmtp-max-retries: 6"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadWebhookConfig(t, strings.Replace(validSMTPYAML, tt.from, tt.to, 1), nil)
			if err == nil || strings.Contains(err.Error(), "private-password") {
				t.Fatalf("validation or redaction failed: %v", err)
			}
		})
	}
}
