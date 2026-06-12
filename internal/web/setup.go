package web

import (
	"net/http"
	"regexp"
	"strings"
)

// Setup wizard, step 1: domain (docs/flows/setup.md). The DNS check and
// ACME issuance land with the TLS milestone; in dev mode (plain HTTP) the
// domain is recorded and the wizard proceeds. Steps 2–6 follow in the
// wizard milestone — until then the wizard stops after step 1.

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}(:[0-9]+)?$|^(localhost|127\.0\.0\.1)(:[0-9]+)?$`)

func (a *App) setupStep1(w http.ResponseWriter, r *http.Request) {
	n, err := a.Store.CommunityCount()
	if err != nil {
		a.internalError(w, err)
		return
	}
	if n > 0 {
		// SETUP-12: the wizard never reappears after completion.
		http.NotFound(w, r)
		return
	}
	a.render(w, "setup_domain.html", map[string]any{
		"Title": "Set up your community", "Step": 1, "Steps": 6, "Domain": "", "Error": "",
	})
}

func (a *App) setupStep1Submit(w http.ResponseWriter, r *http.Request) {
	n, err := a.Store.CommunityCount()
	if err != nil {
		a.internalError(w, err)
		return
	}
	if n > 0 {
		http.NotFound(w, r)
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.FormValue("domain")))
	if !domainRe.MatchString(domain) {
		a.render(w, "setup_domain.html", map[string]any{
			"Title": "Set up your community", "Step": 1, "Steps": 6,
			"Domain": domain,
			"Error":  "That does not look like a domain. Use something like community.example.org.",
		})
		return
	}
	if _, err := a.Store.CreateCommunity("main", domain); err != nil {
		a.internalError(w, err)
		return
	}
	// TODO(wizard milestone): DNS check, ACME issuance, redirect to HTTPS
	// (SETUP-02/03), then /setup/password. Until then, acknowledge.
	http.Redirect(w, r, "/setup/password", http.StatusSeeOther)
}
