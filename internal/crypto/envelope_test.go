package crypto

import (
	"bytes"
	"testing"
)

func TestWrapUnwrapRoundTrip(t *testing.T) {
	dek, err := NewDEK()
	if err != nil {
		t.Fatal(err)
	}
	blob, err := WrapDEK(dek, []byte("correct horse"), TestArgon2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnwrapDEK(blob, []byte("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, dek) {
		t.Fatal("unwrapped DEK differs")
	}
}

func TestWrongPasswordFails(t *testing.T) {
	dek, _ := NewDEK()
	blob, _ := WrapDEK(dek, []byte("right"), TestArgon2)
	if _, err := UnwrapDEK(blob, []byte("wrong")); err != ErrWrongKey {
		t.Fatalf("want ErrWrongKey, got %v", err)
	}
}

// TestKEY02_RotationRewrapsOnlyTheDEK pins the rotation property of
// docs/testing/cases/keys.md KEY-02: rotating the master password re-wraps
// the DEK; secrets encrypted under it are untouched and still decrypt.
func TestKEY02_RotationRewrapsOnlyTheDEK(t *testing.T) {
	dek, _ := NewDEK()
	oldBlob, _ := WrapDEK(dek, []byte("old password"), TestArgon2)
	secret, err := Encrypt(dek, []byte("nsec1xyz"), []byte("identity:42"))
	if err != nil {
		t.Fatal(err)
	}

	// Rotation: unwrap with old, re-wrap with new. One blob changes.
	got, err := UnwrapDEK(oldBlob, []byte("old password"))
	if err != nil {
		t.Fatal(err)
	}
	newBlob, err := WrapDEK(got, []byte("new password"), TestArgon2)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := UnwrapDEK(newBlob, []byte("old password")); err != ErrWrongKey {
		t.Fatalf("old password must be invalid after rotation, got %v", err)
	}
	dek2, err := UnwrapDEK(newBlob, []byte("new password"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := Decrypt(dek2, secret, []byte("identity:42"))
	if err != nil || string(pt) != "nsec1xyz" {
		t.Fatalf("secret must survive rotation untouched: %v", err)
	}
}

// TestKEY06_AADBindsCiphertextToRow pins KEY-06: a ciphertext moved to a
// different row (different AAD) fails loudly.
func TestKEY06_AADBindsCiphertextToRow(t *testing.T) {
	dek, _ := NewDEK()
	blob, _ := Encrypt(dek, []byte("nsec1xyz"), []byte("identity:1"))
	if _, err := Decrypt(dek, blob, []byte("identity:2")); err != ErrWrongKey {
		t.Fatalf("swapped AAD must fail, got %v", err)
	}
	if _, err := Decrypt(dek, blob, []byte("identity:1")); err != nil {
		t.Fatalf("original AAD must succeed: %v", err)
	}
}

func TestMachineKeyWrap(t *testing.T) {
	dek, _ := NewDEK()
	mk, _ := NewMachineKey()
	blob, err := WrapDEKWithKey(dek, mk)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnwrapDEKWithKey(blob, mk)
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("machine unwrap failed: %v", err)
	}
	other, _ := NewMachineKey()
	if _, err := UnwrapDEKWithKey(blob, other); err != ErrWrongKey {
		t.Fatalf("wrong machine key must fail, got %v", err)
	}
}

func TestTamperedBlobsFail(t *testing.T) {
	dek, _ := NewDEK()
	blob, _ := WrapDEK(dek, []byte("pw"), TestArgon2)
	blob[len(blob)-1] ^= 0xFF
	if _, err := UnwrapDEK(blob, []byte("pw")); err != ErrWrongKey {
		t.Fatalf("tampered blob must fail, got %v", err)
	}
	if _, err := UnwrapDEK([]byte{0x09, 0x00}, []byte("pw")); err != ErrCorrupt {
		t.Fatalf("garbage blob must be ErrCorrupt, got %v", err)
	}
}
