package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/en9inerd/shhh/internal/channel"
	"github.com/en9inerd/shhh/internal/config"
)

// clientIP extracts the IP from an addr that may be "host:port" or bare IP.
func clientIP(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

// watchConnPerIP returns middleware that caps concurrent SSE connections per IP.
func watchConnPerIP(limit int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	conns := make(map[string]int)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r.RemoteAddr)
			mu.Lock()
			if conns[ip] >= limit {
				mu.Unlock()
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
			conns[ip]++
			mu.Unlock()
			defer func() {
				mu.Lock()
				conns[ip]--
				if conns[ip] == 0 {
					delete(conns, ip)
				}
				mu.Unlock()
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func channelPush(cs *channel.ChannelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !channel.IsValidUUID(id) {
			http.Error(w, "invalid channel id", http.StatusBadRequest)
			return
		}
		ch, ok := cs.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}

		blob, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		if len(blob) < channel.MinBlobSize {
			http.Error(w, "blob too short", http.StatusBadRequest)
			return
		}

		if _, ok = ch.Push(blob); !ok {
			http.Error(w, "queue full", http.StatusTooManyRequests)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	}
}

type sseMessageData struct {
	Blob     string `json:"blob"`
	PushedAt string `json:"pushed_at"`
}

func channelPull(logger *slog.Logger, cs *channel.ChannelStore, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !channel.IsValidUUID(id) {
			http.Error(w, "invalid channel id", http.StatusBadRequest)
			return
		}
		ch, ok := cs.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}

		limit := cfg.ChannelMaxMsgs
		if ls := r.URL.Query().Get("limit"); ls != "" {
			n, err := strconv.Atoi(ls)
			if err != nil || n <= 0 || n > cfg.ChannelMaxMsgs {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			limit = n
		}

		msgs := ch.Pull(limit)
		type pullResp struct {
			Messages []sseMessageData `json:"messages"`
		}
		resp := pullResp{Messages: make([]sseMessageData, 0, len(msgs))}
		for _, m := range msgs {
			resp.Messages = append(resp.Messages, sseMessageData{
				Blob:     base64.StdEncoding.EncodeToString(m.Blob),
				PushedAt: m.PushedAt.UTC().Format(time.RFC3339),
			})
		}

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("channel pull: encode response", "error", err)
		}
	}
}

func channelWatch(logger *slog.Logger, cs *channel.ChannelStore, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !channel.IsValidUUID(id) {
			http.Error(w, "invalid channel id", http.StatusBadRequest)
			return
		}
		ch, ok := cs.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}

		// All error responses (400, 404, 429) must be sent before any Write/Flush
		// because the first Write commits response headers and status code.

		// Atomically check watcher cap, register, and snapshot the queue.
		sub, snapshot, ok := ch.Subscribe(cfg.ChannelMaxMsgs)
		if !ok {
			http.Error(w, "too many watchers", http.StatusTooManyRequests)
			return
		}
		defer ch.Unsubscribe(sub)

		// ResponseController walks Unwrap chains — works through middleware wrappers
		// that implement Unwrap() (e.g. the recoverer's recoverWriter).
		rc := http.NewResponseController(w)

		// Override both deadlines so the connection can live beyond httpServer.ReadTimeout/WriteTimeout.
		if err := rc.SetReadDeadline(time.Time{}); err != nil {
			logger.Warn("channelWatch: SetReadDeadline", "error", err)
		}
		if err := rc.SetWriteDeadline(time.Time{}); err != nil {
			logger.Error("channelWatch: SetWriteDeadline failed, aborting SSE", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		// Set SSE response headers before the first Write commits them.
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")

		fmt.Fprint(w, "event: connected\ndata: {}\n\n")
		rc.Flush()

		for _, msg := range snapshot {
			writeSSEMessage(w, msg)
			rc.Flush()
		}

		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()

		for {
			select {
			case msg, open := <-sub:
				if !open {
					return
				}
				writeSSEMessage(w, msg)
				rc.Flush()
			case <-keepalive.C:
				fmt.Fprint(w, ": keepalive\n\n")
				rc.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}

func writeSSEMessage(w http.ResponseWriter, msg channel.Message) {
	data, _ := json.Marshal(sseMessageData{
		Blob:     base64.StdEncoding.EncodeToString(msg.Blob),
		PushedAt: msg.PushedAt.UTC().Format(time.RFC3339),
	})
	fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
}
