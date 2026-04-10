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

const multipartOverhead = 10 * 1024 // extra room for form metadata beyond the file payload

func NewServer(
	logger *slog.Logger,
	cfg *config.Config,
	memStore *memstore.MemoryStore,
) (http.Handler, error) {
	r := router.New(http.NewServeMux())

	maxRequestSize := cfg.MaxFileSize + multipartOverhead
	r.Use(
		middleware.Headers(
			"X-Content-Type-Options: nosniff",
			"X-Frame-Options: DENY",
			"Referrer-Policy: no-referrer",
			"Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'",
			"Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()",
			"Cross-Origin-Opener-Policy: same-origin",
		),
		middleware.CORS(middleware.CORSConfig{
			Origin:  cfg.CORSOrigin,
			Methods: []string{"GET", "POST", "OPTIONS"},
			Headers: []string{"Content-Type", "Authorization"},
			MaxAge:  3600,
		}),
		middleware.RealIP,
		middleware.Recoverer(logger, false),
		middleware.Timeout(25*time.Second),
		middleware.Health,
		middleware.GlobalThrottle(100),
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
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))
	r.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		staticHandler.ServeHTTP(w, r)
	}))

	r.Mount("/api").Route(func(apiGroup *router.Group) {
		registerRoutes(apiGroup, logger, cfg, memStore)
	})

	r.Group().Route(func(webGroup *router.Group) {
		registerWebRoutes(webGroup, logger, cfg, memStore, templates)
	})

	r.NotFoundHandler(notFoundPage(logger, templates))

	return r, nil
}
