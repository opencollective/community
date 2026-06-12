package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/opencollective/community/internal/auth"
	"github.com/opencollective/community/internal/store"
)

// Login (docs/flows/login.md): passwordless email codes. The page behaves
// identically whether the address has an account or not (LOGIN-04).

func identityFrom(r *http.Request) *store.Identity {
	i, _ := r.Context().Value(identityKey).(*store.Identity)
	return i
}

// isAdmin reports whether an identity is the community's admin account.
func (a *App) isAdmin(c *store.Community, identityID int64) bool {
	idStr, err := c.Setting(setAdminIdentity)
	if err != nil {
		return false
	}
	return idStr == itoa(identityID)
}

// memberLevel reports whether an identity may see member pages: the admin,
// or any active identity holding a member-level role.
func (a *App) memberLevel(c *store.Community, i *store.Identity) bool {
	if i == nil || i.Status != "active" {
		return false
	}
	if a.isAdmin(c, i.ID) {
		return true
	}
	names, err := c.RoleNames(i.ID)
	if err != nil {
		return false
	}
	for _, n := range names {
		switch n {
		case "member", "steward", "moderator", "fiscal host":
			return true
		}
	}
	return false
}

// requireMember gates member-only pages: anonymous → /login, non-member →
// the same neutral not-found as any unauthorized page (JOIN-03, LOG-01).
func (a *App) requireMember(h func(http.ResponseWriter, *http.Request, *store.Community, *store.Identity)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := communityFrom(r)
		i := identityFrom(r)
		if c == nil {
			http.NotFound(w, r)
			return
		}
		if i == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if !a.memberLevel(c, i) {
			http.NotFound(w, r)
			return
		}
		h(w, r, c, i)
	}
}

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "login.html", map[string]any{
		"Title": "Log in", "Email": "", "Sent": false, "Error": "",
	})
}

func (a *App) loginSubmit(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	code := strings.TrimSpace(r.FormValue("code"))

	if code == "" {
		if !strings.Contains(email, "@") {
			a.render(w, "login.html", map[string]any{
				"Title": "Log in", "Email": email, "Sent": false,
				"Error": "Enter your email address.",
			})
			return
		}
		// Only active accounts get a code — but the page never says
		// whether one was sent (LOGIN-04).
		ident, err := c.IdentityByEmail(email)
		if err == nil && ident.Status == "active" {
			generated, cerr := auth.CreateCode(c, email, "login", a.Argon2, a.Now())
			if errors.Is(cerr, auth.ErrRateLimited) {
				a.render(w, "login.html", map[string]any{
					"Title": "Log in", "Email": email, "Sent": false,
					"Error": "Too many codes requested for this address — try again in an hour.",
				})
				return
			}
			if cerr == nil {
				if m, merr := a.mailer(c); merr == nil {
					_ = m.Send(r.Context(), mailMessage([]string{email},
						"Your login code",
						"Your code: "+generated+"\nIt expires in 10 minutes."))
				}
			}
		} else if err != nil && !errors.Is(err, store.ErrNotFound) {
			a.internalError(w, err)
			return
		}
		a.render(w, "login.html", map[string]any{
			"Title": "Log in", "Email": email, "Sent": true, "Error": "",
		})
		return
	}

	if err := auth.VerifyCode(c, email, "login", code, a.Argon2, a.Now()); err != nil {
		a.render(w, "login.html", map[string]any{
			"Title": "Log in", "Email": email, "Sent": true,
			"Error": "That code is wrong or expired.",
		})
		return
	}
	ident, err := c.IdentityByEmail(email)
	if err != nil {
		a.internalError(w, err)
		return
	}
	token, err := auth.CreateSession(c, ident.ID, a.Now())
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.setSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if cookie, err := r.Cookie("session"); err == nil && c != nil {
		_ = auth.DeleteSession(c, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
