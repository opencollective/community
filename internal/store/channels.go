package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Channel is one row of the channels registry
// (docs/nostr/channels.md).
type Channel struct {
	ID                int64
	Slug              string
	Name              string
	Type              string // chat | threads
	Template          string
	Enabled           bool
	Builtin           bool
	DefaultVisibility string // public | members
	Overridable       bool
	ApproveRoles      []string
	ApprovalsRequired int
	Position          int
}

// CreateDefaultChannels installs the built-ins (CHAN-01): #general always
// on, Proposals on, Requests off (it opens a write surface to externals).
func (c *Community) CreateDefaultChannels(now time.Time) error {
	rows := []struct {
		slug, name, typ, template, vis string
		enabled, overridable           bool
	}{
		{"general", "general", "chat", "", "members", true, false},
		{"proposals", "Proposals", "threads", "proposal", "members", true, false},
		{"requests", "Requests", "threads", "request", "public", false, true},
	}
	for i, r := range rows {
		_, err := c.DB.Exec(`
			INSERT INTO channels (slug, name, type, template, enabled, builtin,
				default_visibility, overridable, approve_roles, approvals_required,
				position, created_at)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?, 'steward', 1, ?, ?)`,
			r.slug, r.name, r.typ, r.template, boolInt(r.enabled),
			r.vis, boolInt(r.overridable), i, now.Unix())
		if err != nil {
			return err
		}
	}
	return nil
}

func scanChannel(row interface{ Scan(...any) error }) (*Channel, error) {
	var ch Channel
	var enabled, builtin, overridable int
	var roles string
	err := row.Scan(&ch.ID, &ch.Slug, &ch.Name, &ch.Type, &ch.Template,
		&enabled, &builtin, &ch.DefaultVisibility, &overridable,
		&roles, &ch.ApprovalsRequired, &ch.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ch.Enabled, ch.Builtin, ch.Overridable = enabled == 1, builtin == 1, overridable == 1
	for _, r := range strings.Split(roles, ",") {
		if r = strings.TrimSpace(r); r != "" {
			ch.ApproveRoles = append(ch.ApproveRoles, r)
		}
	}
	return &ch, nil
}

const channelCols = `id, slug, name, type, template, enabled, builtin,
	default_visibility, overridable, approve_roles, approvals_required, position`

// ChannelBySlug fetches one channel.
func (c *Community) ChannelBySlug(slug string) (*Channel, error) {
	return scanChannel(c.DB.QueryRow(
		`SELECT `+channelCols+` FROM channels WHERE slug = ?`, slug))
}

// Channels lists the registry in display order.
func (c *Community) Channels() ([]*Channel, error) {
	rows, err := c.DB.Query(`SELECT ` + channelCols + ` FROM channels ORDER BY position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// SetChannelEnabled toggles a channel; #general has no off switch.
func (c *Community) SetChannelEnabled(slug string, enabled bool) error {
	if slug == "general" && !enabled {
		return errors.New("store: #general cannot be disabled")
	}
	_, err := c.DB.Exec(`UPDATE channels SET enabled = ? WHERE slug = ?`,
		boolInt(enabled), slug)
	return err
}

// SetChannelPolicy updates a channel's approval policy (CHAN-16) and
// visibility defaults.
func (c *Community) SetChannelPolicy(slug string, approveRoles []string, required int, defaultVisibility string, overridable bool) error {
	if required < 1 {
		required = 1
	}
	if defaultVisibility != "public" {
		defaultVisibility = "members"
	}
	_, err := c.DB.Exec(`
		UPDATE channels SET approve_roles = ?, approvals_required = ?,
			default_visibility = ?, overridable = ? WHERE slug = ?`,
		strings.Join(approveRoles, ","), required, defaultVisibility,
		boolInt(overridable), slug)
	return err
}

// HasAnyRole reports whether an identity holds at least one of the names.
func (c *Community) HasAnyRole(identityID int64, names []string) (bool, error) {
	held, err := c.RoleNames(identityID)
	if err != nil {
		return false, err
	}
	for _, h := range held {
		for _, n := range names {
			if h == n {
				return true, nil
			}
		}
	}
	return false, nil
}