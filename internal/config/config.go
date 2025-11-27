package config

import (
	"flag"
	"strconv"
	"time"
)

type Config struct {
	Port          string
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
	maxPhraseSize := fs.Int("max-phrase-size", getEnvInt("SHHH_MAX_PHRASE_SIZE", 16), "Max passphrase size")
	maxItems := fs.Int("max-items", getEnvInt("SHHH_MAX_ITEMS", 100), "Max number of items in memory")
	maxFileSize := fs.Int64("max-file-size", getEnvInt64("SHHH_MAX_FILE_SIZE", 2*1024*1024), "Max file size in bytes")
	maxRetention := fs.Duration("max-retention", getEnvDuration("SHHH_MAX_RETENTION", 24*time.Hour), "Max retention time")

	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}

	return &Config{
		Port:          *port,
		MaxPhraseSize: *maxPhraseSize,
		MaxItems:      *maxItems,
		MaxFileSize:   *maxFileSize,
		MaxRetention:  *maxRetention,
	}, nil
}
