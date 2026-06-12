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
	Name           string
	Email          string
	Pubkey         string
	NsecEnc        []byte
	IsOrganization bool
	Newsletter     bool
	Status         string // unconfirmed | unclaimed | pending | active
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

func (c *Community) IdentityByPubkey(pubkey string) (*Identity, error) {
	return c.identityWhere(`pubkey = ?`, pubkey)
}

// SetMuted flips an identity's chat mute (CHAT-05).
func (c *Community) SetMuted(id int64, muted bool) error {
	_, err := c.DB.Exec(`UPDATE identities SET muted = ? WHERE id = ?`, boolInt(muted), id)
	return err
}

// Muted reports an identity's mute state.
func (c *Community) Muted(id int64) (bool, error) {
	var m int
	err := c.DB.QueryRow(`SELECT muted FROM identities WHERE id = ?`, id).Scan(&m)
	return m == 1, err
}

func (c *Community) identityWhere(where string, arg any) (*Identity, error) {
	var i Identity
	var email sql.NullString
	var isOrg, nl int
	err := c.DB.QueryRow(
		`SELECT id, username, name, email, pubkey, nsec_enc, is_organization, newsletter, status
		 FROM identities WHERE `+where, arg,
	).Scan(&i.ID, &i.Username, &i.Name, &email, &i.Pubkey, &i.NsecEnc, &isOrg, &nl, &i.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	i.Email = email.String
	i.IsOrganization = isOrg == 1
	i.Newsletter = nl == 1
	return &i, nil
}

// UpdateProfile sets the display fields an identity owner controls.
func (c *Community) UpdateProfile(id int64, name string, newsletter bool) error {
	_, err := c.DB.Exec(`UPDATE identities SET name = ?, newsletter = ? WHERE id = ?`,
		name, boolInt(newsletter), id)
	return err
}

// SetIdentityStatus moves an identity through its lifecycle.
func (c *Community) SetIdentityStatus(id int64, status string) error {
	_, err := c.DB.Exec(`UPDATE identities SET status = ? WHERE id = ?`, status, id)
	return err
}

// GCUnconfirmed deletes follower identities never confirmed within the
// window (FOLLOW-04). Key material goes with them.
func (c *Community) GCUnconfirmed(cutoff time.Time) error {
	_, err := c.DB.Exec(
		`DELETE FROM identities WHERE status = 'unconfirmed' AND created_at < ?`, cutoff.Unix())
	return err
}

// MemberRows lists active member-level identities for the directory,
// optionally filtered (case-insensitive substring on username and name).
func (c *Community) MemberRows(q string) ([]*Identity, error) {
	rows, err := c.DB.Query(`
		SELECT DISTINCT i.id, i.username, i.name, COALESCE(i.email,''), i.pubkey, i.is_organization, i.status
		FROM identities i
		JOIN role_members m ON m.identity_id = i.id
		JOIN roles r ON r.id = m.role_id
		WHERE i.status = 'active' AND r.name IN ('member','steward','moderator','fiscal host')
		  AND (i.username LIKE '%' || ? || '%' OR i.name LIKE '%' || ? || '%' COLLATE NOCASE)
		ORDER BY i.created_at`, q, q)
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
