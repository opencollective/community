package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/opencollective/community/internal/auth"
	"github.com/opencollective/community/internal/identity"
	"github.com/opencollective/community/internal/store"
)

var errLocked = errors.New("community locked")

// Unclaimed account control and claiming (docs/nostr/identities.md
// § unclaimed accounts). Anyone can create an account for someone else;
// the creator and admins control it until it is claimed (UNCL-02/03/04).

func (a *App) canControl(c *store.Community, viewer *store.Identity, account *store.Identity) bool {
	if a.isAdmin(c, viewer.ID) {
		return true
	}
	ok, _ := c.IsManager(account.ID, viewer.ID)
	return ok
}

func (a *App) accountsPage(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	managed, err := c.ManagedBy(viewer.ID)
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.render(w, "accounts.html", map[string]any{
		"Title": "Accounts you manage", "Managed": managed, "Error": r.URL.Query().Get("err"),
	})
}

func (a *App) accountCreate(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	name := strings.TrimSpace(r.FormValue("name"))
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	isOrg := r.FormValue("organization") == "1"
	if name == "" || identity.ValidateUsername(username) != nil {
		http.Redirect(w, r, "/accounts?err=invalid", http.StatusSeeOther)
		return
	}
	if taken, _ := c.UsernameTaken(username); taken {
		http.Redirect(w, r, "/accounts?err=taken", http.StatusSeeOther)
		return
	}
	dek, ok := a.DEK(c)
	if !ok {
		a.internalError(w, errLocked)
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
	ident, err := c.CreateIdentity(username, "", kp.PublicHex, enc, isOrg, "unclaimed", a.Now())
	if err != nil {
		a.internalError(w, err)
		return
	}
	_ = c.UpdateProfile(ident.ID, name, false)
	// The creator is the first manager (MGD-01).
	if err := c.AddManager(ident.ID, viewer.ID, viewer.ID, a.Now()); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/accounts/"+username, http.StatusSeeOther)
}

func (a *App) accountDetail(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	account, err := c.IdentityByUsername(r.PathValue("username"))
	if err != nil || !a.canControl(c, viewer, account) {
		http.NotFound(w, r)
		return
	}
	claimEmail, _ := c.ClaimEmail(account.ID)
	a.render(w, "account_detail.html", map[string]any{
		"Title": account.Username, "Account": account,
		"ClaimEmail": claimEmail, "Claimed": account.Status == "active",
		"Error": r.URL.Query().Get("err"),
	})
}

func (a *App) accountClaimEmail(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	account, err := c.IdentityByUsername(r.PathValue("username"))
	if err != nil || !a.canControl(c, viewer, account) {
		http.NotFound(w, r)
		return
	}
	if account.Status == "active" {
		http.NotFound(w, r) // already claimed — no handover
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !strings.Contains(email, "@") {
		http.Redirect(w, r, "/accounts/"+account.Username+"?err=email", http.StatusSeeOther)
		return
	}
	// Replacing the claim email invalidates any earlier pending claim
	// (UNCL-04): we bind the account to exactly this address.
	if err := c.SetClaimEmail(account.ID, email); err != nil {
		a.internalError(w, err)
		return
	}
	code, cerr := auth.CreateCode(c, email, "claim", a.Argon2, a.Now())
	if cerr == nil {
		if m, merr := a.mailer(c); merr == nil {
			base := a.publicURL(c, "http", "/accounts/claim?email="+email)
			_ = m.Send(r.Context(), mailMessage([]string{email},
				"Claim your account",
				"Someone set up the @"+account.Username+" account for you.\n"+
					"Claim it with this code: "+code+"\n\n"+base))
		}
	}
	http.Redirect(w, r, "/accounts/"+account.Username+"?err=sent", http.StatusSeeOther)
}

func (a *App) claimForm(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, "account_claim.html", map[string]any{
		"Title": "Claim your account", "Email": r.URL.Query().Get("email"), "Error": "",
	})
}

func (a *App) claimSubmit(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	code := strings.TrimSpace(r.FormValue("code"))

	fail := func() {
		a.render(w, "account_claim.html", map[string]any{
			"Title": "Claim your account", "Email": email, "Error": "That code is wrong or expired.",
		})
	}
	// The code must verify, and the account must currently point its claim
	// at exactly this email (UNCL-04 — a replaced email no longer matches).
	account, err := c.UnclaimedByClaimEmail(email)
	if err != nil {
		fail()
		return
	}
	if verr := auth.VerifyCode(c, email, "claim", code, a.Argon2, a.Now()); verr != nil {
		fail()
		return
	}
	if err := c.ClaimIdentity(account.ID, email); err != nil {
		a.internalError(w, err)
		return
	}
	token, err := auth.CreateSession(c, account.ID, a.Now())
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.setSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
