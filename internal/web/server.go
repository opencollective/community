// Package web is communityd's HTTP layer: tenant resolution by Host
// header (ADR 0009 — in place from the first PR), the setup wizard, and
// the application routes. TLS/autocert wraps this handler in production.
package web

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/opencollective/community/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets/*
var assetFS embed.FS

// App wires the HTTP layer to the store.
type App struct {
	Store *store.Server
	Log   *slog.Logger

	tmpl *template.Template
}

func New(s *store.Server, log *slog.Logger) (*App, error) {
	funcs := template.FuncMap{
		"seq": func(n int) []int {
			out := make([]int, n)
			for i := range out {
				out[i] = i + 1
			}
			return out
		},
	}
	t, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &App{Store: s, Log: log, tmpl: t}, nil
}

type ctxKey int

const communityKey ctxKey = 0

// communityFrom returns the request's resolved community, or nil during
// first-run setup. Handlers must use this — never global state.
func communityFrom(r *http.Request) *store.Community {
	c, _ := r.Context().Value(communityKey).(*store.Community)
	return c
}

// Handler builds the full route table behind the tenant middleware.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assetFS)))

	mux.HandleFunc("GET /setup", a.setupStep1)
	mux.HandleFunc("POST /setup", a.setupStep1Submit)

	mux.HandleFunc("/", a.home)

	return a.resolveTenant(mux)
}

// resolveTenant implements the convention of ADR 0009: every request
// resolves Host → community before any handler runs. While the server has
// no community yet (first run), everything except the wizard and statics
// redirects to /setup (SETUP-01).
func (a *App) resolveTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := a.Store.CommunityCount()
		if err != nil {
			a.internalError(w, err)
			return
		}
		if n == 0 {
			switch {
			case r.URL.Path == "/setup" || r.URL.Path == "/healthz" ||
				len(r.URL.Path) > 8 && r.URL.Path[:8] == "/static/":
				next.ServeHTTP(w, r)
			default:
				http.Redirect(w, r, "/setup", http.StatusFound)
			}
			return
		}

		c, err := a.Store.CommunityByHost(r.Host)
		if errors.Is(err, store.ErrNotFound) {
			// Unknown Host: neutral not-found, leaking nothing (TEN-01).
			http.NotFound(w, r)
			return
		}
		if err != nil {
			a.internalError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), communityKey, c)))
	})
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	c := communityFrom(r)
	name, err := c.Setting("name")
	if errors.Is(err, store.ErrNotFound) {
		name = c.Slug
	} else if err != nil {
		a.internalError(w, err)
		return
	}
	desc, _ := c.Setting("description")
	a.render(w, "home.html", map[string]any{
		"Title":       name,
		"Name":        name,
		"Description": desc,
	})
}

func (a *App) render(w http.ResponseWriter, name string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tmpl.ExecuteTemplate(w, name, data); err != nil {
		a.Log.Error("template", "name", name, "err", err)
	}
}

func (a *App) internalError(w http.ResponseWriter, err error) {
	a.Log.Error("internal error", "err", err)
	http.Error(w, "something went wrong on our side", http.StatusInternalServerError)
}
