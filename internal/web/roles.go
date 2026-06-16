package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// Role administration (docs/flows/roles.md). Permissions are checked at
// action time, so grants and revocations take effect immediately (ROLE-04).
// Role changes are projected to the protocol: stewards become NIP-72
// moderators in the kind 34550 definition; moderate_chat holders become
// NIP-29 group admins via the zooid config (ROLE-06).

var allPermissions = []struct{ Key, Label string }{
	{store.PermApproveMembers, "Approve member applications"},
	{store.PermProposePosts, "Propose posts as the community"},
	{store.PermApprovePosts, "Approve posts as the community"},
	{store.PermModerateChat, "Moderate channels"},
	{store.PermManageRoles, "Manage roles and members"},
	{store.PermHoldFunds, "Hold funds (fiscal host)"},
}

func (a *App) requireManageRoles(h func(http.ResponseWriter, *http.Request, *store.Community, *store.Identity)) http.HandlerFunc {
	return a.requireMember(func(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
		ok := a.isAdmin(c, viewer.ID)
		if !ok {
			ok, _ = c.HasPermission(viewer.ID, store.PermManageRoles)
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		h(w, r, c, viewer)
	})
}

func (a *App) rolesPage(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	roles, err := c.RolesWithCounts()
	if err != nil {
		a.internalError(w, err)
		return
	}
	canManage := a.isAdmin(c, viewer.ID)
	if !canManage {
		canManage, _ = c.HasPermission(viewer.ID, store.PermManageRoles)
	}
	a.render(w, "roles_list.html", map[string]any{
		"Title": "Roles", "Roles": roles, "CanManage": canManage,
		"Permissions": allPermissions,
	})
}

func (a *App) roleDetail(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	role, err := c.RoleByName(r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	members, err := c.RoleMemberUsernames(role.ID)
	if err != nil {
		a.internalError(w, err)
		return
	}
	type permRow struct {
		Key, Label string
		On         bool
	}
	var perms []permRow
	for _, p := range allPermissions {
		perms = append(perms, permRow{p.Key, p.Label, contains(role.Permissions, p.Key)})
	}
	a.render(w, "role_detail.html", map[string]any{
		"Title": role.Name, "Role": role, "Members": members, "Perms": perms,
	})
}

func (a *App) roleCreate(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/roles", http.StatusSeeOther)
		return
	}
	if _, err := c.RoleByName(name); err == nil {
		http.Redirect(w, r, "/roles?err=exists", http.StatusSeeOther)
		return
	}
	if err := c.CreateCustomRole(name, r.Form["perm"], strings.TrimSpace(r.FormValue("color")), a.Now()); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/roles/"+name, http.StatusSeeOther)
}

func (a *App) roleUpdate(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	role, err := c.RoleByName(r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	color := strings.TrimSpace(r.FormValue("color"))
	newName := strings.TrimSpace(r.FormValue("name"))
	// Default roles can be recoloured and re-permissioned but not renamed
	// (code references them by name); custom roles can also be renamed.
	allowRename := !role.IsDefault && newName != "" && newName != role.Name
	if allowRename {
		if _, err := c.RoleByName(newName); err == nil {
			http.Redirect(w, r, "/roles/"+role.Name+"?err=exists", http.StatusSeeOther)
			return
		}
	}
	if err := c.UpdateRole(role.ID, newName, r.Form["perm"], color, allowRename); err != nil {
		a.internalError(w, err)
		return
	}
	a.afterRoleChange(c)
	target := role.Name
	if allowRename {
		target = newName
	}
	http.Redirect(w, r, "/roles/"+target, http.StatusSeeOther)
}

func (a *App) roleDelete(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	role, err := c.RoleByName(r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := c.DeleteRole(role.ID); err != nil {
		http.Redirect(w, r, "/roles/"+role.Name+"?err=default", http.StatusSeeOther)
		return
	}
	a.afterRoleChange(c)
	http.Redirect(w, r, "/roles", http.StatusSeeOther)
}

func (a *App) roleAssign(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	role, err := c.RoleByName(r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	target, err := c.IdentityByUsername(username)
	if err != nil {
		http.Redirect(w, r, "/roles/"+role.Name+"?err=nouser", http.StatusSeeOther)
		return
	}
	if r.FormValue("remove") == "1" {
		_ = c.RemoveRoleByID(role.ID, target.ID)
	} else {
		// hold_funds requires an operable account — claimed or managed
		// (UNCL-05). An unclaimed, unmanaged account can't be a host.
		if contains(role.Permissions, store.PermHoldFunds) && target.Status != "active" {
			if managed, _ := c.IsManaged(target.ID); !managed {
				http.Redirect(w, r, "/roles/"+role.Name+"?err=unclaimed", http.StatusSeeOther)
				return
			}
		}
		_ = c.AssignRoleByID(role.ID, target.ID)
	}
	a.afterRoleChange(c)
	http.Redirect(w, r, "/roles/"+role.Name, http.StatusSeeOther)
}

// afterRoleChange re-projects roles to the protocol: the zooid manager
// config (chat moderators) and the kind 34550 definition (stewards as
// moderators). Best-effort.
func (a *App) afterRoleChange(c *store.Community) {
	if err := a.syncZooid(c); err != nil {
		a.Log.Error("role change: sync zooid", "err", err)
	}
	a.republishCommunityDef(c)
}

func (a *App) republishCommunityDef(c *store.Community) {
	p, ok := a.publisher(c)
	if !ok {
		return
	}
	community, err := a.communityIdentity(c)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		return
	}
	mods, err := c.RolePubkeys("steward")
	if err != nil {
		return
	}
	name, _ := c.Setting(setName)
	desc, _ := c.Setting(setDescription)
	icon, _ := c.Setting(setIcon)
	evt := publish.CommunityDefinitionEvent(c.Slug, name, desc, icon, mods, a.Now())
	if err := p.PublishAs(ctx, community, claim, evt); err != nil {
		a.Log.Error("republish community definition", "err", err)
	}
}
