// Package bunker is the NIP-46 remote signer
// (docs/architecture/bunker.md). Private keys exist in plaintext only
// inside this package's call frames; nothing here ever returns or
// serializes a secret key (BUNKER-04).
package bunker

import (
	"fmt"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/internal/identity"
	"github.com/opencollective/community/internal/store"
)

// DEKFunc supplies the community's data encryption key; ok is false while
// the community is locked (strict mode after a restart).
type DEKFunc func() ([]byte, bool)

// Signer signs as identities of one community.
type Signer struct {
	C   *store.Community
	DEK DEKFunc
	Now func() time.Time
}

// identitySecret decrypts an identity's signing key. Internal on purpose.
func (s *Signer) identitySecret(ident *store.Identity) (string, error) {
	dek, ok := s.DEK()
	if !ok {
		return "", fmt.Errorf("bunker: community %s is locked", s.C.Slug)
	}
	sk, err := crypto.Decrypt(dek, ident.NsecEnc, []byte("nsec:"+ident.Pubkey))
	if err != nil {
		return "", err
	}
	return string(sk), nil
}

// SignAs signs an event with an identity's key — the single signing path
// for the web app's own actions (chat, approvals, profiles). The event's
// PubKey is forced to the identity's.
func (s *Signer) SignAs(ident *store.Identity, evt *nostr.Event) error {
	sk, err := s.identitySecret(ident)
	if err != nil {
		return err
	}
	evt.PubKey = ident.Pubkey
	if evt.CreatedAt == 0 {
		evt.CreatedAt = nostr.Timestamp(s.Now().Unix())
	}
	return evt.Sign(sk)
}

// EnsureBunkerKey returns the identity's bunker pubkey, generating and
// storing the keypair (encrypted, AAD-bound) on first use.
func (s *Signer) EnsureBunkerKey(ident *store.Identity) (string, error) {
	if pk, _, err := s.C.BunkerKey(ident.ID); err == nil {
		return pk, nil
	}
	dek, ok := s.DEK()
	if !ok {
		return "", fmt.Errorf("bunker: community %s is locked", s.C.Slug)
	}
	kp, err := identity.Generate()
	if err != nil {
		return "", err
	}
	enc, err := crypto.Encrypt(dek, []byte(kp.SecretHex), []byte("bunker:"+kp.PublicHex))
	if err != nil {
		return "", err
	}
	if err := s.C.SetBunkerKey(ident.ID, kp.PublicHex, enc); err != nil {
		return "", err
	}
	return kp.PublicHex, nil
}

// bunkerSecret decrypts an identity's bunker transport key.
func (s *Signer) bunkerSecret(identityID int64) (sk, pk string, err error) {
	pk, enc, err := s.C.BunkerKey(identityID)
	if err != nil {
		return "", "", err
	}
	dek, ok := s.DEK()
	if !ok {
		return "", "", fmt.Errorf("bunker: community %s is locked", s.C.Slug)
	}
	raw, err := crypto.Decrypt(dek, enc, []byte("bunker:"+pk))
	if err != nil {
		return "", "", err
	}
	return string(raw), pk, nil
}
