package web

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/opencollective/community/internal/bunker"
	"github.com/opencollective/community/internal/store"
)

// /settings/apps: connect external Nostr apps via bunker URLs, list and
// revoke sessions (docs/architecture/bunker.md, BUNKER-01/05).

const (
	setRelayURL     = "relay_url" // ws URL the bunker listens on (zooid)
	bunkerSecretTTL = 10 * time.Minute
)

// SignerFor returns the community's bunker signer (also used by future
// channel/publishing handlers).
func (a *App) SignerFor(c *store.Community) *bunker.Signer {
	return &bunker.Signer{
		C:   c,
		DEK: func() ([]byte, bool) { return a.DEK(c) },
		Now: a.Now,
	}
}

// StartBunker launches (or restarts the subscription of) the community's
// NIP-46 service when a relay is configured.
func (a *App) StartBunker(c *store.Community) {
	relayURL, err := c.Setting(setRelayURL)
	if err != nil {
		return // no relay yet — the zooid milestone configures it
	}
	a.bunkerMu.Lock()
	defer a.bunkerMu.Unlock()
	if svc, ok := a.bunkers[c.Slug]; ok {
		svc.Refresh()
		return
	}
	svc := &bunker.Service{Signer: a.SignerFor(c), RelayURL: relayURL, Log: a.Log}
	svc.Start(a.baseCtx)
	a.bunkers[c.Slug] = svc
}

// requireUser gates pages for any logged-in active identity (followers
// included — it is their identity).
func (a *App) requireUser(h func(http.ResponseWriter, *http.Request, *store.Community, *store.Identity)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := communityFrom(r)
		i := identityFrom(r)
		if c == nil {
			http.NotFound(w, r)
			return
		}
		if i == nil || i.Status != "active" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		h(w, r, c, i)
	}
}

func (a *App) appsPage(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	a.renderApps(w, r, c, viewer, "")
}

func (a *App) renderApps(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity, bunkerURL string) {
	sessions, err := c.BunkerSessions(viewer.ID)
	if err != nil {
		a.internalError(w, err)
		return
	}
	type row struct {
		ID        int64
		AppName   string
		Client    string
		CreatedAt string
		LastUsed  string
		Revoked   bool
	}
	rows := make([]row, 0, len(sessions))
	for _, s := range sessions {
		name := s.AppName
		if name == "" {
			name = "Unnamed app"
		}
		rows = append(rows, row{
			ID: s.ID, AppName: name,
			Client:    s.ClientPubkey[:12] + "…",
			CreatedAt: time.Unix(s.CreatedAt, 0).UTC().Format("2006-01-02"),
			LastUsed:  time.Unix(s.LastUsedAt, 0).UTC().Format("2006-01-02 15:04"),
			Revoked:   s.Revoked,
		})
	}
	_, hasRelay := a.relayURLFor(c)
	a.render(w, "settings_apps.html", map[string]any{
		"Title": "Connected apps", "Sessions": rows,
		"BunkerURL": bunkerURL, "HasRelay": hasRelay,
	})
}

// relayURLFor returns the public websocket URL apps should use.
func (a *App) relayURLFor(c *store.Community) (string, bool) {
	u, err := c.Setting(setRelayURL)
	if err != nil || u == "" {
		return "", false
	}
	return u, true
}

func (a *App) appsGenerateURL(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	relayURL, ok := a.relayURLFor(c)
	if !ok {
		http.Error(w, "no relay configured yet", http.StatusConflict)
		return
	}
	signer := a.SignerFor(c)
	bunkerPK, err := signer.EnsureBunkerKey(viewer)
	if err != nil {
		a.internalError(w, err)
		return
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		a.internalError(w, err)
		return
	}
	secret := hex.EncodeToString(raw)
	if err := c.CreateBunkerSecret(viewer.ID, secret, bunkerSecretTTL, a.Now()); err != nil {
		a.internalError(w, err)
		return
	}
	a.StartBunker(c) // ensure running + resubscribed with the new key

	bunkerURL := fmt.Sprintf("bunker://%s?relay=%s&secret=%s", bunkerPK, relayURL, secret)
	a.renderApps(w, r, c, viewer, bunkerURL)
}

func (a *App) appsRevoke(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := c.RevokeBunkerSession(viewer.ID, id, a.Now()); err != nil &&
		!errors.Is(err, store.ErrNotFound) {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/settings/apps", http.StatusSeeOther)
}
