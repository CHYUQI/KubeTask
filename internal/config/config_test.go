package config_test

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kubetask.io/kubetask/internal/config"
)

const (
	testEnabledYAML      = "auth-enabled: true"
	testTrueString       = "true"
	testFlagAudience     = "flag-audience"
	missingAudienceError = "auth-audience is required"
	testAudience         = "kubetask"
	testAudienceFlag     = "-auth-audience=flag-audience"
	testEnvAudience      = "env-audience"
)

func TestLoadAuthenticationConfig(t *testing.T) {
	for _, tt := range []struct {
		name        string
		yaml        string
		envEnabled  string
		envAudience string
		args        []string
		enabled     bool
		audience    string
		wantError   string
	}{
		{name: "defaults"},
		{name: "YAML enabled", yaml: "auth-enabled: true\nauth-audience: kubetask", enabled: true, audience: testAudience},
		{name: "missing audience", yaml: testEnabledYAML, wantError: missingAudienceError},
		{name: "blank audience", yaml: "auth-enabled: true\nauth-audience: '  '", wantError: missingAudienceError},
		{name: "trim audience", yaml: "auth-enabled: true\nauth-audience: ' kubetask '", enabled: true, audience: testAudience},
		{name: "disabled with audience", yaml: "auth-enabled: false\nauth-audience: kubetask", audience: testAudience},
		{name: "invalid YAML bool", yaml: "auth-enabled: not-a-bool", wantError: "failed to unmarshal config"},
		{name: "environment enabled", envEnabled: testTrueString, envAudience: testEnvAudience, enabled: true, audience: testEnvAudience},
		{name: "environment overrides YAML", yaml: "auth-enabled: false\nauth-audience: yaml-audience", envEnabled: testTrueString, envAudience: testEnvAudience, enabled: true, audience: testEnvAudience},
		{name: "environment disables YAML", yaml: testEnabledYAML, envEnabled: "false"},
		{name: "environment missing audience", envEnabled: testTrueString, wantError: missingAudienceError},
		{name: "invalid environment bool", envEnabled: "not-a-bool", wantError: "failed to unmarshal config"},
		{name: "flags enabled", args: []string{"-auth-enabled", testAudienceFlag}, enabled: true, audience: testFlagAudience},
		{name: "flag supplies required YAML audience", yaml: testEnabledYAML, args: []string{testAudienceFlag}, enabled: true, audience: testFlagAudience},
		{name: "flags override environment", yaml: "auth-enabled: false\nauth-audience: yaml-audience", envEnabled: "false", envAudience: testEnvAudience, args: []string{"-auth-enabled=true", testAudienceFlag}, enabled: true, audience: testFlagAudience},
		{name: "explicit false overrides environment", envEnabled: testTrueString, args: []string{"-auth-enabled=false"}},
		{name: "explicit empty audience overrides environment", envEnabled: testTrueString, envAudience: testEnvAudience, args: []string{"-auth-audience="}, wantError: missingAudienceError},
		{name: "unvisited flag defaults preserve YAML", yaml: "auth-enabled: true\nauth-audience: yaml-audience", enabled: true, audience: "yaml-audience"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KUBETASK_AUTH_ENABLED", tt.envEnabled)
			t.Setenv("KUBETASK_AUTH_AUDIENCE", tt.envAudience)
			path := filepath.Join(t.TempDir(), "kubetask.yaml")
			yaml := tt.yaml
			if yaml == "" {
				yaml = "{}"
			}
			if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			flags := flag.NewFlagSet("authentication", flag.ContinueOnError)
			flags.SetOutput(io.Discard)
			flags.Bool("auth-enabled", false, "")
			flags.String("auth-audience", "", "")
			if err := flags.Parse(tt.args); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.LoadConfig(path, flags)
			if tt.wantError != "" {
				if cfg != nil || err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("LoadConfig returned config=%v, error=%v; want %q", cfg != nil, err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AuthEnabled != tt.enabled || cfg.AuthAudience != tt.audience || cfg.ConfigFile != path {
				t.Errorf("authentication config = (%v, %q), want (%v, %q) and original config path", cfg.AuthEnabled, cfg.AuthAudience, tt.enabled, tt.audience)
			}
		})
	}
}

func TestLoadConfigOnlyOverridesAuthenticationFlags(t *testing.T) {
	t.Setenv("KUBETASK_AUTH_ENABLED", "")
	t.Setenv("KUBETASK_AUTH_AUDIENCE", "")
	t.Setenv("KUBETASK_LEADER_ELECT", "")
	path := filepath.Join(t.TempDir(), "kubetask.yaml")
	if err := os.WriteFile(path, []byte("leader-elect: true"), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := flag.NewFlagSet("existing flags", flag.ContinueOnError)
	flags.Bool("leader-elect", true, "")
	if err := flags.Parse([]string{"-leader-elect=false"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path, flags)
	if err != nil || !cfg.EnableLeaderElection {
		t.Fatalf("existing flag behavior changed: config returned=%v, error=%v", cfg != nil, err)
	}
}
