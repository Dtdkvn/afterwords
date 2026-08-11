// Package config loads Afterwords' environment-only configuration.
package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is the complete server configuration.
type Config struct {
	Port          int
	BaseURL       string
	DBPath        string
	SMTPHost      string
	SMTPPort      int
	SMTPUser      string
	SMTPPass      string
	SMTPFrom      string
	SMTPTLSPolicy string
	TimeScale     float64
	SecureCookies bool
	TrustProxy    bool
	LogLevel      string
	SetupToken    string
}

// Load parses configuration from environment variables and validates it.
func Load() (Config, error) {
	port, err := envInt("PORT", 8080)
	if err != nil {
		return Config{}, err
	}
	smtpPort, err := envInt("SMTP_PORT", 1025)
	if err != nil {
		return Config{}, err
	}
	timeScale, err := envFloat("TIME_SCALE", 1)
	if err != nil {
		return Config{}, err
	}
	secureCookies, err := envBool("SECURE_COOKIES", false)
	if err != nil {
		return Config{}, err
	}
	trustProxy, err := envBool("TRUST_PROXY", false)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Port:          port,
		BaseURL:       env("BASE_URL", "http://localhost:8080"),
		DBPath:        env("DB_PATH", "./afterwords.db"),
		SMTPHost:      env("SMTP_HOST", ""),
		SMTPPort:      smtpPort,
		SMTPUser:      os.Getenv("SMTP_USER"),
		SMTPPass:      os.Getenv("SMTP_PASS"),
		SMTPFrom:      env("SMTP_FROM", "Afterwords <afterwords@localhost>"),
		SMTPTLSPolicy: env("SMTP_TLS_POLICY", "mandatory"),
		TimeScale:     timeScale,
		SecureCookies: secureCookies,
		TrustProxy:    trustProxy,
		LogLevel:      env("LOG_LEVEL", "info"),
		SetupToken:    os.Getenv("SETUP_TOKEN"),
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("PORT must be between 1 and 65535")
	}
	if cfg.SMTPPort < 1 || cfg.SMTPPort > 65535 {
		return Config{}, fmt.Errorf("SMTP_PORT must be between 1 and 65535")
	}
	if cfg.TimeScale <= 0 || math.IsNaN(cfg.TimeScale) || math.IsInf(cfg.TimeScale, 0) {
		return Config{}, fmt.Errorf("TIME_SCALE must be a finite number greater than zero")
	}
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("BASE_URL must be a valid http or https origin with a host")
	}
	scheme := strings.ToLower(baseURL.Scheme)
	if (scheme != "http" && scheme != "https") || baseURL.Hostname() == "" || baseURL.Opaque != "" {
		return Config{}, fmt.Errorf("BASE_URL must be a valid http or https origin with a host")
	}
	if baseURL.User != nil || (baseURL.Path != "" && baseURL.Path != "/") || baseURL.RawPath != "" || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return Config{}, fmt.Errorf("BASE_URL must be an origin without credentials, path, query, or fragment")
	}
	cfg.BaseURL = scheme + "://" + baseURL.Host
	switch strings.ToLower(cfg.LogLevel) {
	case "debug", "info", "warn", "error":
		cfg.LogLevel = strings.ToLower(cfg.LogLevel)
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, or error")
	}
	switch strings.ToLower(cfg.SMTPTLSPolicy) {
	case "mandatory", "opportunistic", "none":
		cfg.SMTPTLSPolicy = strings.ToLower(cfg.SMTPTLSPolicy)
	default:
		return Config{}, fmt.Errorf("SMTP_TLS_POLICY must be one of mandatory, opportunistic, or none")
	}
	if cfg.SMTPTLSPolicy == "none" && strings.TrimSpace(cfg.SMTPUser) != "" {
		return Config{}, fmt.Errorf("SMTP_USER cannot be used when SMTP_TLS_POLICY=none")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func envFloat(key string, fallback float64) (float64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", key, err)
	}
	return value, nil
}

func envBool(key string, fallback bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", key, err)
	}
	return value, nil
}
