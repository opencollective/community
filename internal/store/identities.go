package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Identity is one row of the identities table — a person, organization or
// the community itself (docs/nostr/identities.md).
type Identity struct {
	ID             int64
	Username       string
	Email          string
	Pubkey         string
	NsecEnc        []byte
	IsOrganization bool
	Status         string // unclaimed | pending | active
}

// CreateIdentity inserts a new identity with its encrypted secret.
func (c *Community) CreateIdentity(username, email, pubkey string, nsecEnc []byte, isOrg bool, status string, now time.Time) (*Identity, error) {
	res, err := c.DB.Exec(
		`INSERT INTO identities (username, email, pubkey, nsec_enc, is_organization, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		username, nullable(email), pubkey, nsecEnc, boolInt(isOrg), status, now.Unix(),
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return c.IdentityByID(id)
}

func (c *Community) IdentityByID(id int64) (*Identity, error) {
	return c.identityWhere(`id = ?`, id)
}

func (c *Community) IdentityByUsername(username string) (*Identity, error) {
	return c.identityWhere(`username = ?`, username)
}

func (c *Community) IdentityByEmail(email string) (*Identity, error) {
	return c.identityWhere(`email = ?`, strings.ToLower(strings.TrimSpace(email)))
}

func (c *Community) identityWhere(where string, arg any) (*Identity, error) {
	var i Identity
	var email sql.NullString
	var isOrg int
	err := c.DB.QueryRow(
		`SELECT id, username, email, pubkey, nsec_enc, is_organization, status
		 FROM identities WHERE `+where, arg,
	).Scan(&i.ID, &i.Username, &email, &i.Pubkey, &i.NsecEnc, &isOrg, &i.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	i.Email = email.String
	i.IsOrganization = isOrg == 1
	return &i, nil
}

// BindEmail attaches a verified email to an identity and activates it.
func (c *Community) BindEmail(id int64, email string) error {
	_, err := c.DB.Exec(
		`UPDATE identities SET email = ?, status = 'active' WHERE id = ?`,
		strings.ToLower(strings.TrimSpace(email)), id,
	)
	return err
}

// UsernameTaken reports whether a username exists (case-insensitive).
func (c *Community) UsernameTaken(username string) (bool, error) {
	var n int
	err := c.DB.QueryRow(`SELECT COUNT(*) FROM identities WHERE username = ?`, username).Scan(&n)
	return n > 0, err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
