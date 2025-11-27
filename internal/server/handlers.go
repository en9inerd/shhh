package server

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/en9inerd/go-pkgs/httpjson"
	"github.com/en9inerd/go-pkgs/router"
	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
)

func handleGroup(apiGroup *router.Group, l *slog.Logger, cfg *config.Config, memStore *memstore.MemoryStore) {
	apiGroup.Use(Logger(l))
	apiGroup.HandleFunc("POST /whisper", saveSecret(l, cfg, memStore))
}

func saveSecret(l *slog.Logger, cfg *config.Config, memStore *memstore.MemoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request := struct {
			Secret     string
			Exp        int
			PassPhrase string
		}{}

		if err := httpjson.DecodeJSON(r, &request); err != nil {
			l.Warn("can't bind request %v", request)
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "can't decode request")
			return
		}

		if len(request.PassPhrase) <= cfg.MaxPhraseSize {
			l.Warn("incorrect passphrase size %d", len(request.PassPhrase))
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, errors.New("incorrect pin size"), "incorrect pin size")
			return
		}

		id, storedItem, err := memStore.Store([]byte(request.Secret), "", request.PassPhrase, time.Second*time.Duration(request.Exp))
		if err != nil {
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "can't create secret")
			return
		}
		w.WriteHeader(http.StatusCreated)
		httpjson.WriteJSON(w, httpjson.JSON{"key": id, "exp": request.Exp})
		l.Info("created secret %s exp %s", id, storedItem.ExpiresAt.Format(time.RFC3339))
	}
}
