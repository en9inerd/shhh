package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/en9inerd/shhh/internal/channel"
	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
)

const testUUID = "a1b2c3d4e5f678901234567890abcdef"

// testChannelSetup returns a server wired with one channel (testUUID) and its store.
func testChannelSetup(t *testing.T, maxMsgs, maxWatchers int) (http.Handler, *channel.ChannelStore) {
	t.Helper()
	cfg, err := config.ParseConfig(func(k string) string { return "" })
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cfg.Channels = []string{testUUID}
	cfg.ChannelMaxMsgs = maxMsgs
	cfg.ChannelMaxWatchers = maxWatchers
	cfg.ChannelMsgTTL = time.Hour

	cs := channel.NewChannelStore(cfg.Channels, cfg.ChannelMaxMsgs, cfg.ChannelMaxWatchers, cfg.ChannelMsgTTL)
	t.Cleanup(cs.Stop)

	store := memstore.NewMemoryStore(testLogger(), cfg.MaxRetention, cfg.MaxItems, cfg.MaxFileSize)
	t.Cleanup(store.Stop)

	h, err := NewServer(testLogger(), cfg, store, cs)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return h, cs
}

// minBlobBytes returns a valid minimum blob for push tests.
func minBlobBytes(t *testing.T) []byte {
	t.Helper()
	plain := make([]byte, channel.MinInnerSize) // 18 bytes: msg_id(16)+type(1)+name_len(1)
	plain[16] = 0x01                            // type = text
	plain[17] = 0x00                            // no name
	blob, err := channel.EncryptBlob(plain, "pass", testUUID)
	if err != nil {
		t.Fatalf("EncryptBlob: %v", err)
	}
	return blob
}

func pushRaw(t *testing.T, h http.Handler, url string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// --- Push tests ---

func TestChannelPush_valid(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	w := pushRaw(t, h, "/api/channel/"+testUUID, minBlobBytes(t))
	if w.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestChannelPush_malformedUUID(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	w := pushRaw(t, h, "/api/channel/NOTVALID32CHARHEXUUUUUUUUUUUUUU", minBlobBytes(t))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
}

func TestChannelPush_unknownUUID(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	w := pushRaw(t, h, "/api/channel/00000000000000000000000000000000", minBlobBytes(t))
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", w.Code)
	}
}

func TestChannelPush_blobTooShort(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	w := pushRaw(t, h, "/api/channel/"+testUUID, make([]byte, 10))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
}

func TestChannelPush_queueFull(t *testing.T) {
	h, _ := testChannelSetup(t, 1, 10) // maxMsgs=1
	if code := pushRaw(t, h, "/api/channel/"+testUUID, minBlobBytes(t)).Code; code != http.StatusNoContent {
		t.Fatalf("first push: got %d", code)
	}
	if code := pushRaw(t, h, "/api/channel/"+testUUID, minBlobBytes(t)).Code; code != http.StatusTooManyRequests {
		t.Fatalf("second push (full): got %d, want 429", code)
	}
}

func TestChannelPush_emptyBody(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	w := pushRaw(t, h, "/api/channel/"+testUUID, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
}

// --- Pull tests ---

func TestChannelPull_empty(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	req := httptest.NewRequest(http.MethodGet, "/api/channel/"+testUUID, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	var resp struct {
		Messages []any `json:"messages"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(resp.Messages))
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestChannelPull_afterPush(t *testing.T) {
	h, cs := testChannelSetup(t, 20, 10)
	ch, _ := cs.Get(testUUID)
	rawBlob := make([]byte, channel.MinBlobSize)
	rawBlob[0] = 0x01
	ch.Push(rawBlob)

	req := httptest.NewRequest(http.MethodGet, "/api/channel/"+testUUID, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	var resp struct {
		Messages []struct {
			Blob     string `json:"blob"`
			PushedAt string `json:"pushed_at"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Blob == "" {
		t.Error("blob is empty")
	}
	if _, err := time.Parse(time.RFC3339, resp.Messages[0].PushedAt); err != nil {
		t.Errorf("pushed_at not RFC3339: %s", resp.Messages[0].PushedAt)
	}
}

func TestChannelPull_unknownUUID(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	req := httptest.NewRequest(http.MethodGet, "/api/channel/00000000000000000000000000000000", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", w.Code)
	}
}

func TestChannelPull_invalidLimit(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)

	for _, qs := range []string{"limit=0", "limit=9999", "limit=-1", "limit=abc"} {
		req := httptest.NewRequest(http.MethodGet, "/api/channel/"+testUUID+"?"+qs, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("?%s: got %d, want 400", qs, w.Code)
		}
	}
}

// --- Watch tests ---
// SSE requires a real HTTP connection (middleware wrappers strip http.Flusher
// from httptest.ResponseRecorder). These tests use httptest.NewServer.

// sseScanner reads SSE lines until it finds the given event type, then calls
// cancel. Returns the data field, or "" on timeout/EOF. A single *bufio.Scanner
// must be shared across consecutive calls on the same connection so the internal
// buffer is not lost between calls.
func readSSEUntilEvent(t *testing.T, sc *bufio.Scanner, wantEvent string, cancel context.CancelFunc) string {
	t.Helper()
	var event, data string
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if event == wantEvent {
				cancel()
				return data
			}
			event, data = "", ""
			continue
		}
		if after, ok := strings.CutPrefix(line, "event: "); ok {
			event = after
		} else if after, ok := strings.CutPrefix(line, "data: "); ok {
			data = after
		}
	}
	return ""
}

func TestChannelWatch_sseHeaders(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/channel/"+testUUID+"/watch", nil)
	resp, err := ts.Client().Do(req)
	if err != nil && ctx.Err() == nil {
		t.Fatalf("do: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream…", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	// Wait for the connected event, then cancel.
	sc := bufio.NewScanner(resp.Body)
	got := readSSEUntilEvent(t, sc, "connected", cancel)
	if got != "{}" {
		t.Errorf("connected data = %q, want {}", got)
	}
}

func TestChannelWatch_snapshotOnConnect(t *testing.T) {
	h, cs := testChannelSetup(t, 20, 10)
	ch, _ := cs.Get(testUUID)
	rawBlob := make([]byte, channel.MinBlobSize)
	rawBlob[0] = 0x01
	ch.Push(rawBlob)

	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/channel/"+testUUID+"/watch", nil)
	resp, err := ts.Client().Do(req)
	if err != nil && ctx.Err() == nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	// We expect the connected event first, then a message event from the snapshot.
	// Share one scanner so its internal buffer isn't lost between calls.
	sc := bufio.NewScanner(resp.Body)
	readSSEUntilEvent(t, sc, "connected", func() {})
	got := readSSEUntilEvent(t, sc, "message", cancel)
	if got == "" {
		t.Error("expected message event in snapshot")
	}
}

func TestChannelWatch_unknownUUID(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	resp, err := ts.Client().Get(ts.URL + "/api/channel/00000000000000000000000000000000/watch")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestChannelWatch_malformedUUID(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	resp, err := ts.Client().Get(ts.URL + "/api/channel/NOTAUUIDNOTAUUIDNOTAUUIDNOTAUUID/watch")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestChannelWatch_watcherCap(t *testing.T) {
	_, cs := testChannelSetup(t, 20, 1) // maxWatchers=1

	// Fill the one slot directly.
	ch, _ := cs.Get(testUUID)
	sub, _, ok := ch.Subscribe(20)
	if !ok {
		t.Fatal("first subscribe failed")
	}
	defer ch.Unsubscribe(sub)

	// Build server using the same ChannelStore (slot already full).
	cfg, _ := config.ParseConfig(func(k string) string { return "" })
	cfg.Channels = []string{testUUID}
	cfg.ChannelMaxMsgs = 20
	cfg.ChannelMaxWatchers = 1
	cfg.ChannelMsgTTL = time.Hour
	store := memstore.NewMemoryStore(testLogger(), cfg.MaxRetention, cfg.MaxItems, cfg.MaxFileSize)
	t.Cleanup(store.Stop)
	h2, err := NewServer(testLogger(), cfg, store, cs)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(h2)
	t.Cleanup(ts.Close)

	resp, err := ts.Client().Get(ts.URL + "/api/channel/" + testUUID + "/watch")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429 (watcher cap hit)", resp.StatusCode)
	}
}

// --- clientIP tests ---

func TestClientIP(t *testing.T) {
	cases := []struct{ addr, want string }{
		{"192.0.2.1:1234", "192.0.2.1"},
		{"192.0.2.1", "192.0.2.1"}, // bare IP, no port
		{"[::1]:8080", "::1"},      // IPv6 with port
	}
	for _, tc := range cases {
		if got := clientIP(tc.addr); got != tc.want {
			t.Errorf("clientIP(%q) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

// --- Per-IP connection cap test ---

func TestWatchConnPerIP_limit(t *testing.T) {
	var once sync.Once
	ready := make(chan struct{})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(ready) }) // signal: first request is in handler
		<-r.Context().Done()
	})

	ts := httptest.NewServer(watchConnPerIP(1)(inner))
	t.Cleanup(ts.Close)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	errCh := make(chan error, 1)
	go func() {
		req, _ := http.NewRequestWithContext(ctx1, http.MethodGet, ts.URL, nil)
		_, err := ts.Client().Do(req)
		errCh <- err
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not arrive")
	}

	// Second request from same IP must get 429.
	resp2, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Fatalf("second request error: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Errorf("second request: got %d, want 429", resp2.StatusCode)
	}

	cancel1()
	<-errCh
}

// --- Additional push/pull coverage ---

func TestChannelPull_validLimit(t *testing.T) {
	h, cs := testChannelSetup(t, 20, 10)
	ch, _ := cs.Get(testUUID)
	for range 3 {
		blob := make([]byte, channel.MinBlobSize)
		blob[0] = 0x01
		ch.Push(blob)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channel/"+testUUID+"?limit=2", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	var resp struct {
		Messages []any `json:"messages"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages (limit=2 of 3), got %d", len(resp.Messages))
	}
}

// --- Channel page tests ---

func TestChannelPage_validChannel(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	req := httptest.NewRequest(http.MethodGet, "/channel/"+testUUID, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestChannelPage_unknownUUID(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	req := httptest.NewRequest(http.MethodGet, "/channel/00000000000000000000000000000000", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", w.Code)
	}
}

func TestChannelPage_invalidUUID(t *testing.T) {
	h, _ := testChannelSetup(t, 20, 10)
	// Uppercase hex — valid URL chars but not a valid channel UUID.
	req := httptest.NewRequest(http.MethodGet, "/channel/ABCDEFABCDEFABCDEFABCDEFABCDEF12", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", w.Code)
	}
}

// --- Config validation tests ---

func TestConfig_InvalidChannelUUID(t *testing.T) {
	_, err := config.ParseConfig(func(k string) string {
		if k == "SHHH_CHANNELS" {
			return "NOTVALID"
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected error for invalid channel UUID in config")
	}
}

func TestConfig_DuplicateChannelUUID(t *testing.T) {
	uuid := "a1b2c3d4e5f678901234567890abcdef"
	_, err := config.ParseConfig(func(k string) string {
		if k == "SHHH_CHANNELS" {
			return uuid + "," + uuid
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected error for duplicate channel UUID")
	}
}

func TestConfig_WhitespaceTrimmedUUID(t *testing.T) {
	uuid := "a1b2c3d4e5f678901234567890abcdef"
	cfg, err := config.ParseConfig(func(k string) string {
		if k == "SHHH_CHANNELS" {
			return " " + uuid + " "
		}
		return ""
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Channels) != 1 || cfg.Channels[0] != uuid {
		t.Fatalf("channels = %v, want [%s]", cfg.Channels, uuid)
	}
}

func TestConfig_ChannelMsgTTLZeroFallback(t *testing.T) {
	cfg, err := config.ParseConfig(func(k string) string {
		if k == "SHHH_CHANNEL_MSG_TTL" {
			return "0s"
		}
		if k == "SHHH_MAX_RETENTION" {
			return "6h"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChannelMsgTTL != 6*time.Hour {
		t.Errorf("ChannelMsgTTL = %v, want 6h (fallback to MaxRetention)", cfg.ChannelMsgTTL)
	}
}
