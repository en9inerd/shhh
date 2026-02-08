package config

import (
	"flag"
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
}

func ParseConfig(args []string, getenv func(string) string) (*Config, error) {
	getEnv := func(key, fallback string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return fallback
	}

	getEnvInt := func(key string, fallback int) int {
		if v := getenv(key); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}
		return fallback
	}

	getEnvInt64 := func(key string, fallback int64) int64 {
		if v := getenv(key); v != "" {
			if i, err := strconv.ParseInt(v, 10, 64); err == nil {
				return i
			}
		}
		return fallback
	}

	getEnvDuration := func(key string, fallback time.Duration) time.Duration {
		if v := getenv(key); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				return d
			}
		}
		return fallback
	}

	fs := flag.NewFlagSet("shhh", flag.ContinueOnError)

	port := fs.String("port", getEnv("SHHH_PORT", "8000"), "Port to listen on")
	baseURL := fs.String("base-url", getEnv("SHHH_BASE_URL", ""), "Public base URL (e.g. https://shhh.example.com)")
	minPhraseSize := fs.Int("min-phrase-size", getEnvInt("SHHH_MIN_PHRASE_SIZE", 5), "Min passphrase size")
	maxPhraseSize := fs.Int("max-phrase-size", getEnvInt("SHHH_MAX_PHRASE_SIZE", 128), "Max passphrase size")
	maxItems := fs.Int("max-items", getEnvInt("SHHH_MAX_ITEMS", 100), "Max number of items in memory")
	maxFileSize := fs.Int64("max-file-size", getEnvInt64("SHHH_MAX_FILE_SIZE", 2*1024*1024), "Max file size in bytes")
	maxRetention := fs.Duration("max-retention", getEnvDuration("SHHH_MAX_RETENTION", 24*time.Hour), "Max retention time")

	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}

	parsedBaseURL, err := sanitizeBaseURL(*baseURL)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:          *port,
		BaseURL:       parsedBaseURL,
		MinPhraseSize: *minPhraseSize,
		MaxPhraseSize: *maxPhraseSize,
		MaxItems:      *maxItems,
		MaxFileSize:   *maxFileSize,
		MaxRetention:  *maxRetention,
	}, nil
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
