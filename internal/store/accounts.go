package store

import (
	"strings"
	"time"
)

// Unclaimed and managed accounts (docs/nostr/identities.md).

// AddManager records a management relationship; the creator of an
// on-behalf account is its first manager (MGD-01).
func (c *Community) AddManager(managedID, managerID, grantedBy int64, now time.Time) error {
	_, err := c.DB.Exec(
		`INSERT OR IGNORE INTO account_managers (identity_id, manager_id, granted_by, since)
		 VALUES (?, ?, ?, ?)`, managedID, managerID, grantedBy, now.Unix())
	return err
}

// IsManager reports whether managerID actively manages managedID (not
// paused by a claim).
func (c *Community) IsManager(managedID, managerID int64) (bool, error) {
	var n int
	err := c.DB.QueryRow(
		`SELECT COUNT(*) FROM account_managers WHERE identity_id = ? AND manager_id = ? AND paused = 0`,
		managedID, managerID).Scan(&n)
	return n > 0, err
}

// IsManaged reports whether an identity has any active manager (operable
// even while unclaimed — UNCL-05, MGD-07).
func (c *Community) IsManaged(managedID int64) (bool, error) {
	var n int
	err := c.DB.QueryRow(
		`SELECT COUNT(*) FROM account_managers WHERE identity_id = ? AND paused = 0`, managedID).Scan(&n)
	return n > 0, err
}

// ManagedBy lists the accounts a manager actively manages.
func (c *Community) ManagedBy(managerID int64) ([]*Identity, error) {
	rows, err := c.DB.Query(`
		SELECT i.id, i.username, i.name, COALESCE(i.email,''), i.pubkey, i.is_organization, i.status
		FROM identities i
		JOIN account_managers m ON m.identity_id = i.id
		WHERE m.manager_id = ? AND m.paused = 0 ORDER BY i.username`, managerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Identity
	for rows.Next() {
		var i Identity
		var isOrg int
		if err := rows.Scan(&i.ID, &i.Username, &i.Name, &i.Email, &i.Pubkey, &isOrg, &i.Status); err != nil {
			return nil, err
		}
		i.IsOrganization = isOrg == 1
		out = append(out, &i)
	}
	return out, rows.Err()
}

// PauseManagers pauses all management when an account is claimed (the new
// owner re-confirms managers explicitly; MGD-06).
func (c *Community) PauseManagers(managedID int64) error {
	_, err := c.DB.Exec(`UPDATE account_managers SET paused = 1 WHERE identity_id = ?`, managedID)
	return err
}

// SetClaimEmail records the address that may claim an account; replacing
// it invalidates any earlier pending claim (UNCL-04).
func (c *Community) SetClaimEmail(id int64, email string) error {
	_, err := c.DB.Exec(`UPDATE identities SET claim_email = ? WHERE id = ?`,
		strings.ToLower(strings.TrimSpace(email)), id)
	return err
}

// ClaimEmail returns an account's pending claim address.
func (c *Community) ClaimEmail(id int64) (string, error) {
	var e string
	err := c.DB.QueryRow(`SELECT claim_email FROM identities WHERE id = ?`, id).Scan(&e)
	return e, err
}

// ClaimIdentity binds the email, activates the account, and pauses
// management — the claimer takes control (UNCL-03).
func (c *Community) ClaimIdentity(id int64, email string) error {
	if _, err := c.DB.Exec(
		`UPDATE identities SET email = ?, status = 'active', claim_email = '' WHERE id = ?`,
		strings.ToLower(strings.TrimSpace(email)), id); err != nil {
		return err
	}
	return c.PauseManagers(id)
}

// SetCreatedByOrganization marks an account as an organization (used when
// a credit source becomes an unclaimed identity).
func (c *Community) UnclaimedByClaimEmail(email string) (*Identity, error) {
	return c.identityWhere(`claim_email = ? AND status = 'unclaimed'`,
		strings.ToLower(strings.TrimSpace(email)))
}
