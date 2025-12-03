package server

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/memstore"
	"github.com/en9inerd/shhh/ui"
)

type templateData struct {
	Form        any
	CurrentYear int
	PageTitle   string
	PageDesc    string
	Config      *config.Config
	Intervals   []expirationInterval
	SecretID    string
}

type expirationInterval struct {
	Label    string
	Seconds  int
	Selected bool
}

type templateCache struct {
	templates map[string]*template.Template
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"div": func(a, b int64) int64 { return a / b },
	}
}

func newTemplateCache() (*templateCache, error) {
	cache := &templateCache{
		templates: make(map[string]*template.Template),
	}

	tmplFS, err := fs.Sub(ui.Files, "templates")
	if err != nil {
		return nil, fmt.Errorf("failed to get templates subdirectory: %w", err)
	}

	pages, err := fs.Glob(tmplFS, "pages/*.tmpl.html")
	if err != nil {
		return nil, fmt.Errorf("failed to glob pages: %w", err)
	}

	for _, page := range pages {
		name := strings.TrimSuffix(filepath.Base(page), ".tmpl.html")
		patterns := []string{
			"layouts/base.tmpl.html",
			"partials/*.tmpl.html",
			page,
		}

		ts, err := template.New("base").Funcs(templateFuncs()).ParseFS(tmplFS, patterns...)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", page, err)
		}
		cache.templates[name] = ts
	}

	partials, err := fs.Glob(tmplFS, "partials/*.tmpl.html")
	if err != nil {
		return nil, fmt.Errorf("failed to glob partials: %w", err)
	}

	for _, partial := range partials {
		name := strings.TrimSuffix(filepath.Base(partial), ".tmpl.html")
		ts, err := template.New(name).Funcs(templateFuncs()).ParseFS(tmplFS, partial)
		if err != nil {
			return nil, fmt.Errorf("failed to parse partial %s: %w", partial, err)
		}
		cache.templates[name] = ts
	}

	return cache, nil
}

func (tc *templateCache) render(w http.ResponseWriter, name string, td *templateData) error {
	tmpl, ok := tc.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, "base", td)
}

func (tc *templateCache) renderFragment(w http.ResponseWriter, name string, td *templateData) error {
	tmpl, ok := tc.templates[name]
	if !ok {
		return fmt.Errorf("template fragment %s not found", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, name, td)
}

func getExpirationIntervals(maxRetention time.Duration) []expirationInterval {
	intervals := []expirationInterval{
		{Label: "5 minutes", Seconds: 300},
		{Label: "15 minutes", Seconds: 900},
		{Label: "30 minutes", Seconds: 1800},
		{Label: "1 hour", Seconds: 3600},
		{Label: "6 hours", Seconds: 21600},
		{Label: "12 hours", Seconds: 43200},
		{Label: "24 hours", Seconds: 86400},
	}

	maxSeconds := int(maxRetention.Seconds())
	filtered := make([]expirationInterval, 0, len(intervals)+1)
	for _, interval := range intervals {
		if interval.Seconds <= maxSeconds {
			filtered = append(filtered, interval)
		}
	}
	return append(filtered, expirationInterval{Label: "Custom", Seconds: 0})
}


func renderError(w http.ResponseWriter, templates *templateCache, message string) {
	templates.renderFragment(w, "errors", &templateData{Form: map[string]string{"error": message}})
}

func parseExpiration(expUnit, customExp string) (int, error) {
	expStr := expUnit
	if expUnit == "custom" {
		if customExp == "" {
			return 0, fmt.Errorf("custom expiration value is required")
		}
		expStr = customExp
	}
	if expStr == "" {
		return 0, fmt.Errorf("expiration is required")
	}
	exp, err := strconv.Atoi(expStr)
	if err != nil || exp < 1 {
		return 0, fmt.Errorf("invalid expiration")
	}
	return exp, nil
}


func renderPage(w http.ResponseWriter, logger *slog.Logger, templates *templateCache, pageName string, td *templateData) {
	if err := templates.render(w, pageName, td); err != nil {
		logger.Error("failed to render page", "page", pageName, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func homePage(logger *slog.Logger, cfg *config.Config, templates *templateCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		intervals := getExpirationIntervals(cfg.MaxRetention)
		for i := range intervals {
			if intervals[i].Seconds > 0 {
				intervals[i].Selected = true
				break
			}
		}
		renderPage(w, logger, templates, "create", &templateData{
			Config:      cfg,
			Intervals:   intervals,
			PageTitle:   "Home - SHHH",
			PageDesc:    "Create and share encrypted secrets with expiration",
			CurrentYear: time.Now().Year(),
		})
	}
}

func retrievePage(logger *slog.Logger, templates *templateCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, logger, templates, "retrieve", &templateData{
			SecretID:    r.PathValue("id"),
			PageTitle:   "Retrieve Secret - SHHH",
			PageDesc:    "Retrieve your encrypted secret",
			CurrentYear: time.Now().Year(),
		})
	}
}

func notFoundPage(logger *slog.Logger, templates *templateCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		renderPage(w, logger, templates, "404", &templateData{
			PageTitle:   "Not Found - SHHH",
			PageDesc:    "Page not found",
			CurrentYear: time.Now().Year(),
		})
	}
}

func createSecretWeb(logger *slog.Logger, cfg *config.Config, memStore *memstore.MemoryStore, templates *templateCache, getData func(*http.Request) ([]byte, string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, filename, err := getData(r)
		if err != nil {
			logger.Warn("failed to get data", "error", err)
			renderError(w, templates, err.Error())
			return
		}

		passphrase := r.FormValue("passphrase")
		if err := validatePassphrase(passphrase, cfg); err != nil {
			renderError(w, templates, err.Error())
			return
		}

		exp, err := parseExpiration(r.FormValue("exp_unit"), r.FormValue("custom_exp"))
		if err != nil {
			renderError(w, templates, err.Error())
			return
		}

		id, storedItem, err := memStore.Store(data, filename, passphrase, calculateTTL(exp, cfg.MaxRetention))
		if err != nil {
			logger.Warn("failed to store", "error", err)
			renderError(w, templates, "Failed to create secret")
			return
		}

		logger.Info("created secret", "id", id, "filename", filename, "expires_at", storedItem.ExpiresAt.Format(time.RFC3339))
		renderSuccess(w, templates, id, cfg)
	}
}

func createTextSecretWeb(logger *slog.Logger, cfg *config.Config, memStore *memstore.MemoryStore, templates *templateCache) http.HandlerFunc {
	getData := func(r *http.Request) ([]byte, string, error) {
		if err := r.ParseForm(); err != nil {
			return nil, "", fmt.Errorf("invalid form data")
		}
		secret := r.FormValue("secret")
		if secret == "" {
			return nil, "", fmt.Errorf("secret is required")
		}
		return []byte(secret), "", nil
	}
	return createSecretWeb(logger, cfg, memStore, templates, getData)
}

func createFileSecretWeb(logger *slog.Logger, cfg *config.Config, memStore *memstore.MemoryStore, templates *templateCache) http.HandlerFunc {
	getData := func(r *http.Request) ([]byte, string, error) {
		if err := r.ParseMultipartForm(cfg.MaxFileSize + 10240); err != nil {
			return nil, "", fmt.Errorf("invalid form data")
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return nil, "", fmt.Errorf("file is required")
		}
		defer file.Close()

		fileData, err := io.ReadAll(file)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read file")
		}
		return fileData, header.Filename, nil
	}
	return createSecretWeb(logger, cfg, memStore, templates, getData)
}

func renderSuccess(w http.ResponseWriter, templates *templateCache, id string, cfg *config.Config) {
	if err := templates.renderFragment(w, "success", &templateData{
		SecretID: id,
		Config:   cfg,
	}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func retrieveSecretWeb(logger *slog.Logger, memStore *memstore.MemoryStore, templates *templateCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			logger.Warn("failed to parse form", "error", err)
			renderError(w, templates, "Invalid form data")
			return
		}

		id, passphrase := r.FormValue("id"), r.FormValue("passphrase")
		if id == "" || passphrase == "" {
			renderError(w, templates, "ID and passphrase are required")
			return
		}

		data, filename, err := memStore.Retrieve(id, passphrase)
		if err != nil {
			logger.Warn("secret retrieval failed", "id", id, "error", err)
			renderError(w, templates, "Secret not found or expired")
			return
		}

		form := map[string]interface{}{"is_file": filename != ""}
		if filename != "" {
			form["filename"] = filename
			form["file_data_b64"] = base64.StdEncoding.EncodeToString(data)
		} else {
			form["secret"] = string(data)
		}

		templates.renderFragment(w, "secret_result", &templateData{
			SecretID: id,
			Form:     form,
		})
	}
}
