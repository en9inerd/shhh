package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testServer(t *testing.T) (http.Handler, *memstore.MemoryStore) {
	t.Helper()
	return testServerWithEnv(t, nil)
}

func testServerWithEnv(t *testing.T, env map[string]string) (http.Handler, *memstore.MemoryStore) {
	t.Helper()
	getenv := func(k string) string {
		if env != nil {
			if v, ok := env[k]; ok {
				return v
			}
		}
		return ""
	}
	cfg, err := config.ParseConfig(getenv)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	store := memstore.NewMemoryStore(testLogger(), cfg.MaxRetention, cfg.MaxItems, cfg.MaxFileSize)
	t.Cleanup(store.Stop)
	h, err := NewServer(testLogger(), cfg, store, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return h, store
}

func postMultipartFile(t *testing.T, ts *httptest.Server, build func(*multipart.Writer) error) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := build(mw); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/file", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestParseExpiration(t *testing.T) {
	got, err := parseExpiration("3600", "")
	if err != nil || got != 3600 {
		t.Errorf("parseExpiration fixed: got %d err %v", got, err)
	}
	_, err = parseExpiration("custom", "")
	if err == nil {
		t.Fatal("custom without value: want error")
	}
	got, err = parseExpiration("custom", "120")
	if err != nil || got != 120 {
		t.Errorf("parseExpiration custom: got %d err %v", got, err)
	}
	_, err = parseExpiration("", "")
	if err == nil {
		t.Fatal("empty: want error")
	}
}

func TestGetExpirationIntervals(t *testing.T) {
	all := getExpirationIntervals(24 * time.Hour)
	if len(all) < 8 {
		t.Fatalf("expected presets + Custom, got %d", len(all))
	}
	if all[len(all)-1].Label != "Custom" {
		t.Errorf("last interval: got %+v", all[len(all)-1])
	}

	short := getExpirationIntervals(2 * time.Hour)
	for _, iv := range short {
		if iv.Seconds > 7200 && iv.Seconds != 0 {
			t.Errorf("interval %s (%ds) should not exceed 2h cap", iv.Label, iv.Seconds)
		}
	}
}

func TestExpirationExceedsMaxSeconds(t *testing.T) {
	max := 24 * time.Hour
	if expirationExceedsMaxSeconds(86401, max) != true {
		t.Error("86401s should exceed 24h")
	}
	if expirationExceedsMaxSeconds(86400, max) != false {
		t.Error("86400s should not exceed 24h")
	}
}

func TestValidatePassphrase(t *testing.T) {
	cfg := &config.Config{MinPhraseSize: 5, MaxPhraseSize: 10}
	if err := validatePassphrase("12345", cfg); err != nil {
		t.Errorf("valid min length: %v", err)
	}
	if err := validatePassphrase("ab", cfg); err == nil {
		t.Fatal("too short: want error")
	}
	if err := validatePassphrase("12345678901", cfg); err == nil {
		t.Fatal("too long: want error")
	}
	if err := validatePassphrase("", cfg); err == nil {
		t.Fatal("empty: want error")
	}
}

func TestBaseURLFromRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	cfg := &config.Config{BaseURL: "https://fixed.example"}
	if got := baseURLFromRequest(r, cfg); got != "https://fixed.example" {
		t.Errorf("explicit BaseURL: got %q", got)
	}

	r2 := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	if got := baseURLFromRequest(r2, &config.Config{}); got != "http://example.com" {
		t.Errorf("inferred http: got %q", got)
	}

	r3 := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	r3.TLS = new(tls.ConnectionState)
	if got := baseURLFromRequest(r3, &config.Config{}); got != "https://example.com" {
		t.Errorf("TLS: got %q", got)
	}

	r4 := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	r4.Header.Set("X-Forwarded-Proto", "https")
	if got := baseURLFromRequest(r4, &config.Config{}); got != "https://example.com" {
		t.Errorf("X-Forwarded-Proto: got %q", got)
	}
}

func TestFormatExpiresAtDisplay(t *testing.T) {
	ts := time.Date(2026, 4, 9, 15, 4, 5, 0, time.UTC)
	got := formatExpiresAtDisplay(ts)
	want := "April 9, 2026 at 15:04 UTC"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAPI_SaveSecret_ExpirationExceedsMaxReturns400(t *testing.T) {
	h, _ := testServerWithEnv(t, map[string]string{"SHHH_MAX_RETENTION": "1h"})
	ts := httptest.NewServer(h)
	defer ts.Close()

	body := `{"secret":"hello","exp":999999,"passphrase":"secret123"}`
	resp, err := ts.Client().Post(ts.URL+"/api/secret", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
	}
	var verr struct {
		Errors map[string][]string `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msgs := verr.Errors["exp"]; len(msgs) == 0 {
		t.Fatalf("expected exp field error: %+v", verr.Errors)
	}
}

func TestAPI_SaveAndRetrieveSecret(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	body := `{"secret":"hello","exp":3600,"passphrase":"secret123"}`
	resp, err := ts.Client().Post(ts.URL+"/api/secret", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/secret: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	var created struct {
		Key       string `json:"key"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Key == "" || created.ExpiresAt == "" {
		t.Fatalf("response: %+v", created)
	}
	expAt, err := time.Parse(time.RFC3339, created.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at RFC3339: %v", err)
	}
	if !expAt.After(time.Now().UTC().Add(3500*time.Second)) || !expAt.Before(time.Now().UTC().Add(3700*time.Second)) {
		t.Errorf("expires_at out of range: %v", expAt)
	}

	retBody := `{"passphrase":"secret123"}`
	req2, err := http.NewRequest(http.MethodPost, ts.URL+"/api/secret/"+created.Key, strings.NewReader(retBody))
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := ts.Client().Do(req2)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("retrieve status %d: %s", resp2.StatusCode, b)
	}
	var out struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&out); err != nil {
		t.Fatalf("decode retrieve: %v", err)
	}
	if out.Secret != "hello" {
		t.Errorf("secret: got %q", out.Secret)
	}
}

func TestAPI_RetrieveWrongPassphrase(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	body := `{"secret":"x","exp":3600,"passphrase":"secret123"}`
	resp, err := ts.Client().Post(ts.URL+"/api/secret", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: %d %s", resp.StatusCode, b)
	}
	var created struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	bad := `{"passphrase":"wrongpassphrase"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/secret/"+created.Key, strings.NewReader(bad))
	req.Header.Set("Content-Type", "application/json")
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("wrong passphrase: status %d", resp.StatusCode)
	}
}

func TestAPI_SaveSecret_Validation(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	body := `{"secret":"hi","exp":3600,"passphrase":"x"}`
	resp, err := ts.Client().Post(ts.URL+"/api/secret", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	var verr struct {
		Errors map[string][]string `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(verr.Errors) == 0 {
		t.Fatal("expected field errors")
	}
}

func TestAPI_UploadFile_ContentDisposition(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", `report "final".txt`)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("payload"))
	w.WriteField("passphrase", "secret123")
	w.WriteField("exp", "3600")
	w.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/file", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload: %d %s", resp.StatusCode, b)
	}
	var up struct {
		Key       string `json:"key"`
		ExpiresAt string `json:"expires_at"`
		Filename  string `json:"filename"`
	}
	json.NewDecoder(resp.Body).Decode(&up)

	retBody := `{"passphrase":"secret123"}`
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/secret/"+up.Key, strings.NewReader(retBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := ts.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("retrieve file: %d %s", resp2.StatusCode, b)
	}
	cd := resp2.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "filename") {
		t.Errorf("Content-Disposition: %q", cd)
	}
	if resp2.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("Content-Type: %q", resp2.Header.Get("Content-Type"))
	}
	slurp, _ := io.ReadAll(resp2.Body)
	if string(slurp) != "payload" {
		t.Errorf("body: got %q", slurp)
	}
}

func TestAPI_UploadFile_ExpirationExceedsMaxReturns400(t *testing.T) {
	h, _ := testServerWithEnv(t, map[string]string{"SHHH_MAX_RETENTION": "1h"})
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp := postMultipartFile(t, ts, func(mw *multipart.Writer) error {
		fw, err := mw.CreateFormFile("file", "a.txt")
		if err != nil {
			return err
		}
		if _, err := fw.Write([]byte("x")); err != nil {
			return err
		}
		if err := mw.WriteField("passphrase", "secret123"); err != nil {
			return err
		}
		return mw.WriteField("exp", "999999")
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
	}
}

func TestAPI_UploadFile_NotMultipart(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := ts.Client().Post(ts.URL+"/api/file", "text/plain", strings.NewReader("not a multipart body"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
	}
}

func TestAPI_UploadFile_MissingFileField(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp := postMultipartFile(t, ts, func(mw *multipart.Writer) error {
		if err := mw.WriteField("passphrase", "secret123"); err != nil {
			return err
		}
		return mw.WriteField("exp", "3600")
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
	}
}

func TestAPI_UploadFile_MissingPassphrase(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp := postMultipartFile(t, ts, func(mw *multipart.Writer) error {
		fw, err := mw.CreateFormFile("file", "a.txt")
		if err != nil {
			return err
		}
		if _, err := fw.Write([]byte("x")); err != nil {
			return err
		}
		return mw.WriteField("exp", "3600")
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
	}
}

func TestAPI_UploadFile_ShortPassphrase(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp := postMultipartFile(t, ts, func(mw *multipart.Writer) error {
		fw, err := mw.CreateFormFile("file", "a.txt")
		if err != nil {
			return err
		}
		if _, err := fw.Write([]byte("x")); err != nil {
			return err
		}
		if err := mw.WriteField("passphrase", "bad"); err != nil {
			return err
		}
		return mw.WriteField("exp", "3600")
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
	}
}

func TestAPI_UploadFile_MissingExpiration(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp := postMultipartFile(t, ts, func(mw *multipart.Writer) error {
		fw, err := mw.CreateFormFile("file", "a.txt")
		if err != nil {
			return err
		}
		if _, err := fw.Write([]byte("x")); err != nil {
			return err
		}
		return mw.WriteField("passphrase", "secret123")
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
	}
}

func TestAPI_UploadFile_InvalidExpiration(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	t.Run("zero", func(t *testing.T) {
		resp := postMultipartFile(t, ts, func(mw *multipart.Writer) error {
			fw, err := mw.CreateFormFile("file", "a.txt")
			if err != nil {
				return err
			}
			if _, err := fw.Write([]byte("x")); err != nil {
				return err
			}
			if err := mw.WriteField("passphrase", "secret123"); err != nil {
				return err
			}
			return mw.WriteField("exp", "0")
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
		}
	})

	t.Run("not a number", func(t *testing.T) {
		resp := postMultipartFile(t, ts, func(mw *multipart.Writer) error {
			fw, err := mw.CreateFormFile("file", "a.txt")
			if err != nil {
				return err
			}
			if _, err := fw.Write([]byte("x")); err != nil {
				return err
			}
			if err := mw.WriteField("passphrase", "secret123"); err != nil {
				return err
			}
			return mw.WriteField("exp", "abc")
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
		}
	})
}

func TestAPI_UploadFile_StoreFull(t *testing.T) {
	h, _ := testServerWithEnv(t, map[string]string{"SHHH_MAX_ITEMS": "1"})
	ts := httptest.NewServer(h)
	defer ts.Close()

	upload := func() *http.Response {
		return postMultipartFile(t, ts, func(mw *multipart.Writer) error {
			fw, err := mw.CreateFormFile("file", "a.txt")
			if err != nil {
				return err
			}
			if _, err := fw.Write([]byte("one")); err != nil {
				return err
			}
			if err := mw.WriteField("passphrase", "secret123"); err != nil {
				return err
			}
			return mw.WriteField("exp", "3600")
		})
	}

	r1 := upload()
	defer r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(r1.Body)
		t.Fatalf("first upload: %d %s", r1.StatusCode, b)
	}

	r2 := upload()
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(r2.Body)
		t.Fatalf("second upload when full: want 400, got %d: %s", r2.StatusCode, b)
	}
}

func TestAPI_SaveSecret_InvalidJSON(t *testing.T) {
	h, _ := testServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := ts.Client().Post(ts.URL+"/api/secret", "application/json", strings.NewReader("{not-json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400, got %d: %s", resp.StatusCode, b)
	}
}
