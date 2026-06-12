//go:build integration

package tests

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/tests/harness"
)

// TestKEY01_SecretsAreCiphertextAtRest pins KEY-01: the raw database file
// contains no plaintext nsec — verified against the *actual* secret, which
// the test recovers through the keyring like the bunker will.
func TestKEY01_SecretsAreCiphertextAtRest(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	c := h.Community()

	admin, err := c.IdentityByUsername("xavier")
	if err != nil {
		t.Fatal(err)
	}
	dek, ok := h.App.DEK(c)
	if !ok {
		t.Fatal("community must be unlocked")
	}
	secret, err := crypto.Decrypt(dek, admin.NsecEnc, []byte("nsec:"+admin.Pubkey))
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != 64 {
		t.Fatalf("expected 64-char hex secret, got %d bytes", len(secret))
	}

	// Flush the WAL so the main file holds everything, then scan it.
	if _, err := c.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(c.Dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), string(secret)) {
		t.Fatal("plaintext secret found in the database file")
	}
	if strings.Contains(string(raw), "nsec1") {
		t.Fatal("bech32 nsec found in the database file")
	}
	// Non-secret data stays readable (KEY-01's second half).
	if !strings.Contains(string(raw), "xavier") {
		t.Fatal("usernames must remain plaintext")
	}
}

// TestKEY03_AutoUnlockResumesAfterRestart pins KEY-03.
func TestKEY03_AutoUnlockResumesAfterRestart(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	h.Restart()

	if _, ok := h.App.DEK(h.Community()); !ok {
		t.Fatal("auto-unlock mode must recover the DEK after a restart")
	}
	resp := h.Get("/")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("homepage after restart: want 200, got %d", resp.StatusCode)
	}
}

// TestKEY04_StrictModeLocksUntilUnlock pins KEY-04.
func TestKEY04_StrictModeLocksUntilUnlock(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(true)
	h.Restart()
	c := h.Community()

	if _, ok := h.App.DEK(c); ok {
		t.Fatal("strict mode must be locked after a restart")
	}
	// Public pages still render.
	resp := h.Get("/")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("public homepage while locked: want 200, got %d", resp.StatusCode)
	}

	// Wrong password fails.
	resp = h.PostForm("/unlock", url.Values{"password": {"wrong password!"}})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Wrong master password") {
		t.Fatal("wrong password must be rejected at /unlock")
	}
	if _, ok := h.App.DEK(c); ok {
		t.Fatal("wrong password must not unlock")
	}

	// Right password unlocks; signing capability returns.
	resp = h.PostForm("/unlock", url.Values{"password": {harness.MasterPassword}})
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("unlock: want 303, got %d", resp.StatusCode)
	}
	if _, ok := h.App.DEK(c); !ok {
		t.Fatal("correct password must unlock")
	}
}
