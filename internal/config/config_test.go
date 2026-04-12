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
	if cfg.CORSOrigin != "" {
		t.Errorf("CORSOrigin: got %q want empty", cfg.CORSOrigin)
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

// --- Channel config tests ---

func TestParseConfig_ChannelSettings(t *testing.T) {
	env := map[string]string{
		"SHHH_CHANNELS":             "a1b2c3d4e5f678901234567890abcdef,00000000000000000000000000000000",
		"SHHH_CHANNEL_MAX_MSGS":     "30",
		"SHHH_CHANNEL_MAX_WATCHERS": "5",
		"SHHH_CHANNEL_MSG_TTL":      "12h",
		"SHHH_WATCH_CONN_PER_IP":    "2",
		"SHHH_WATCH_RPS_PER_IP":     "1.5",
		"SHHH_TRUSTED_PROXIES":      "10.0.0.1, 10.0.0.2",
	}
	cfg, err := ParseConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.Channels) != 2 {
		t.Errorf("Channels: got %d, want 2", len(cfg.Channels))
	}
	if cfg.ChannelMaxMsgs != 30 {
		t.Errorf("ChannelMaxMsgs: got %d, want 30", cfg.ChannelMaxMsgs)
	}
	if cfg.ChannelMaxWatchers != 5 {
		t.Errorf("ChannelMaxWatchers: got %d, want 5", cfg.ChannelMaxWatchers)
	}
	if cfg.ChannelMsgTTL != 12*time.Hour {
		t.Errorf("ChannelMsgTTL: got %v, want 12h", cfg.ChannelMsgTTL)
	}
	if cfg.WatchConnPerIP != 2 {
		t.Errorf("WatchConnPerIP: got %d, want 2", cfg.WatchConnPerIP)
	}
	if cfg.WatchRPSPerIP != 1.5 {
		t.Errorf("WatchRPSPerIP: got %f, want 1.5", cfg.WatchRPSPerIP)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "10.0.0.1" || cfg.TrustedProxies[1] != "10.0.0.2" {
		t.Errorf("TrustedProxies: got %v", cfg.TrustedProxies)
	}
}

func TestParseConfig_InvalidChannelUUIDs(t *testing.T) {
	cases := []string{
		"NOTVALID",                          // too short
		"ABCDEFABCDEFABCDEFABCDEFABCDEF12",  // uppercase
		"a1b2c3d4e5f678901234567890abcde",   // 31 chars
		"a1b2c3d4e5f678901234567890abcdeff", // 33 chars
		"a1b2c3d4e5f678901234567890abcdeg",  // invalid char 'g'
	}
	for _, bad := range cases {
		_, err := ParseConfig(func(k string) string {
			if k == "SHHH_CHANNELS" {
				return bad
			}
			return ""
		})
		if err == nil {
			t.Errorf("expected error for invalid channel UUID %q", bad)
		}
	}
}

func TestParseConfig_DuplicateChannel(t *testing.T) {
	uuid := "a1b2c3d4e5f678901234567890abcdef"
	_, err := ParseConfig(func(k string) string {
		if k == "SHHH_CHANNELS" {
			return uuid + "," + uuid
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected error for duplicate channel UUID")
	}
}

func TestParseConfig_EmptyChannels(t *testing.T) {
	cfg, err := ParseConfig(func(string) string { return "" })
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Channels != nil {
		t.Errorf("Channels: got %v, want nil", cfg.Channels)
	}
}

func TestParseStringList(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a,b,c", []string{"a", "b", "c"}},
		{"  a , b , c  ", []string{"a", "b", "c"}},
		{",a,,b,", []string{"a", "b"}},
	}
	for _, tc := range cases {
		got := parseStringList(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("parseStringList(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i, v := range tc.want {
			if got[i] != v {
				t.Errorf("parseStringList(%q)[%d] = %q, want %q", tc.input, i, got[i], v)
			}
		}
	}
}
