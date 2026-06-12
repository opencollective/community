package store

import (
	"strings"
	"time"
)

// Permission flags (docs/flows/roles.md).
const (
	PermApproveMembers = "approve_members"
	PermProposePosts   = "propose_posts"
	PermApprovePosts   = "approve_posts"
	PermModerateChat   = "moderate_chat"
	PermManageRoles    = "manage_roles"
	PermHoldFunds      = "hold_funds"
)

// DefaultRoles are created at setup step 6 and cannot be deleted
// (SETUP-11, ROLE-01).
var DefaultRoles = []struct {
	Name        string
	Permissions []string
}{
	{"steward", []string{PermApproveMembers, PermProposePosts, PermApprovePosts}},
	{"moderator", []string{PermModerateChat}},
	{"member", nil},
	{"follower", nil},
	{"fiscal host", []string{PermHoldFunds}},
}

// CreateRole inserts a role; isDefault roles are undeletable.
func (c *Community) CreateRole(name string, permissions []string, isDefault bool, now time.Time) (int64, error) {
	res, err := c.DB.Exec(
		`INSERT INTO roles (name, permissions, is_default, created_at) VALUES (?, ?, ?, ?)`,
		name, strings.Join(permissions, ","), boolInt(isDefault), now.Unix(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateDefaultRoles installs the five defaults.
func (c *Community) CreateDefaultRoles(now time.Time) error {
	for _, r := range DefaultRoles {
		if _, err := c.CreateRole(r.Name, r.Permissions, true, now); err != nil {
			return err
		}
	}
	return nil
}

// AssignRole adds an identity to a role by role name.
func (c *Community) AssignRole(identityID int64, roleName string) error {
	_, err := c.DB.Exec(
		`INSERT OR IGNORE INTO role_members (role_id, identity_id)
		 SELECT id, ? FROM roles WHERE name = ?`, identityID, roleName)
	return err
}

// HasPermission reports whether an identity holds a permission through any
// of its roles. Checked at action time (LOGIN-07).
func (c *Community) HasPermission(identityID int64, perm string) (bool, error) {
	rows, err := c.DB.Query(
		`SELECT r.permissions FROM roles r
		 JOIN role_members m ON m.role_id = r.id WHERE m.identity_id = ?`, identityID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var perms string
		if err := rows.Scan(&perms); err != nil {
			return false, err
		}
		for _, p := range strings.Split(perms, ",") {
			if p == perm {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

// RoleNames returns the role names an identity holds.
func (c *Community) RoleNames(identityID int64) ([]string, error) {
	rows, err := c.DB.Query(
		`SELECT r.name FROM roles r JOIN role_members m ON m.role_id = r.id
		 WHERE m.identity_id = ? ORDER BY r.created_at`, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}
