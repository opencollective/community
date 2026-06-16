package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Role administration (docs/flows/roles.md, ROLE-*).

// RoleInfo is a role with its member count for the management UI.
type RoleInfo struct {
	ID          int64
	Name        string
	Permissions []string
	IsDefault   bool
	Color       string
	Count       int
}

func splitPerms(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// RolesWithCounts lists all roles with their member counts.
func (c *Community) RolesWithCounts() ([]*RoleInfo, error) {
	rows, err := c.DB.Query(`
		SELECT r.id, r.name, r.permissions, r.is_default, r.color,
		       (SELECT COUNT(*) FROM role_members m WHERE m.role_id = r.id)
		FROM roles r ORDER BY r.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RoleInfo
	for rows.Next() {
		var ri RoleInfo
		var perms string
		var isDefault int
		if err := rows.Scan(&ri.ID, &ri.Name, &perms, &isDefault, &ri.Color, &ri.Count); err != nil {
			return nil, err
		}
		ri.Permissions = splitPerms(perms)
		ri.IsDefault = isDefault == 1
		out = append(out, &ri)
	}
	return out, rows.Err()
}

// RoleByName returns one role.
func (c *Community) RoleByName(name string) (*RoleInfo, error) {
	var ri RoleInfo
	var perms string
	var isDefault int
	err := c.DB.QueryRow(
		`SELECT id, name, permissions, is_default, color FROM roles WHERE name = ?`, name).
		Scan(&ri.ID, &ri.Name, &perms, &isDefault, &ri.Color)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ri.Permissions = splitPerms(perms)
	ri.IsDefault = isDefault == 1
	return &ri, nil
}

// CreateCustomRole adds a non-default role.
func (c *Community) CreateCustomRole(name string, permissions []string, color string, now time.Time) error {
	_, err := c.DB.Exec(
		`INSERT INTO roles (name, permissions, is_default, color, created_at) VALUES (?, ?, 0, ?, ?)`,
		name, strings.Join(permissions, ","), color, now.Unix())
	return err
}

// UpdateRole sets a role's permissions and color (and renames custom roles).
func (c *Community) UpdateRole(id int64, name string, permissions []string, color string, allowRename bool) error {
	if allowRename {
		_, err := c.DB.Exec(`UPDATE roles SET name = ?, permissions = ?, color = ? WHERE id = ?`,
			name, strings.Join(permissions, ","), color, id)
		return err
	}
	_, err := c.DB.Exec(`UPDATE roles SET permissions = ?, color = ? WHERE id = ?`,
		strings.Join(permissions, ","), color, id)
	return err
}

// DeleteRole removes a custom role; default roles cannot be deleted (ROLE-01).
func (c *Community) DeleteRole(id int64) error {
	res, err := c.DB.Exec(`DELETE FROM roles WHERE id = ? AND is_default = 0`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("store: default roles cannot be deleted")
	}
	return nil
}

// AssignRoleByID / RemoveRoleByID manage membership.
func (c *Community) AssignRoleByID(roleID, identityID int64) error {
	_, err := c.DB.Exec(
		`INSERT OR IGNORE INTO role_members (role_id, identity_id) VALUES (?, ?)`, roleID, identityID)
	return err
}

func (c *Community) RemoveRoleByID(roleID, identityID int64) error {
	_, err := c.DB.Exec(`DELETE FROM role_members WHERE role_id = ? AND identity_id = ?`, roleID, identityID)
	return err
}

// RolePubkeys lists the pubkeys of active identities holding a role by
// name (for protocol projection — stewards as 34550 moderators, ROLE-06).
func (c *Community) RolePubkeys(roleName string) ([]string, error) {
	rows, err := c.DB.Query(`
		SELECT i.pubkey FROM identities i
		JOIN role_members m ON m.identity_id = i.id
		JOIN roles r ON r.id = m.role_id
		WHERE r.name = ? AND i.status = 'active' AND i.pubkey IS NOT NULL`, roleName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			return nil, err
		}
		out = append(out, pk)
	}
	return out, rows.Err()
}

// RoleMemberUsernames lists the usernames holding a role.
func (c *Community) RoleMemberUsernames(roleID int64) ([]string, error) {
	rows, err := c.DB.Query(`
		SELECT i.username FROM identities i
		JOIN role_members m ON m.identity_id = i.id
		WHERE m.role_id = ? ORDER BY i.username`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
