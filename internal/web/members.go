package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/opencollective/community/internal/store"
)

// Members directory and pending-applications review
// (docs/design/screens.md, docs/flows/join.md).

func (a *App) membersPage(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	members, err := c.MemberRows(q)
	if err != nil {
		a.internalError(w, err)
		return
	}
	type row struct {
		Username, Name string
		Roles          []string
	}
	rows := make([]row, 0, len(members))
	for _, m := range members {
		roles, _ := c.RoleNames(m.ID)
		var badges []string
		for _, rn := range roles {
			if rn != "follower" {
				badges = append(badges, rn)
			}
		}
		if a.isAdmin(c, m.ID) {
			badges = append([]string{"admin"}, badges...)
		}
		rows = append(rows, row{Username: m.Username, Name: m.Name, Roles: badges})
	}
	pending, err := c.PendingApplications()
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.render(w, "members.html", map[string]any{
		"Title": "Members", "Members": rows, "Query": q, "PendingCount": len(pending),
	})
}

func (a *App) pendingPage(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	apps, err := c.PendingApplications()
	if err != nil {
		a.internalError(w, err)
		return
	}
	canDecide := a.isAdmin(c, viewer.ID)
	if !canDecide {
		canDecide, _ = c.HasPermission(viewer.ID, store.PermApproveMembers)
	}
	type row struct {
		*store.Application
		Approvals int
		Approvers string
	}
	rows := make([]row, 0, len(apps))
	for _, app := range apps {
		ids, _ := c.Deciders(app.ID, "approve")
		var names []string
		for _, id := range ids {
			if ident, err := c.IdentityByID(id); err == nil {
				names = append(names, "@"+ident.Username)
			}
		}
		rows = append(rows, row{Application: app, Approvals: len(ids), Approvers: strings.Join(names, ", ")})
	}
	a.render(w, "members_pending.html", map[string]any{
		"Title": "Pending applications", "Apps": rows, "CanDecide": canDecide, "Error": r.URL.Query().Get("err"),
	})
}

// pendingDecide handles POST /members/pending/{id}: an approve or decline
// by the viewer, finalizing when the policy is met (JOIN-04..08).
func (a *App) pendingDecide(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	decision := r.FormValue("decision")
	reason := strings.TrimSpace(r.FormValue("reason"))
	if decision != "approve" && decision != "decline" {
		http.NotFound(w, r)
		return
	}
	isAdmin := a.isAdmin(c, viewer.ID)
	hasPerm, err := c.HasPermission(viewer.ID, store.PermApproveMembers)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if !isAdmin && !hasPerm {
		// Members may look, not act (JOIN-07).
		http.NotFound(w, r)
		return
	}
	app, err := c.ApplicationByID(id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && app.Status != "pending") {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}

	fresh, err := c.RecordDecision(app.ID, viewer.ID, decision, a.Now())
	if err != nil {
		a.internalError(w, err)
		return
	}
	if !fresh {
		http.Redirect(w, r, "/members/pending?err=already", http.StatusSeeOther)
		return
	}

	// Quorum: the admin alone, or two distinct holders whose permission is
	// re-checked now (JOIN-05/06; the role may have been lost since).
	met := isAdmin
	if !met {
		deciders, err := c.Deciders(app.ID, decision)
		if err != nil {
			a.internalError(w, err)
			return
		}
		valid := 0
		for _, did := range deciders {
			if a.isAdmin(c, did) {
				met = true
				break
			}
			if ok, _ := c.HasPermission(did, store.PermApproveMembers); ok {
				valid++
			}
		}
		if valid >= 2 {
			met = true
		}
	}
	if met {
		if decision == "approve" {
			err = a.finalizeApproval(r, c, app)
		} else {
			err = a.finalizeDecline(r, c, app, reason)
		}
		if err != nil {
			a.internalError(w, err)
			return
		}
	}
	http.Redirect(w, r, "/members/pending", http.StatusSeeOther)
}

func (a *App) finalizeApproval(r *http.Request, c *store.Community, app *store.Application) error {
	if err := c.SetApplicationStatus(app.ID, "approved", "", a.Now()); err != nil {
		return err
	}
	if err := c.SetIdentityStatus(app.IdentityID, "active"); err != nil {
		return err
	}
	if err := c.AssignRole(app.IdentityID, "member"); err != nil {
		return err
	}
	if ident, err := c.IdentityByID(app.IdentityID); err == nil {
		// Relay membership happens inside the publish (join + claim).
		a.publishIdentityEvents(c, ident)
		a.addToChannelGroups(c, ident)
	}
	if m, err := a.mailer(c); err == nil && app.Email != "" {
		_ = m.Send(r.Context(), mailMessage([]string{app.Email},
			"Welcome — your application was approved",
			"You are now a member. Log in here: https://"+c.Hostname+"/login"))
	}
	return nil
}

func (a *App) finalizeDecline(r *http.Request, c *store.Community, app *store.Application, reason string) error {
	if err := c.SetApplicationStatus(app.ID, "declined", reason, a.Now()); err != nil {
		return err
	}
	body := "Your application to join was declined."
	if reason != "" {
		body += "\n\nReason: " + reason
	}
	body += "\n\nYou may apply again after 30 days."
	if m, err := a.mailer(c); err == nil && app.Email != "" {
		_ = m.Send(r.Context(), mailMessage([]string{app.Email}, "About your application", body))
	}
	return nil
}
