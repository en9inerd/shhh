package server

import (
	"errors"
	"log/slog"
	"net/http"
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

func (r *saveSecretRequest) Validate(v *validator.Validator) {
	v.CheckField(validator.NotBlank(r.Secret), "secret", "secret is required")
	v.CheckField(validator.NotBlank(r.PassPhrase), "passphrase", "passphrase is required")
	v.CheckField(validator.MinChars(r.PassPhrase, 1), "passphrase", "passphrase cannot be empty")
	v.CheckField(validator.MinInt(r.Exp, 1), "exp", "expiration must be at least 1 second")
}

func saveSecret(l *slog.Logger, cfg *config.Config, memStore *memstore.MemoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req saveSecretRequest

		if err := httpjson.DecodeJSON(r, &req); err != nil {
			l.Warn("can't bind request %v", req)
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "can't decode request")
			return
		}

		req.Validate(&req.Validator)
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
			if err.Error() == "item not found" {
				l.Warn("secret not found", "id", id)
				httpjson.SendErrorJSON(w, r, l, http.StatusNotFound, err, "secret not found")
				return
			}
			if err.Error() == "item expired" {
				l.Warn("secret expired", "id", id)
				httpjson.SendErrorJSON(w, r, l, http.StatusGone, err, "secret expired")
				return
			}
			if err.Error() == "decryption failed" {
				l.Warn("decryption failed", "id", id)
				httpjson.SendErrorJSON(w, r, l, http.StatusUnauthorized, err, "invalid passphrase")
				return
			}
			l.Error("failed to retrieve secret", "id", id, "error", err)
			httpjson.SendErrorJSON(w, r, l, http.StatusInternalServerError, err, "failed to retrieve secret")
			return
		}

		response := httpjson.JSON{
			"secret": string(data),
		}
		if filename != "" {
			response["filename"] = filename
		}

		httpjson.WriteJSON(w, response)
		l.Info("retrieved secret", "id", id)
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
