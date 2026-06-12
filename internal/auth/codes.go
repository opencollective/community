// Package auth implements email codes and web sessions
// (docs/flows/login.md). Codes are 6 digits, stored only as argon2id
// hashes, single-use, expiring, attempt-limited, rate-limited.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/internal/store"
)

const (
	codeTTL      = 10 * time.Minute
	maxAttempts  = 5
	maxPerWindow = 3
	rateWindow   = time.Hour
)

var (
	ErrRateLimited = errors.New("auth: too many codes requested — try again later")
	ErrBadCode     = errors.New("auth: invalid or expired code")
)

// CreateCode issues a 6-digit code for email+purpose and returns it for
// sending. Enforces the per-address rate limit (LOGIN-05).
func CreateCode(c *store.Community, email, purpose string, p crypto.Argon2Params, now time.Time) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var recent int
	err := c.DB.QueryRow(
		`SELECT COUNT(*) FROM email_codes WHERE email = ? AND purpose = ? AND created_at > ?`,
		email, purpose, now.Add(-rateWindow).Unix(),
	).Scan(&recent)
	if err != nil {
		return "", err
	}
	if recent >= maxPerWindow {
		return "", ErrRateLimited
	}

	code, err := sixDigits()
	if err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := append(salt, hashCode(code, salt, p)...)

	// A fresh code supersedes earlier ones for the same email+purpose.
	if _, err := c.DB.Exec(`DELETE FROM email_codes WHERE email = ? AND purpose = ? AND expires_at < ?`,
		email, purpose, now.Unix()); err != nil {
		return "", err
	}
	_, err = c.DB.Exec(
		`INSERT INTO email_codes (email, code_hash, purpose, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		email, hash, purpose, now.Add(codeTTL).Unix(), now.Unix(),
	)
	if err != nil {
		return "", err
	}
	return code, nil
}

// VerifyCode consumes a code. Wrong attempts are counted; the 5th
// invalidates the code (LOGIN-02). Expired codes fail (LOGIN-03). The
// response is identical whether the email is known or not (LOGIN-04 is the
// caller's concern; this layer just never reveals which part failed).
func VerifyCode(c *store.Community, email, purpose, code string, p crypto.Argon2Params, now time.Time) error {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.TrimSpace(code)

	rows, err := c.DB.Query(
		`SELECT id, code_hash, attempts, expires_at FROM email_codes
		 WHERE email = ? AND purpose = ? ORDER BY created_at DESC`,
		email, purpose,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, attempts int64
		var hash []byte
		var expires int64
		if err := rows.Scan(&id, &hash, &attempts, &expires); err != nil {
			return err
		}
		if now.Unix() > expires || attempts >= maxAttempts || len(hash) <= 16 {
			continue
		}
		salt, want := hash[:16], hash[16:]
		if subtle.ConstantTimeCompare(hashCode(code, salt, p), want) == 1 {
			rows.Close()
			_, err := c.DB.Exec(`DELETE FROM email_codes WHERE id = ?`, id)
			return err
		}
		rows.Close()
		_, _ = c.DB.Exec(`UPDATE email_codes SET attempts = attempts + 1 WHERE id = ?`, id)
		return ErrBadCode
	}
	return ErrBadCode
}

func hashCode(code string, salt []byte, p crypto.Argon2Params) []byte {
	return argon2.IDKey([]byte(code), salt, p.Time, p.MemoryKiB, p.Threads, 32)
}

func sixDigits() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
