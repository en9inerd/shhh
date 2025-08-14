package server

import (
	"log/slog"
	"net/http"

	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
)

func registerRoutes(
	mux *http.ServeMux,
	logger *slog.Logger,
	config *config.Config,
	memStore *memstore.MemoryStore) {

	mux.Handle("/healthz", healthHandler())
}
