package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// Members ICS feed tokens (docs/nostr/channels.md § events, EVT-11). Only
// the hash is stored; generating a new one replaces the old (regenerate
// invalidates the previous URL).

// FeedToken returns the identity's current token, creating one on first
// use.
func (c *Community) FeedToken(identityID int64, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	h := sha256.Sum256(raw)
	_, err := c.DB.Exec(
		`INSERT INTO feed_tokens (identity_id, token_hash, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(identity_id) DO UPDATE SET token_hash = excluded.token_hash, created_at = excluded.created_at`,
		identityID, h[:], now.Unix())
	return token, err
}

// IdentityByFeedToken resolves a feed token to its identity id, or
// ErrNotFound.
func (c *Community) IdentityByFeedToken(token string) (int64, error) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return 0, ErrNotFound
	}
	h := sha256.Sum256(raw)
	var id int64
	err = c.DB.QueryRow(`SELECT identity_id FROM feed_tokens WHERE token_hash = ?`, h[:]).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}
