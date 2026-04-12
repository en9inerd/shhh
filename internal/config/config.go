package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/en9inerd/go-pkgs/validator"
	"github.com/en9inerd/shhh/internal/channel"
)

type Config struct {
	Port          string
	BaseURL       string
	CORSOrigin    string
	MinPhraseSize int
	MaxPhraseSize int
	MaxItems      int
	MaxFileSize   int64
	MaxRetention  time.Duration
	Verbose       bool

	// Channel feature
	Channels           []string
	ChannelMsgTTL      time.Duration
	ChannelMaxMsgs     int
	ChannelMaxWatchers int
	WatchConnPerIP     int
	WatchRPSPerIP      float64
	TrustedProxies     []string
}

func ParseConfig(getenv func(string) string) (*Config, error) {
	parsedBaseURL, err := sanitizeBaseURL(envStr(getenv, "SHHH_BASE_URL", ""))
	if err != nil {
		return nil, err
	}

	channels, err := parseChannels(envStr(getenv, "SHHH_CHANNELS", ""))
	if err != nil {
		return nil, err
	}

	maxRetention := envDuration(getenv, "SHHH_MAX_RETENTION", 24*time.Hour)
	msgTTL := envDuration(getenv, "SHHH_CHANNEL_MSG_TTL", 24*time.Hour)
	if msgTTL <= 0 {
		msgTTL = maxRetention
	}

	return &Config{
		Port:          envStr(getenv, "SHHH_PORT", "8000"),
		BaseURL:       parsedBaseURL,
		CORSOrigin:    envStr(getenv, "SHHH_CORS_ORIGIN", ""),
		MinPhraseSize: envInt(getenv, "SHHH_MIN_PHRASE_SIZE", 5),
		MaxPhraseSize: envInt(getenv, "SHHH_MAX_PHRASE_SIZE", 128),
		MaxItems:      envInt(getenv, "SHHH_MAX_ITEMS", 100),
		MaxFileSize:   envInt64(getenv, "SHHH_MAX_FILE_SIZE", 2*1024*1024),
		MaxRetention:  maxRetention,
		Verbose:       envBool(getenv, "SHHH_VERBOSE", false),

		Channels:           channels,
		ChannelMsgTTL:      msgTTL,
		ChannelMaxMsgs:     envInt(getenv, "SHHH_CHANNEL_MAX_MSGS", 20),
		ChannelMaxWatchers: envInt(getenv, "SHHH_CHANNEL_MAX_WATCHERS", 10),
		WatchConnPerIP:     envInt(getenv, "SHHH_WATCH_CONN_PER_IP", 3),
		WatchRPSPerIP:      envFloat64(getenv, "SHHH_WATCH_RPS_PER_IP", 2.0),
		TrustedProxies:     parseStringList(envStr(getenv, "SHHH_TRUSTED_PROXIES", "")),
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

func envFloat64(getenv func(string) string, key string, fallback float64) float64 {
	if v := getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

// parseChannels parses and validates a comma-separated list of 32-char lowercase
// hex channel UUIDs. Returns an error if any entry is malformed or duplicated.
func parseChannels(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	seen := make(map[string]bool, len(parts))
	result := make([]string, 0, len(parts))
	for _, raw := range parts {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if !channel.IsValidUUID(id) {
			return nil, fmt.Errorf("invalid channel UUID %q: must be 32-character lowercase hex", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate channel UUID %q", id)
		}
		seen[id] = true
		result = append(result, id)
	}
	return result, nil
}

func parseStringList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			result = append(result, v)
		}
	}
	return result
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
