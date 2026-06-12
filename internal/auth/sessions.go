package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/opencollective/community/internal/store"
)

const sessionTTL = 30 * 24 * time.Hour

// CreateSession opens a web session for an identity and returns the cookie
// token. Only its hash is stored.
func CreateSession(c *store.Community, identityID int64, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	h := sha256.Sum256(raw)
	_, err := c.DB.Exec(
		`INSERT INTO web_sessions (token_hash, identity_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		h[:], identityID, now.Unix(), now.Add(sessionTTL).Unix(),
	)
	return token, err
}

// SessionIdentity resolves a cookie token to an identity id, sliding the
// expiry (docs/flows/login.md).
func SessionIdentity(c *store.Community, token string, now time.Time) (int64, bool) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return 0, false
	}
	h := sha256.Sum256(raw)
	var id, expires int64
	err = c.DB.QueryRow(
		`SELECT identity_id, expires_at FROM web_sessions WHERE token_hash = ?`, h[:],
	).Scan(&id, &expires)
	if errors.Is(err, sql.ErrNoRows) || err != nil || now.Unix() > expires {
		return 0, false
	}
	_, _ = c.DB.Exec(`UPDATE web_sessions SET expires_at = ? WHERE token_hash = ?`,
		now.Add(sessionTTL).Unix(), h[:])
	return id, true
}
