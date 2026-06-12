package web

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/opencollective/community/internal/auth"
	"github.com/opencollective/community/internal/identity"
	"github.com/opencollective/community/internal/store"
)

// Follow (docs/flows/follow.md): email-only, double opt-in, an invisible
// Nostr identity created behind the scenes. Publishing the kind 0/3 events
// happens at the zooid milestone; identity and opt-in state are real now.

const followWindow = 30 * 24 * time.Hour

func (a *App) followPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "follow.html", map[string]any{
		"Title": "Follow", "Done": false, "Error": "",
	})
}

func (a *App) followSubmit(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !strings.Contains(email, "@") {
		a.render(w, "follow.html", map[string]any{
			"Title": "Follow", "Done": false, "Error": "Enter your email address.",
		})
		return
	}

	// Opportunistic GC of never-confirmed follows (FOLLOW-04).
	if err := c.GCUnconfirmed(a.Now().Add(-followWindow)); err != nil {
		a.internalError(w, err)
		return
	}

	done := func() {
		a.render(w, "follow.html", map[string]any{
			"Title": "Follow", "Done": true, "Error": "",
		})
	}

	m, err := a.mailer(c)
	if err != nil {
		a.internalError(w, err)
		return
	}

	// An existing address gets the same page and an email — never an
	// on-page disclosure (FOLLOW-05).
	if existing, err := c.IdentityByEmail(email); err == nil {
		switch existing.Status {
		case "unconfirmed":
			a.sendFollowConfirmation(r, c, existing, email)
		default:
			_ = m.Send(r.Context(), mailMessage([]string{email},
				"You already follow this community",
				"This address is already registered here — nothing to do."))
		}
		done()
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		a.internalError(w, err)
		return
	}

	dek, ok := a.DEK(c)
	if !ok {
		a.internalError(w, fmt.Errorf("community locked"))
		return
	}
	username, err := a.uniqueUsername(c, usernameFromEmail(email))
	if err != nil {
		a.internalError(w, err)
		return
	}
	kp, err := identity.Generate()
	if err != nil {
		a.internalError(w, err)
		return
	}
	enc, err := encryptSecret(dek, kp)
	if err != nil {
		a.internalError(w, err)
		return
	}
	ident, err := c.CreateIdentity(username, email, kp.PublicHex, enc, false, "unconfirmed", a.Now())
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.sendFollowConfirmation(r, c, ident, email)
	done()
}

func (a *App) sendFollowConfirmation(r *http.Request, c *store.Community, ident *store.Identity, email string) {
	token, err := auth.CreateLinkToken(c, email, "follow-confirm", followWindow, a.Now())
	if err != nil {
		a.Log.Error("follow token", "err", err)
		return
	}
	base := "https://" + c.Hostname
	if a.DevMode {
		base = "" // relative link works in tests; real dev uses the page
	}
	link := fmt.Sprintf("%s/follow/confirm?email=%s&token=%s", base, email, token)
	m, err := a.mailer(c)
	if err != nil {
		a.Log.Error("follow mailer", "err", err)
		return
	}
	_ = m.Send(r.Context(), mailMessage([]string{email},
		"Confirm to start following",
		"Confirm and you'll follow as @"+ident.Username+":\n\n"+link+
			"\n\nNew blog posts and newsletters will land in this inbox. Unsubscribe anytime."))
}

func (a *App) followConfirm(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	token := r.URL.Query().Get("token")

	if err := auth.VerifyLinkToken(c, email, "follow-confirm", token, a.Now()); err != nil {
		http.NotFound(w, r)
		return
	}
	ident, err := c.IdentityByEmail(email)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ident.Status == "unconfirmed" {
		if err := c.SetIdentityStatus(ident.ID, "active"); err != nil {
			a.internalError(w, err)
			return
		}
	}
	if err := c.UpdateProfile(ident.ID, ident.Name, true); err != nil {
		a.internalError(w, err)
		return
	}
	if err := c.AssignRole(ident.ID, "follower"); err != nil {
		a.internalError(w, err)
		return
	}
	// TODO(zooid milestone): publish kind 0 + kind 3 (FOLLOW-03's relay half).
	a.render(w, "follow_confirmed.html", map[string]any{
		"Title": "Following", "Username": ident.Username,
	})
}

var nonUsername = regexp.MustCompile(`[^a-z0-9_.-]`)

// usernameFromEmail derives a base username from the local part
// (FOLLOW-01/02).
func usernameFromEmail(email string) string {
	local := email[:strings.Index(email, "@")]
	u := nonUsername.ReplaceAllString(strings.ToLower(local), "")
	if len(u) > 24 {
		u = u[:24]
	}
	if len(u) < 2 || identity.ValidateUsername(u) != nil {
		u = "friend"
	}
	return u
}

// uniqueUsername suffixes the base until free: marie, marie1, marie2 …
func (a *App) uniqueUsername(c *store.Community, base string) (string, error) {
	candidate := base
	for n := 0; ; n++ {
		if n > 0 {
			candidate = fmt.Sprintf("%s%d", base, n)
		}
		taken, err := c.UsernameTaken(candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
}
