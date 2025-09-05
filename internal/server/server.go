package server

import (
	"log/slog"
	"net/http"

	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
)

func NewServer(
	logger *slog.Logger,
	config *config.Config,
	memStore *memstore.MemoryStore,
) http.Handler {
	mux := http.NewServeMux()

	registerRoutes(mux, logger, config, memStore)

	var handler http.Handler = mux

	// Middlewares

	return handler
}
