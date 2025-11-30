package server

import (
	"log/slog"

	"github.com/en9inerd/go-pkgs/router"
	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
)

func registerRoutes(
	apiGroup *router.Group,
	logger *slog.Logger,
	cfg *config.Config,
	memStore *memstore.MemoryStore,
) {
	apiGroup.Use(Logger(logger))
	apiGroup.HandleFunc("POST /secret", saveSecret(logger, cfg, memStore))
	apiGroup.HandleFunc("POST /file", uploadFile(logger, cfg, memStore))
	apiGroup.HandleFunc("GET /secret/{id}/{passphrase}", retrieveSecret(logger, memStore))
	apiGroup.HandleFunc("GET /params", getParams(logger, cfg))
}
