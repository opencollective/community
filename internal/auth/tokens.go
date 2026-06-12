package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"time"

	"github.com/opencollective/community/internal/store"
)

// Link tokens reuse the email_codes table for URL-borne confirmations
// (follow confirmation, future claims). They are 32 random bytes — sha256
// suffices, unlike guessable 6-digit codes.

// CreateLinkToken issues a token for email+purpose, valid for ttl.
func CreateLinkToken(c *store.Community, email, purpose string, ttl time.Duration, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	h := sha256.Sum256(raw)
	_, err := c.DB.Exec(
		`INSERT INTO email_codes (email, code_hash, purpose, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		email, h[:], purpose, now.Add(ttl).Unix(), now.Unix(),
	)
	return token, err
}

// VerifyLinkToken consumes a token. Single-use, expiring.
func VerifyLinkToken(c *store.Community, email, purpose, token string, now time.Time) error {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return ErrBadCode
	}
	h := sha256.Sum256(raw)
	rows, err := c.DB.Query(
		`SELECT id, code_hash, expires_at FROM email_codes WHERE email = ? AND purpose = ?`,
		email, purpose)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, expires int64
		var hash []byte
		if err := rows.Scan(&id, &hash, &expires); err != nil {
			return err
		}
		if now.Unix() > expires {
			continue
		}
		if subtle.ConstantTimeCompare(hash, h[:]) == 1 {
			rows.Close()
			_, err := c.DB.Exec(`DELETE FROM email_codes WHERE id = ?`, id)
			return err
		}
	}
	return ErrBadCode
}

// DeleteSession revokes one web session by its cookie token (LOGIN-06).
func DeleteSession(c *store.Community, token string) error {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil
	}
	h := sha256.Sum256(raw)
	_, err = c.DB.Exec(`DELETE FROM web_sessions WHERE token_hash = ?`, h[:])
	return err
}
