package server

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/en9inerd/go-pkgs/middleware"
	"github.com/en9inerd/go-pkgs/router"
	"github.com/en9inerd/shhh/internal/channel"
	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
	"github.com/en9inerd/shhh/ui"
)

const multipartOverhead = 10 * 1024 // extra room for form metadata beyond the file payload

func NewServer(
	logger *slog.Logger,
	cfg *config.Config,
	memStore *memstore.MemoryStore,
	channelStore *channel.ChannelStore,
) (http.Handler, error) {
	r := router.New(http.NewServeMux())

	maxRequestSize := cfg.MaxFileSize + multipartOverhead

	// Global middleware: security headers, CORS, IP resolution, recovery, health.
	// Timeout, GlobalThrottle, and SizeLimit are applied per sub-group so that the
	// long-lived SSE watch route is excluded from all three.
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
			Methods: []string{"GET", "POST", "PUT", "OPTIONS"},
			Headers: []string{"Content-Type", "Authorization"},
			MaxAge:  3600,
		}),
		func(h http.Handler) http.Handler {
			return middleware.RealIPWithTrustedProxies(cfg.TrustedProxies, h)
		},
		middleware.Recoverer(logger, false),
		middleware.Health,
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

	// Secret API routes.
	r.Mount("/api").Route(func(apiGroup *router.Group) {
		apiGroup.Use(
			middleware.Timeout(25*time.Second),
			middleware.GlobalThrottle(100),
			middleware.SizeLimit(maxRequestSize),
		)
		registerRoutes(apiGroup, logger, cfg, memStore)
	})

	// Channel push/pull: same size limit now that push sends raw binary (no base64 overhead).
	if channelStore != nil {
		r.Mount("/api").Route(func(chanGroup *router.Group) {
			chanGroup.Use(
				middleware.Timeout(25*time.Second),
				middleware.GlobalThrottle(100),
				middleware.SizeLimit(maxRequestSize),
			)
			registerChannelRoutes(chanGroup, logger, cfg, channelStore)
		})
	}

	// Web UI routes.
	r.Group().Route(func(webGroup *router.Group) {
		webGroup.Use(
			middleware.Timeout(25*time.Second),
			middleware.GlobalThrottle(100),
			middleware.SizeLimit(maxRequestSize),
		)
		registerWebRoutes(webGroup, logger, cfg, memStore, templates)
	})

	if channelStore != nil {
		// Watch (SSE): registered directly on root to bypass Timeout and
		// GlobalThrottle. Per-IP rate-limit and connection cap applied inline.
		watchHandler := router.Wrap(
			channelWatch(logger, channelStore, cfg),
			middleware.RateLimit(middleware.RateLimitConfig{RPS: cfg.WatchRPSPerIP, Burst: 3}),
			watchConnPerIP(cfg.WatchConnPerIP),
		)
		r.Handle("GET /api/channel/{id}/watch", watchHandler)

		// Channel web page.
		r.Group().Route(func(webGroup *router.Group) {
			webGroup.Use(
				middleware.Timeout(25*time.Second),
				middleware.GlobalThrottle(100),
				middleware.SizeLimit(maxRequestSize),
				middleware.RateLimit(middleware.RateLimitConfig{RPS: 20, Burst: 30}),
				middleware.Logger(logger),
				middleware.StripSlashes,
			)
			webGroup.HandleFunc("GET /channel/{id}", channelPage(logger, templates, channelStore))
		})
	}

	r.NotFoundHandler(notFoundPage(logger, templates))

	return r, nil
}
