// Package crypto implements the envelope encryption scheme of
// docs/architecture/key-management.md (ADR 0003, ADR 0009): one DEK per
// community encrypts every secret (XChaCha20-Poly1305 with row-bound AAD);
// the DEK is wrapped by a KEK derived from the master password (argon2id),
// and optionally by a random machine key for auto-unlock (ADR 0004).
package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	dekSize     = 32
	saltSize    = 16
	machineSize = 32

	blobV1Password = 0x01 // argon2id-wrapped
	blobV2RawKey   = 0x02 // wrapped with a raw 32-byte key (machine key)
)

var (
	// ErrWrongKey is returned when a password or key fails to unwrap or
	// decrypt. Deliberately indistinguishable from tampering.
	ErrWrongKey = errors.New("crypto: wrong key or corrupted data")
	// ErrCorrupt is returned for structurally invalid blobs.
	ErrCorrupt = errors.New("crypto: malformed blob")
)

// Argon2Params tunes the password KDF. Production parameters follow the
// spec; tests inject minimal legal parameters so wrapping stays real but
// fast (docs/testing/environment.md).
type Argon2Params struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// DefaultArgon2 matches docs/architecture/key-management.md.
var DefaultArgon2 = Argon2Params{Time: 3, MemoryKiB: 64 * 1024, Threads: 4}

// TestArgon2 is for test harnesses only.
var TestArgon2 = Argon2Params{Time: 1, MemoryKiB: 8, Threads: 1}

// NewDEK returns a fresh random data encryption key.
func NewDEK() ([]byte, error) {
	return randomBytes(dekSize)
}

// NewMachineKey returns a fresh random machine wrap key.
func NewMachineKey() ([]byte, error) {
	return randomBytes(machineSize)
}

// WrapDEK seals the DEK under a key derived from password.
// Blob layout: version(1) time(4) memKiB(4) threads(1) salt(16) nonce(24) sealed.
func WrapDEK(dek, password []byte, p Argon2Params) ([]byte, error) {
	if len(dek) != dekSize {
		return nil, fmt.Errorf("crypto: DEK must be %d bytes", dekSize)
	}
	salt, err := randomBytes(saltSize)
	if err != nil {
		return nil, err
	}
	kek := argon2.IDKey(password, salt, p.Time, p.MemoryKiB, p.Threads, dekSize)
	header := make([]byte, 0, 10+saltSize)
	header = append(header, blobV1Password)
	header = binary.BigEndian.AppendUint32(header, p.Time)
	header = binary.BigEndian.AppendUint32(header, p.MemoryKiB)
	header = append(header, p.Threads)
	header = append(header, salt...)
	return seal(kek, dek, header)
}

// UnwrapDEK reverses WrapDEK. A wrong password returns ErrWrongKey.
func UnwrapDEK(blob, password []byte) ([]byte, error) {
	if len(blob) < 10+saltSize || blob[0] != blobV1Password {
		return nil, ErrCorrupt
	}
	p := Argon2Params{
		Time:      binary.BigEndian.Uint32(blob[1:5]),
		MemoryKiB: binary.BigEndian.Uint32(blob[5:9]),
		Threads:   blob[9],
	}
	salt := blob[10 : 10+saltSize]
	kek := argon2.IDKey(password, salt, p.Time, p.MemoryKiB, p.Threads, dekSize)
	return open(kek, blob, 10+saltSize)
}

// WrapDEKWithKey seals the DEK under a raw 32-byte key (the machine key).
// Blob layout: version(1) nonce(24) sealed.
func WrapDEKWithKey(dek, key []byte) ([]byte, error) {
	if len(dek) != dekSize {
		return nil, fmt.Errorf("crypto: DEK must be %d bytes", dekSize)
	}
	return seal(key, dek, []byte{blobV2RawKey})
}

// UnwrapDEKWithKey reverses WrapDEKWithKey.
func UnwrapDEKWithKey(blob, key []byte) ([]byte, error) {
	if len(blob) < 1 || blob[0] != blobV2RawKey {
		return nil, ErrCorrupt
	}
	return open(key, blob, 1)
}

// Encrypt seals plaintext under the DEK, bound to aad (e.g. the identity
// row id) so ciphertexts cannot be swapped between rows (KEY-06).
// Blob layout: nonce(24) sealed.
func Encrypt(dek, plaintext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, err
	}
	nonce, err := randomBytes(aead.NonceSize())
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, nonce...)
	return aead.Seal(out, nonce, plaintext, aad), nil
}

// Decrypt reverses Encrypt. AAD mismatch or tampering returns ErrWrongKey.
func Decrypt(dek, blob, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, err
	}
	if len(blob) < aead.NonceSize()+aead.Overhead() {
		return nil, ErrCorrupt
	}
	nonce, ct := blob[:aead.NonceSize()], blob[aead.NonceSize():]
	pt, err := aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, ErrWrongKey
	}
	return pt, nil
}

func seal(key, plaintext, header []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce, err := randomBytes(aead.NonceSize())
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(header)+len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, header...)
	out = append(out, nonce...)
	// The header is authenticated: tampering with KDF params is detected.
	return aead.Seal(out, nonce, plaintext, header), nil
}

func open(key, blob []byte, headerLen int) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	if len(blob) < headerLen+aead.NonceSize()+aead.Overhead() {
		return nil, ErrCorrupt
	}
	header := blob[:headerLen]
	nonce := blob[headerLen : headerLen+aead.NonceSize()]
	ct := blob[headerLen+aead.NonceSize():]
	pt, err := aead.Open(nil, nonce, ct, header)
	if err != nil {
		return nil, ErrWrongKey
	}
	return pt, nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
