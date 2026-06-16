package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/opencollective/community/internal/store"
)

// /settings/community (admin only): channel toggles and per-channel
// approval policies (CHAN-01/02/16, ADR 0010/0012). Theme, posting
// policies and the rest of the settings surface land with their features.

func (a *App) requireAdmin(h func(http.ResponseWriter, *http.Request, *store.Community, *store.Identity)) http.HandlerFunc {
	return a.requireMember(func(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
		if !a.isAdmin(c, viewer.ID) {
			http.NotFound(w, r)
			return
		}
		h(w, r, c, viewer)
	})
}

func (a *App) settingsPage(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	channels, err := c.Channels()
	if err != nil {
		a.internalError(w, err)
		return
	}
	policies, err := c.AllPostPolicies()
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.render(w, "settings_community.html", map[string]any{
		"Title": "Community settings", "Channels": channels, "Policies": policies,
	})
}

// settingsPolicy updates a publishing section's approval policy (PUB-11).
func (a *App) settingsPolicy(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	ct := r.PathValue("type")
	cur, err := c.PostPolicyFor(ct)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var roles []string
	for _, role := range strings.Split(r.FormValue("approve_roles"), ",") {
		if role = strings.TrimSpace(role); role != "" {
			roles = append(roles, role)
		}
	}
	required := atoiDefault(r.FormValue("approvals_required"), cur.ApprovalsRequired)
	if err := c.SetPostPolicy(ct, roles, required); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/settings/community", http.StatusSeeOther)
}

func (a *App) settingsChannel(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	slug := r.PathValue("slug")
	ch, err := c.ChannelBySlug(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if ch.Slug != "general" {
		enabled := r.FormValue("enabled") == "1"
		if err := c.SetChannelEnabled(slug, enabled); err != nil {
			a.internalError(w, err)
			return
		}
		if enabled && !ch.Enabled {
			// Channel turned on: provision its NIP-29 group and grant
			// current members access (CHAN-02 re-enable keeps history —
			// the group and events persist on the relay).
			a.ensureChannelGroup(c, slug)
		}
	}

	var roles []string
	for _, role := range strings.Split(r.FormValue("approve_roles"), ",") {
		if role = strings.TrimSpace(role); role != "" {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		roles = ch.ApproveRoles
	}
	required, err := strconv.Atoi(r.FormValue("approvals_required"))
	if err != nil || required < 1 {
		required = ch.ApprovalsRequired
	}
	visibility := r.FormValue("default_visibility")
	if visibility == "" {
		visibility = ch.DefaultVisibility
	}
	overridable := ch.Overridable
	if v := r.FormValue("overridable"); v != "" {
		overridable = v == "1"
	}
	if err := c.SetChannelPolicy(slug, roles, required, visibility, overridable); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/settings/community", http.StatusSeeOther)
}
