package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Publishing policy and newsletter bookkeeping (docs/nostr/publishing.md).

// PostPolicy is one section's approval policy.
type PostPolicy struct {
	ContentType       string
	ApproveRoles      []string
	ApprovalsRequired int
}

// DefaultPostPolicies are installed at setup (ADR 0011): announcements and
// blog 1 steward, newsletter 2 stewards.
var DefaultPostPolicies = []PostPolicy{
	{"announcement", []string{"steward"}, 1},
	{"blog", []string{"steward"}, 1},
	{"newsletter", []string{"steward"}, 2},
}

// CreateDefaultPostPolicies installs the section defaults.
func (c *Community) CreateDefaultPostPolicies() error {
	for _, p := range DefaultPostPolicies {
		_, err := c.DB.Exec(
			`INSERT INTO post_policies (content_type, approve_roles, approvals_required)
			 VALUES (?, ?, ?)`,
			p.ContentType, strings.Join(p.ApproveRoles, ","), p.ApprovalsRequired)
		if err != nil {
			return err
		}
	}
	return nil
}

// PostPolicyFor returns a section's policy.
func (c *Community) PostPolicyFor(contentType string) (*PostPolicy, error) {
	var roles string
	var required int
	err := c.DB.QueryRow(
		`SELECT approve_roles, approvals_required FROM post_policies WHERE content_type = ?`,
		contentType).Scan(&roles, &required)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p := &PostPolicy{ContentType: contentType, ApprovalsRequired: required}
	for _, r := range strings.Split(roles, ",") {
		if r = strings.TrimSpace(r); r != "" {
			p.ApproveRoles = append(p.ApproveRoles, r)
		}
	}
	return p, nil
}

// SetPostPolicy updates a section's policy (PUB-11).
func (c *Community) SetPostPolicy(contentType string, roles []string, required int) error {
	if required < 1 {
		required = 1
	}
	_, err := c.DB.Exec(
		`UPDATE post_policies SET approve_roles = ?, approvals_required = ? WHERE content_type = ?`,
		strings.Join(roles, ","), required, contentType)
	return err
}

// AllPostPolicies lists the sections in a stable order.
func (c *Community) AllPostPolicies() ([]*PostPolicy, error) {
	out := make([]*PostPolicy, 0, 3)
	for _, ct := range []string{"announcement", "blog", "newsletter"} {
		p, err := c.PostPolicyFor(ct)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// NewsletterSent reports whether a newsletter event was already emailed.
func (c *Community) NewsletterSent(eventID string) (bool, error) {
	var n int
	err := c.DB.QueryRow(`SELECT COUNT(*) FROM newsletter_log WHERE event_id = ?`, eventID).Scan(&n)
	return n > 0, err
}

// ClaimNewsletterSend records the intent to send, returning false if
// already claimed (MAIL-05 at-most-once under concurrency).
func (c *Community) ClaimNewsletterSend(eventID string, now time.Time) (bool, error) {
	res, err := c.DB.Exec(
		`INSERT OR IGNORE INTO newsletter_log (event_id, sent_at, recipient_count) VALUES (?, ?, 0)`,
		eventID, now.Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RecordNewsletterSent finalizes the recipient count.
func (c *Community) RecordNewsletterSent(eventID string, recipients int) error {
	_, err := c.DB.Exec(
		`UPDATE newsletter_log SET recipient_count = ? WHERE event_id = ?`, recipients, eventID)
	return err
}

// NewsletterRecipients lists email addresses opted into the newsletter
// (active identities with a confirmed email and the opt-in flag).
func (c *Community) NewsletterRecipients() ([]string, error) {
	rows, err := c.DB.Query(
		`SELECT email FROM identities
		 WHERE status = 'active' AND newsletter = 1 AND email IS NOT NULL AND email != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
