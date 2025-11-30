package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/en9inerd/go-pkgs/httpjson"
	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
	"github.com/en9inerd/shhh/internal/validator"
)

type saveSecretRequest struct {
	Secret     string `json:"secret"`
	Exp        int    `json:"exp"`
	PassPhrase string `json:"passphrase"`
	validator.Validator
}

func (r *saveSecretRequest) Validate(v *validator.Validator, cfg *config.Config) {
	v.CheckField(validator.NotBlank(r.Secret), "secret", "secret is required")
	v.CheckField(validator.MaxChars(r.Secret, int(cfg.MaxFileSize)), "secret", "secret exceeds maximum size")
	v.CheckField(validator.NotBlank(r.PassPhrase), "passphrase", "passphrase is required")
	v.CheckField(validator.MaxChars(r.PassPhrase, cfg.MaxPhraseSize), "passphrase", "passphrase exceeds maximum size")
	v.CheckField(validator.MinInt(r.Exp, 1), "exp", "expiration must be at least 1 second")
}

func saveSecret(l *slog.Logger, cfg *config.Config, memStore *memstore.MemoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req saveSecretRequest

		if err := httpjson.DecodeJSON(r, &req); err != nil {
			l.Warn("can't bind request", "error", err)
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "can't decode request")
			return
		}

		req.Validate(&req.Validator, cfg)
		if !req.Valid() {
			l.Warn("validation failed", "errors", req.FieldErrors)
			w.WriteHeader(http.StatusBadRequest)
			httpjson.WriteJSON(w, httpjson.JSON{"errors": req.FieldErrors})
			return
		}

		ttl := time.Duration(req.Exp) * time.Second
		if ttl > cfg.MaxRetention {
			ttl = cfg.MaxRetention
		}

		id, storedItem, err := memStore.Store([]byte(req.Secret), "", req.PassPhrase, ttl)
		if err != nil {
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "can't create secret")
			return
		}

		w.WriteHeader(http.StatusCreated)
		httpjson.WriteJSON(w, httpjson.JSON{
			"key": id,
			"exp": req.Exp,
		})
		l.Info("created secret", "id", id, "expires_at", storedItem.ExpiresAt.Format(time.RFC3339))
	}
}

func retrieveSecret(l *slog.Logger, memStore *memstore.MemoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		passphrase := r.PathValue("passphrase")

		if id == "" || passphrase == "" {
			l.Warn("missing id or passphrase")
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, errors.New("id and passphrase are required"), "id and passphrase are required")
			return
		}

		data, filename, err := memStore.Retrieve(id, passphrase)
		if err != nil {
			l.Warn("secret retrieval failed", "id", id)
			httpjson.SendErrorJSON(w, r, l, http.StatusNotFound, errors.New("secret not found"), "secret not found")
			return
		}

		if filename != "" {
			// Escape quotes for Content-Disposition header
			safeFilename := strings.ReplaceAll(filename, `"`, `\"`)
			encodedFilename := url.QueryEscape(filename)

			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safeFilename, encodedFilename))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			l.Info("retrieved file", "id", id, "filename", filename)
			return
		}

		response := httpjson.JSON{
			"secret": string(data),
		}

		httpjson.WriteJSON(w, response)
		l.Info("retrieved secret", "id", id)
	}
}

func uploadFile(l *slog.Logger, cfg *config.Config, memStore *memstore.MemoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(cfg.MaxFileSize + 10240)
		if err != nil {
			l.Warn("can't parse multipart form", "error", err)
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "can't parse multipart form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			l.Warn("can't get file from form", "error", err)
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "file is required")
			return
		}
		defer file.Close()

		fileData, err := io.ReadAll(file)
		if err != nil {
			l.Warn("can't read file data", "error", err)
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "can't read file data")
			return
		}

		passphrase := r.FormValue("passphrase")
		if passphrase == "" {
			l.Warn("passphrase is required")
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, errors.New("passphrase is required"), "passphrase is required")
			return
		}

		if len(passphrase) > cfg.MaxPhraseSize {
			l.Warn("passphrase exceeds maximum size")
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, errors.New("passphrase exceeds maximum size"), "passphrase exceeds maximum size")
			return
		}

		expStr := r.FormValue("exp")
		if expStr == "" {
			l.Warn("expiration is required")
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, errors.New("expiration is required"), "expiration is required")
			return
		}

		exp, err := strconv.Atoi(expStr)
		if err != nil || exp < 1 {
			l.Warn("invalid expiration value", "exp", expStr)
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, errors.New("expiration must be at least 1 second"), "expiration must be at least 1 second")
			return
		}

		ttl := time.Duration(exp) * time.Second
		if ttl > cfg.MaxRetention {
			ttl = cfg.MaxRetention
		}

		filename := header.Filename
		if filename == "" {
			filename = r.FormValue("filename")
		}

		id, storedItem, err := memStore.Store(fileData, filename, passphrase, ttl)
		if err != nil {
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "can't store file")
			return
		}

		w.WriteHeader(http.StatusCreated)
		httpjson.WriteJSON(w, httpjson.JSON{
			"key":      id,
			"exp":      exp,
			"filename": storedItem.Filename,
		})
		l.Info("uploaded file", "id", id, "filename", storedItem.Filename, "expires_at", storedItem.ExpiresAt.Format(time.RFC3339))
	}
}

func getParams(l *slog.Logger, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpjson.WriteJSON(w, httpjson.JSON{
			"max_phrase_size": cfg.MaxPhraseSize,
			"max_items":       cfg.MaxItems,
			"max_file_size":   cfg.MaxFileSize,
			"max_retention":   int(cfg.MaxRetention.Seconds()),
		})
		l.Debug("params requested")
	}
}
