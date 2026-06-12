// Package identity generates and represents Nostr identities
// (docs/nostr/identities.md). Secret keys exist in plaintext only inside
// the bunker process; everywhere else they are envelope-encrypted blobs.
package identity

import (
	"fmt"
	"regexp"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// KeyPair holds a freshly generated identity. SecretHex must be encrypted
// (internal/crypto) before it touches any storage.
type KeyPair struct {
	SecretHex string
	PublicHex string
	Npub      string
}

// Generate creates a new secp256k1 keypair from crypto/rand.
func Generate() (KeyPair, error) {
	sk := nostr.GeneratePrivateKey()
	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		return KeyPair{}, fmt.Errorf("identity: derive public key: %w", err)
	}
	npub, err := nip19.EncodePublicKey(pk)
	if err != nil {
		return KeyPair{}, fmt.Errorf("identity: encode npub: %w", err)
	}
	return KeyPair{SecretHex: sk, PublicHex: pk, Npub: npub}, nil
}

var usernameRe = regexp.MustCompile(`^[a-z0-9_.-]{2,30}$`)

// Reserved usernames per docs/nostr/identities.md plus route names.
var reserved = map[string]bool{
	"admin": true, "community": true, "_": true, "root": true, "www": true,
	"members": true, "settings": true, "posts": true, "roles": true,
	"channels": true, "compose": true, "login": true, "logout": true,
	"join": true, "follow": true, "setup": true, "unlock": true,
	"treasury": true, "log": true, "relay": true, "upload": true,
	"media": true, "feed": true, "static": true,
}

// ValidateUsername enforces the username rules. The returned error is
// user-facing.
func ValidateUsername(u string) error {
	if !usernameRe.MatchString(u) {
		return fmt.Errorf("usernames are 2–30 characters: lowercase letters, digits, _ . -")
	}
	if reserved[u] {
		return fmt.Errorf("%q is reserved", u)
	}
	return nil
}
