package server

import (
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// statusWriter wraps http.ResponseWriter to capture status code
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Logger middleware with masking for sensitive data, using slog
func Logger(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			next.ServeHTTP(sw, r)

			duration := time.Since(start)

			path := r.URL.Path
			if strings.Contains(r.URL.Path, "/secret/") {
				path = maskSensitivePath(r.URL)
			}

			remoteIP := "-"
			if r.RemoteAddr != "" {
				if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
					remoteIP = host
				}
			}

			l.Info("http request",
				"method", r.Method,
				"path", path,
				"ip", remoteIP,
				"status", sw.status,
				"duration", duration,
			)
		})
	}
}

// maskSensitivePath hides passphrases in /secret/{id}/{passphrase} paths
func maskSensitivePath(u *url.URL) string {
	path := u.String()
	if qun, err := url.QueryUnescape(path); err == nil {
		path = qun
	}

	if strings.Contains(path, "/secret/") {
		elems := strings.Split(path, "/")
		for i, elem := range elems {
			if elem == "secret" && i+2 < len(elems) {
				id := elems[i+1]
				if len(id) > 8 {
					id = id[:8] + "..."
				}
				path = strings.Join(elems[:i+1], "/") + "/" + id + "/*****"
				break
			}
		}
	}
	return path
}
