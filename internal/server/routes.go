package server

import (
	"log/slog"

	"github.com/en9inerd/go-pkgs/middleware"
	"github.com/en9inerd/go-pkgs/router"
	"github.com/en9inerd/shhh/internal/channel"
	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
)

func registerRoutes(
	apiGroup *router.Group,
	logger *slog.Logger,
	cfg *config.Config,
	memStore *memstore.MemoryStore,
) {
	apiGroup.Use(
		middleware.RateLimit(middleware.RateLimitConfig{RPS: 10, Burst: 20}),
		middleware.Logger(logger),
	)
	apiGroup.HandleFunc("POST /secret", saveSecret(logger, cfg, memStore))
	apiGroup.HandleFunc("POST /file", uploadFile(logger, cfg, memStore))
	apiGroup.HandleFunc("POST /secret/{id}", retrieveSecret(logger, memStore))
	apiGroup.HandleFunc("GET /params", getParams(logger, cfg))
}

func registerChannelRoutes(
	apiGroup *router.Group,
	logger *slog.Logger,
	cfg *config.Config,
	cs *channel.ChannelStore,
) {
	apiGroup.Use(
		middleware.RateLimit(middleware.RateLimitConfig{RPS: 10, Burst: 20}),
		middleware.Logger(logger),
	)
	apiGroup.HandleFunc("PUT /channel/{id}", channelPush(cs))
	apiGroup.HandleFunc("GET /channel/{id}", channelPull(logger, cs, cfg))
}

func registerWebRoutes(
	webGroup *router.Group,
	logger *slog.Logger,
	cfg *config.Config,
	memStore *memstore.MemoryStore,
	templates *templateCache,
) {
	webGroup.Use(
		middleware.RateLimit(middleware.RateLimitConfig{RPS: 20, Burst: 30}),
		middleware.Logger(logger),
		middleware.StripSlashes,
	)
	webGroup.HandleFunc("GET /", homePage(logger, cfg, templates))
	webGroup.HandleFunc("GET /secret/{id}", retrievePage(logger, templates))
	webGroup.HandleFunc("POST /web/secret", createTextSecretWeb(logger, cfg, memStore, templates))
	webGroup.HandleFunc("POST /web/file", createFileSecretWeb(logger, cfg, memStore, templates))
	webGroup.HandleFunc("POST /web/retrieve", retrieveSecretWeb(logger, memStore, templates))
}
