package server

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/en9inerd/go-pkgs/middleware"
	"github.com/en9inerd/go-pkgs/router"
	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
	"github.com/en9inerd/shhh/ui"
)

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' 'unsafe-hashes'")
		next.ServeHTTP(w, r)
	})
}

func NewServer(
	logger *slog.Logger,
	cfg *config.Config,
	memStore *memstore.MemoryStore,
) (http.Handler, error) {
	r := router.New(http.NewServeMux())

	maxRequestSize := cfg.MaxFileSize + 10240
	r.Use(
		SecurityHeaders,
		middleware.RealIP,
		middleware.Recoverer(logger, false),
		middleware.GlobalThrottle(1000),
		middleware.Timeout(60*time.Second),
		middleware.Health,
		middleware.SizeLimit(maxRequestSize),
	)

	templates, err := newTemplateCache()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize templates: %w", err)
	}

	staticFS, err := fs.Sub(ui.Files, "static")
	if err != nil {
		return nil, fmt.Errorf("failed to get static subdirectory: %w", err)
	}
	r.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Mount("/api").Route(func(apiGroup *router.Group) {
		registerRoutes(apiGroup, logger, cfg, memStore)
	})

	r.Group().Route(func(webGroup *router.Group) {
		registerWebRoutes(webGroup, logger, cfg, memStore, templates)
	})

	r.NotFoundHandler(notFoundPage(logger, templates))

	return r, nil
}
