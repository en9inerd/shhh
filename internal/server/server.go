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

// SecurityHeaders middleware adds security headers to all responses
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func NewServer(
	logger *slog.Logger,
	cfg *config.Config,
	memStore *memstore.MemoryStore,
) http.Handler {
	r := router.New(http.NewServeMux())

	// global middleware
	r.Use(
		SecurityHeaders,
		middleware.RealIP,
		middleware.Recoverer(logger, false),
		middleware.GlobalThrottle(1000),
		middleware.Timeout(60*time.Second),
		middleware.Health,
		middleware.SizeLimit(64*1024),
	)

	r.Mount("/api").Route(func(g *router.Group) {
		registerRoutes(g, logger, cfg, memStore)
	})

	return r
}
