package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         string
	MaxItems     int
	MaxFileSize  int64
	MaxRetention time.Duration
}

func ParseConfig(args []string) (*Config, error) {
	fs := flag.NewFlagSet("shhh", flag.ContinueOnError)

	port := fs.String("port", getEnv("SHHH_PORT", "8000"), "Port to listen on")
	maxItems := fs.Int("max-items", getEnvInt("SHHH_MAX_ITEMS", 100), "Max number of items in memory")
	maxFileSize := fs.Int64("max-file-size", getEnvInt64("SHHH_MAX_FILE_SIZE", 2*1024*1024), "Max file size in bytes")
	maxRetention := fs.Duration("max-retention", getEnvDuration("SHHH_MAX_RETENTION", 24*time.Hour), "Max retention time")

	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}

	return &Config{
		Port:         *port,
		MaxItems:     *maxItems,
		MaxFileSize:  *maxFileSize,
		MaxRetention: *maxRetention,
	}, nil
}

// --- Helpers ---

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
