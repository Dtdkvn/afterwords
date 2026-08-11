package config

import (
	"strings"
	"testing"
)

func TestLogLevelValidation(t *testing.T) {
	setValidConfigEnv(t)
	for _, level := range []string{"debug", "info", "warn", "error", "WARN"} {
		t.Run("accepts_"+level, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", level)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LogLevel != strings.ToLower(level) {
				t.Fatalf("LOG_LEVEL=%q, want normalized %q", cfg.LogLevel, strings.ToLower(level))
			}
		})
	}
	t.Run("rejects invalid", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "verbose")
		if _, err := Load(); err == nil {
			t.Fatal("expected invalid LOG_LEVEL to fail")
		}
	})
}

func TestSMTPTLSPolicyValidation(t *testing.T) {
	setValidConfigEnv(t)
	for _, policy := range []string{"mandatory", "opportunistic", "none", "MANDATORY"} {
		t.Run("accepts_"+policy, func(t *testing.T) {
			t.Setenv("SMTP_TLS_POLICY", policy)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.SMTPTLSPolicy != strings.ToLower(policy) {
				t.Fatalf("SMTP_TLS_POLICY=%q, want normalized %q", cfg.SMTPTLSPolicy, strings.ToLower(policy))
			}
		})
	}
	t.Run("rejects invalid", func(t *testing.T) {
		t.Setenv("SMTP_TLS_POLICY", "sometimes")
		if _, err := Load(); err == nil {
			t.Fatal("expected invalid SMTP_TLS_POLICY to fail")
		}
	})
	t.Run("rejects credentials over plaintext", func(t *testing.T) {
		t.Setenv("SMTP_TLS_POLICY", "none")
		t.Setenv("SMTP_USER", "mailer")
		if _, err := Load(); err == nil {
			t.Fatal("expected SMTP_USER with SMTP_TLS_POLICY=none to fail")
		}
	})
}

func TestScalarConfigurationValidation(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "PORT syntax", key: "PORT", value: "eight"},
		{name: "PORT range", key: "PORT", value: "65536"},
		{name: "SMTP_PORT syntax", key: "SMTP_PORT", value: "smtp"},
		{name: "SMTP_PORT range", key: "SMTP_PORT", value: "0"},
		{name: "TIME_SCALE syntax", key: "TIME_SCALE", value: "fast"},
		{name: "TIME_SCALE zero", key: "TIME_SCALE", value: "0"},
		{name: "TIME_SCALE negative", key: "TIME_SCALE", value: "-2"},
		{name: "TIME_SCALE NaN", key: "TIME_SCALE", value: "NaN"},
		{name: "TIME_SCALE infinity", key: "TIME_SCALE", value: "+Inf"},
		{name: "SECURE_COOKIES", key: "SECURE_COOKIES", value: "yes"},
		{name: "TRUST_PROXY", key: "TRUST_PROXY", value: "sometimes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidConfigEnv(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load() error = %v, want error naming %s", err, test.key)
			}
		})
	}
}

func TestBaseURLValidation(t *testing.T) {
	for _, value := range []string{
		"localhost:8080",
		"ftp://example.com",
		"https:///missing-host",
		"http://:8080",
		"://invalid",
		"https://user:password@afterwords.example",
		"https://afterwords.example/app",
		"https://afterwords.example/?source=email",
		"https://afterwords.example/#setup",
	} {
		t.Run(value, func(t *testing.T) {
			setValidConfigEnv(t)
			t.Setenv("BASE_URL", value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "BASE_URL") {
				t.Fatalf("Load() error = %v, want BASE_URL validation error", err)
			}
		})
	}
	t.Run("accepts https host and trims trailing slash", func(t *testing.T) {
		setValidConfigEnv(t)
		t.Setenv("BASE_URL", "https://afterwords.example/")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BaseURL != "https://afterwords.example" {
			t.Fatalf("BaseURL = %q", cfg.BaseURL)
		}
	})
}

func setValidConfigEnv(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"PORT":            "8080",
		"BASE_URL":        "http://localhost:8080",
		"SMTP_PORT":       "1025",
		"SMTP_USER":       "",
		"SMTP_TLS_POLICY": "mandatory",
		"TIME_SCALE":      "1",
		"SECURE_COOKIES":  "false",
		"TRUST_PROXY":     "false",
		"LOG_LEVEL":       "info",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}
