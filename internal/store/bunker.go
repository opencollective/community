package store

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"
)

// Bunker state (docs/architecture/bunker.md).

// SetBunkerKey stores an identity's bunker keypair (secret encrypted).
func (c *Community) SetBunkerKey(identityID int64, pubkey string, nsecEnc []byte) error {
	_, err := c.DB.Exec(
		`UPDATE identities SET bunker_pubkey = ?, bunker_nsec_enc = ? WHERE id = ?`,
		pubkey, nsecEnc, identityID)
	return err
}

// BunkerKey returns an identity's bunker keypair material, or ErrNotFound.
func (c *Community) BunkerKey(identityID int64) (pubkey string, nsecEnc []byte, err error) {
	var pk sql.NullString
	err = c.DB.QueryRow(
		`SELECT bunker_pubkey, bunker_nsec_enc FROM identities WHERE id = ?`,
		identityID).Scan(&pk, &nsecEnc)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!pk.Valid || pk.String == "")) {
		return "", nil, ErrNotFound
	}
	return pk.String, nsecEnc, err
}

// BunkerIdentities lists identities with bunker keys: pubkey → identity id.
func (c *Community) BunkerIdentities() (map[string]int64, error) {
	rows, err := c.DB.Query(
		`SELECT bunker_pubkey, id FROM identities WHERE bunker_pubkey IS NOT NULL AND bunker_pubkey != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var pk string
		var id int64
		if err := rows.Scan(&pk, &id); err != nil {
			return nil, err
		}
		out[pk] = id
	}
	return out, rows.Err()
}

// CreateBunkerSecret stores the hash of a one-time connect secret
// (BUNKER-01: 10 minutes, bound to one identity).
func (c *Community) CreateBunkerSecret(identityID int64, secret string, ttl time.Duration, now time.Time) error {
	h := sha256.Sum256([]byte(secret))
	_, err := c.DB.Exec(
		`INSERT INTO bunker_secrets (identity_id, secret_hash, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		identityID, h[:], now.Add(ttl).Unix(), now.Unix())
	return err
}

// ConsumeBunkerSecret validates and burns a connect secret (BUNKER-02).
func (c *Community) ConsumeBunkerSecret(identityID int64, secret string, now time.Time) bool {
	h := sha256.Sum256([]byte(secret))
	res, err := c.DB.Exec(
		`DELETE FROM bunker_secrets WHERE identity_id = ? AND secret_hash = ? AND expires_at >= ?`,
		identityID, h[:], now.Unix())
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// BunkerSession is one connected external app.
type BunkerSession struct {
	ID           int64
	IdentityID   int64
	ClientPubkey string
	AppName      string
	CreatedAt    int64
	LastUsedAt   int64
	Revoked      bool
}

// CreateBunkerSession opens a session for a client pubkey.
func (c *Community) CreateBunkerSession(identityID int64, clientPubkey, appName string, now time.Time) error {
	_, err := c.DB.Exec(
		`INSERT INTO bunker_sessions (identity_id, client_pubkey, app_name, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?)`,
		identityID, clientPubkey, appName, now.Unix(), now.Unix())
	return err
}

// ActiveBunkerSession reports whether a client holds a live session for
// the identity, touching last_used_at when it does.
func (c *Community) ActiveBunkerSession(identityID int64, clientPubkey string, now time.Time) bool {
	var id int64
	err := c.DB.QueryRow(
		`SELECT id FROM bunker_sessions
		 WHERE identity_id = ? AND client_pubkey = ? AND revoked_at IS NULL`,
		identityID, clientPubkey).Scan(&id)
	if err != nil {
		return false
	}
	_, _ = c.DB.Exec(`UPDATE bunker_sessions SET last_used_at = ? WHERE id = ?`, now.Unix(), id)
	return true
}

// BunkerSessions lists an identity's sessions, newest first.
func (c *Community) BunkerSessions(identityID int64) ([]*BunkerSession, error) {
	rows, err := c.DB.Query(
		`SELECT id, identity_id, client_pubkey, app_name, created_at, last_used_at, revoked_at
		 FROM bunker_sessions WHERE identity_id = ? ORDER BY created_at DESC`, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*BunkerSession
	for rows.Next() {
		var s BunkerSession
		var revoked sql.NullInt64
		if err := rows.Scan(&s.ID, &s.IdentityID, &s.ClientPubkey, &s.AppName,
			&s.CreatedAt, &s.LastUsedAt, &revoked); err != nil {
			return nil, err
		}
		s.Revoked = revoked.Valid
		out = append(out, &s)
	}
	return out, rows.Err()
}

// RevokeBunkerSession revokes one session, scoped to its owner (BUNKER-05).
func (c *Community) RevokeBunkerSession(identityID, sessionID int64, now time.Time) error {
	_, err := c.DB.Exec(
		`UPDATE bunker_sessions SET revoked_at = ? WHERE id = ? AND identity_id = ?`,
		now.Unix(), sessionID, identityID)
	return err
}
