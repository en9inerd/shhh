package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/en9inerd/go-pkgs/middleware"
	"github.com/en9inerd/go-pkgs/router"
	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
)

func NewServer(
	logger *slog.Logger,
	cfg *config.Config,
	memStore *memstore.MemoryStore,
) http.Handler {
	r := router.New(http.NewServeMux())

	// global middleware
	r.Use(
		middleware.RealIP,
		middleware.Recoverer(logger),
		middleware.GlobalThrottle(1000),
		middleware.Timeout(60*time.Second),
		middleware.Health,
		middleware.SizeLimit(64*1024),
	)

	r.Mount("/api").Route(func(g *router.Group) {
		handleGroup(g, logger, cfg, memStore)
	})

	// registerRoutes(mux, logger, cfg, memStore)

	// var handler http.Handler = mux

	// Middlewares

	return handler
}
