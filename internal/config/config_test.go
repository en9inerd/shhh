package config

import (
	"testing"
	"time"
)

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := ParseConfig(func(string) string { return "" })
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Port != "8000" {
		t.Errorf("Port: got %q want %q", cfg.Port, "8000")
	}
	if cfg.BaseURL != "" {
		t.Errorf("BaseURL: got %q want empty", cfg.BaseURL)
	}
	if cfg.CORSOrigin != "*" {
		t.Errorf("CORSOrigin: got %q want %q", cfg.CORSOrigin, "*")
	}
	if cfg.MinPhraseSize != 5 {
		t.Errorf("MinPhraseSize: got %d want 5", cfg.MinPhraseSize)
	}
	if cfg.MaxPhraseSize != 128 {
		t.Errorf("MaxPhraseSize: got %d want 128", cfg.MaxPhraseSize)
	}
	if cfg.MaxItems != 100 {
		t.Errorf("MaxItems: got %d want 100", cfg.MaxItems)
	}
	wantFile := int64(2 * 1024 * 1024)
	if cfg.MaxFileSize != wantFile {
		t.Errorf("MaxFileSize: got %d want %d", cfg.MaxFileSize, wantFile)
	}
	if cfg.MaxRetention != 24*time.Hour {
		t.Errorf("MaxRetention: got %v want 24h", cfg.MaxRetention)
	}
	if cfg.Verbose {
		t.Error("Verbose: want false")
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	env := map[string]string{
		"SHHH_PORT":            "9000",
		"SHHH_BASE_URL":        "https://secrets.example.com/path",
		"SHHH_CORS_ORIGIN":     "https://app.example.com",
		"SHHH_MIN_PHRASE_SIZE": "8",
		"SHHH_MAX_PHRASE_SIZE": "64",
		"SHHH_MAX_ITEMS":       "50",
		"SHHH_MAX_FILE_SIZE":   "1048576",
		"SHHH_MAX_RETENTION":   "48h",
		"SHHH_VERBOSE":         "true",
	}
	getenv := func(k string) string { return env[k] }

	cfg, err := ParseConfig(getenv)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Port != "9000" {
		t.Errorf("Port: got %q", cfg.Port)
	}
	if cfg.BaseURL != "https://secrets.example.com/path" {
		t.Errorf("BaseURL: got %q", cfg.BaseURL)
	}
	if cfg.CORSOrigin != "https://app.example.com" {
		t.Errorf("CORSOrigin: got %q", cfg.CORSOrigin)
	}
	if cfg.MinPhraseSize != 8 || cfg.MaxPhraseSize != 64 {
		t.Errorf("phrase sizes: min=%d max=%d", cfg.MinPhraseSize, cfg.MaxPhraseSize)
	}
	if cfg.MaxItems != 50 {
		t.Errorf("MaxItems: got %d", cfg.MaxItems)
	}
	if cfg.MaxFileSize != 1048576 {
		t.Errorf("MaxFileSize: got %d", cfg.MaxFileSize)
	}
	if cfg.MaxRetention != 48*time.Hour {
		t.Errorf("MaxRetention: got %v", cfg.MaxRetention)
	}
	if !cfg.Verbose {
		t.Error("Verbose: want true")
	}
}

func TestParseConfig_InvalidBaseURL(t *testing.T) {
	env := map[string]string{"SHHH_BASE_URL": "not-a-valid-http-url"}
	getenv := func(k string) string { return env[k] }
	_, err := ParseConfig(getenv)
	if err == nil {
		t.Fatal("expected error for invalid SHHH_BASE_URL")
	}
}

func TestParseConfig_BaseURLTrimsTrailingSlash(t *testing.T) {
	env := map[string]string{"SHHH_BASE_URL": "https://example.com/foo/"}
	getenv := func(k string) string { return env[k] }
	cfg, err := ParseConfig(getenv)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.BaseURL != "https://example.com/foo" {
		t.Errorf("BaseURL: got %q want trimmed path", cfg.BaseURL)
	}
}

func TestParseConfig_InvalidIntFallsBack(t *testing.T) {
	env := map[string]string{"SHHH_MAX_ITEMS": "not-a-number"}
	getenv := func(k string) string { return env[k] }
	cfg, err := ParseConfig(getenv)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.MaxItems != 100 {
		t.Errorf("MaxItems: invalid env should fall back to 100, got %d", cfg.MaxItems)
	}
}
