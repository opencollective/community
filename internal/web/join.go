package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/opencollective/community/internal/auth"
	"github.com/opencollective/community/internal/identity"
	"github.com/opencollective/community/internal/store"
)

// Join (docs/flows/join.md): application → email verification → review by
// the admin or two approve_members holders. The protocol half (applicant-
// signed application events) lands with the bunker milestone.

const reapplyWindow = 30 * 24 * time.Hour

func (a *App) joinPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "join.html", map[string]any{
		"Title": "Apply to join", "Stage": "form",
		"Name": "", "Username": "", "Email": "", "Motivation": "", "Error": "",
	})
}

// joinCheck is the live username availability probe (JOIN-02).
func (a *App) joinCheck(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	u := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("username")))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := identity.ValidateUsername(u); err != nil {
		fmt.Fprintf(w, "invalid: %s", err)
		return
	}
	taken, err := c.UsernameTaken(u)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if !taken {
		fmt.Fprint(w, "available")
		return
	}
	suggestion, err := a.uniqueUsername(c, u)
	if err != nil {
		a.internalError(w, err)
		return
	}
	fmt.Fprintf(w, "taken — try %s", suggestion)
}

func (a *App) joinSubmit(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	name := strings.TrimSpace(r.FormValue("name"))
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	motivation := strings.TrimSpace(r.FormValue("motivation"))
	newsletter := r.FormValue("newsletter") == "1"

	fail := func(msg string) {
		a.render(w, "join.html", map[string]any{
			"Title": "Apply to join", "Stage": "form",
			"Name": name, "Username": username, "Email": email,
			"Motivation": motivation, "Error": msg,
		})
	}
	if name == "" || !strings.Contains(email, "@") || motivation == "" {
		fail("Name, email and a few words about you are required.")
		return
	}

	// One open application per email (JOIN-09).
	if _, err := c.OpenApplicationByEmail(email); err == nil {
		fail("There is already an application for this address — it is waiting for review.")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		a.internalError(w, err)
		return
	}

	var ident *store.Identity
	existing, err := c.IdentityByEmail(email)
	switch {
	case err == nil:
		// A follower (or earlier applicant) keeps their identity and
		// username (FOLLOW-07).
		if a.memberLevel(c, existing) {
			fail("This address already belongs to a member — log in instead.")
			return
		}
		if declined, derr := c.LastDeclineFor(existing.ID); derr == nil && !declined.IsZero() &&
			a.Now().Before(declined.Add(reapplyWindow)) {
			fail("A previous application for this address was declined recently — you can reapply after 30 days.")
			return
		}
		ident = existing
	case errors.Is(err, store.ErrNotFound):
		if verr := identity.ValidateUsername(username); verr != nil {
			fail(verr.Error())
			return
		}
		if taken, terr := c.UsernameTaken(username); terr != nil {
			a.internalError(w, terr)
			return
		} else if taken {
			suggestion, _ := a.uniqueUsername(c, username)
			fail(fmt.Sprintf("That username is taken — %s is available.", suggestion))
			return
		}
		dek, ok := a.DEK(c)
		if !ok {
			a.internalError(w, fmt.Errorf("community locked"))
			return
		}
		kp, kerr := identity.Generate()
		if kerr != nil {
			a.internalError(w, kerr)
			return
		}
		enc, eerr := encryptSecret(dek, kp)
		if eerr != nil {
			a.internalError(w, eerr)
			return
		}
		ident, err = c.CreateIdentity(username, email, kp.PublicHex, enc, false, "pending", a.Now())
		if err != nil {
			a.internalError(w, err)
			return
		}
	default:
		a.internalError(w, err)
		return
	}

	if err := c.UpdateProfile(ident.ID, name, newsletter); err != nil {
		a.internalError(w, err)
		return
	}
	if _, err := c.CreateApplication(ident.ID, motivation, newsletter, a.Now()); err != nil {
		a.internalError(w, err)
		return
	}
	// We review humans, not typos (JOIN-01): verify the address by code.
	code, err := auth.CreateCode(c, email, "join", a.Argon2, a.Now())
	if err != nil && !errors.Is(err, auth.ErrRateLimited) {
		a.internalError(w, err)
		return
	}
	if err == nil {
		if m, merr := a.mailer(c); merr == nil {
			_ = m.Send(r.Context(), mailMessage([]string{email},
				"Confirm your application",
				"Your code: "+code+"\nIt expires in 10 minutes."))
		}
	}
	a.render(w, "join.html", map[string]any{
		"Title": "Apply to join", "Stage": "verify", "Email": email, "Error": "",
	})
}

func (a *App) joinVerify(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	code := strings.TrimSpace(r.FormValue("code"))

	if err := auth.VerifyCode(c, email, "join", code, a.Argon2, a.Now()); err != nil {
		a.render(w, "join.html", map[string]any{
			"Title": "Apply to join", "Stage": "verify", "Email": email,
			"Error": "That code is wrong or expired.",
		})
		return
	}
	app, err := c.OpenApplicationByEmail(email)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if err := c.MarkApplicationPending(app.ID); err != nil {
		a.internalError(w, err)
		return
	}
	if m, merr := a.mailer(c); merr == nil {
		_ = m.Send(r.Context(), mailMessage([]string{email},
			"Application received",
			"Your application to join is in — it will be reviewed by the admin or two stewards."))
	}
	a.render(w, "join.html", map[string]any{
		"Title": "Apply to join", "Stage": "done", "Email": email, "Error": "",
	})
}
