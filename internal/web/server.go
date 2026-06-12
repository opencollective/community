// Package web is communityd's HTTP layer: tenant resolution by Host
// header (ADR 0009 — in place from the first PR), the setup wizard, and
// the application routes. TLS/autocert wraps this handler in production.
package web

import (
	"context"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opencollective/community/internal/auth"
	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/internal/mail"
	"github.com/opencollective/community/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets/*
var assetFS embed.FS

// App wires the HTTP layer to the store. The injectable fields exist for
// the test harness (docs/testing/environment.md): clock, argon2
// parameters, mailer factory, domain checker.
type App struct {
	Store *store.Server
	Log   *slog.Logger

	// Now is the clock; tests advance it (never sleep).
	Now func() time.Time
	// Argon2 tunes password hashing; tests use crypto.TestArgon2.
	Argon2 crypto.Argon2Params
	// MailerFactory builds the outgoing mailer; tests inject a fake.
	MailerFactory mail.Factory
	// CheckDomain validates DNS before certificate issuance (SETUP-02);
	// nil skips the check (dev, tests).
	CheckDomain func(domain string) error
	// DevMode serves plain HTTP and keeps redirects relative.
	DevMode bool

	tmpl *template.Template
	keys *keyring

	mailMu  sync.Mutex
	mailers map[string]mail.Mailer // community slug -> configured mailer
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
	return &App{
		Store:         s,
		Log:           log,
		Now:           time.Now,
		Argon2:        crypto.DefaultArgon2,
		MailerFactory: mail.New,
		DevMode:       true,
		tmpl:          t,
		keys:          newKeyring(),
		mailers:       map[string]mail.Mailer{},
	}, nil
}

type ctxKey int

const (
	communityKey ctxKey = iota
	identityKey
)

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
	mux.HandleFunc("GET /setup/password", a.requireStep(2, a.setupPassword))
	mux.HandleFunc("POST /setup/password", a.requireStep(2, a.setupPasswordSubmit))
	mux.HandleFunc("GET /setup/admin", a.requireStep(3, a.setupAdmin))
	mux.HandleFunc("POST /setup/admin", a.requireStep(3, a.setupAdminSubmit))
	mux.HandleFunc("GET /setup/email", a.requireStep(4, a.setupEmail))
	mux.HandleFunc("POST /setup/email", a.requireStep(4, a.setupEmailSubmit))
	mux.HandleFunc("GET /setup/verify", a.requireStep(5, a.setupVerify))
	mux.HandleFunc("POST /setup/verify", a.requireStep(5, a.setupVerifySubmit))
	mux.HandleFunc("GET /setup/community", a.requireStep(6, a.setupCommunity))
	mux.HandleFunc("POST /setup/community", a.requireStep(6, a.setupCommunitySubmit))

	mux.HandleFunc("GET /unlock", a.unlockPage)
	mux.HandleFunc("POST /unlock", a.unlockSubmit)

	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.loginSubmit)
	mux.HandleFunc("POST /logout", a.logout)

	mux.HandleFunc("GET /follow", a.followPage)
	mux.HandleFunc("POST /follow", a.followSubmit)
	mux.HandleFunc("GET /follow/confirm", a.followConfirm)

	mux.HandleFunc("GET /join", a.joinPage)
	mux.HandleFunc("POST /join", a.joinSubmit)
	mux.HandleFunc("GET /join/check", a.joinCheck)
	mux.HandleFunc("POST /join/verify", a.joinVerify)

	mux.HandleFunc("GET /members", a.requireMember(a.membersPage))
	mux.HandleFunc("GET /members/pending", a.requireMember(a.pendingPage))
	mux.HandleFunc("POST /members/pending/{id}", a.requireMember(a.pendingDecide))

	mux.HandleFunc("/", a.home)

	return a.resolveTenant(mux)
}

// resolveTenant implements the convention of ADR 0009: every request
// resolves Host → community before any handler runs. While the server has
// no community yet (first run), everything except the wizard and statics
// redirects to /setup (SETUP-01). Once a community exists but its wizard
// is unfinished, every page resumes the wizard (SETUP-04).
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
				strings.HasPrefix(r.URL.Path, "/static/"):
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
		r = r.WithContext(context.WithValue(r.Context(), communityKey, c))

		if cookie, cerr := r.Cookie("session"); cerr == nil {
			if id, ok := auth.SessionIdentity(c, cookie.Value, a.Now()); ok {
				if ident, ierr := c.IdentityByID(id); ierr == nil {
					r = r.WithContext(context.WithValue(r.Context(), identityKey, ident))
				}
			}
		}

		if !strings.HasPrefix(r.URL.Path, "/setup") &&
			!strings.HasPrefix(r.URL.Path, "/static/") && r.URL.Path != "/healthz" {
			step, err := a.wizardStep(c)
			if err != nil {
				a.internalError(w, err)
				return
			}
			if step < 7 {
				http.Redirect(w, r, stepPaths[step], http.StatusFound)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	c := communityFrom(r)
	name, err := c.Setting(setName)
	if errors.Is(err, store.ErrNotFound) {
		name = c.Slug
	} else if err != nil {
		a.internalError(w, err)
		return
	}
	desc, _ := c.Setting(setDescription)
	a.render(w, "home.html", map[string]any{
		"Title":       name,
		"Name":        name,
		"Description": desc,
	})
}

// mailer returns the community's configured mailer, building it from
// settings on first use (the API key is encrypted with the DEK).
func (a *App) mailer(c *store.Community) (mail.Mailer, error) {
	a.mailMu.Lock()
	defer a.mailMu.Unlock()
	if m, ok := a.mailers[c.Slug]; ok {
		return m, nil
	}
	provider, err := c.Setting(setEmailProvider)
	if err != nil {
		return nil, err
	}
	from, err := c.Setting(setEmailFrom)
	if err != nil {
		return nil, err
	}
	keyHex, err := c.Setting(setEmailAPIKey)
	if err != nil {
		return nil, err
	}
	dek, ok := a.DEK(c)
	if !ok {
		return nil, fmt.Errorf("web: community %s is locked", c.Slug)
	}
	keyEnc, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, err
	}
	apiKey, err := crypto.Decrypt(dek, keyEnc, []byte("setting:"+setEmailAPIKey))
	if err != nil {
		return nil, err
	}
	m, err := a.MailerFactory(provider, string(apiKey), from)
	if err != nil {
		return nil, err
	}
	a.mailers[c.Slug] = m
	return m, nil
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   !a.DevMode,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 60 * 60,
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
