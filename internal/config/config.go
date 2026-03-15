package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/en9inerd/go-pkgs/validator"
)

type Config struct {
	Port          string
	BaseURL       string
	MinPhraseSize int
	MaxPhraseSize int
	MaxItems      int
	MaxFileSize   int64
	MaxRetention  time.Duration

	// Runtime
	Verbose bool
}

func ParseConfig(getenv func(string) string) (*Config, error) {
	parsedBaseURL, err := sanitizeBaseURL(envStr(getenv, "SHHH_BASE_URL", ""))
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:          envStr(getenv, "SHHH_PORT", "8000"),
		BaseURL:       parsedBaseURL,
		MinPhraseSize: envInt(getenv, "SHHH_MIN_PHRASE_SIZE", 5),
		MaxPhraseSize: envInt(getenv, "SHHH_MAX_PHRASE_SIZE", 128),
		MaxItems:      envInt(getenv, "SHHH_MAX_ITEMS", 100),
		MaxFileSize:   envInt64(getenv, "SHHH_MAX_FILE_SIZE", 2*1024*1024),
		MaxRetention:  envDuration(getenv, "SHHH_MAX_RETENTION", 24*time.Hour),
		Verbose:       envBool(getenv, "SHHH_VERBOSE", false),
	}, nil
}

func envStr(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(getenv func(string) string, key string, fallback int) int {
	if v := getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envInt64(getenv func(string) string, key string, fallback int64) int64 {
	if v := getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

func envDuration(getenv func(string) string, key string, fallback time.Duration) time.Duration {
	if v := getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envBool(getenv func(string) string, key string, fallback bool) bool {
	if v := getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func sanitizeBaseURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	raw = strings.TrimRight(raw, "/")

	if !validator.IsHTTPURL(raw) {
		return "", fmt.Errorf("base-url must be a valid HTTP or HTTPS URL")
	}

	return raw, nil
}
