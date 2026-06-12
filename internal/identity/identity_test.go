package identity

import (
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestGenerate(t *testing.T) {
	kp, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if len(kp.SecretHex) != 64 || len(kp.PublicHex) != 64 {
		t.Fatalf("unexpected key lengths: %d %d", len(kp.SecretHex), len(kp.PublicHex))
	}
	if !strings.HasPrefix(kp.Npub, "npub1") {
		t.Fatalf("npub encoding wrong: %s", kp.Npub)
	}
	pk, err := nostr.GetPublicKey(kp.SecretHex)
	if err != nil || pk != kp.PublicHex {
		t.Fatalf("public key not derivable from secret: %v", err)
	}
	kp2, _ := Generate()
	if kp2.SecretHex == kp.SecretHex {
		t.Fatal("two generated keys are identical")
	}
}

// TestSETUP07_UsernameRules pins the validation half of SETUP-07.
func TestSETUP07_UsernameRules(t *testing.T) {
	for _, bad := range []string{"a", "Aa", "has space", "admin", "setup", "Émile", "x", strings.Repeat("a", 31)} {
		if err := ValidateUsername(bad); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
	for _, good := range []string{"xavier", "an", "citizen-spring", "marie.curie", "bob_2"} {
		if err := ValidateUsername(good); err != nil {
			t.Errorf("expected %q to be accepted: %v", good, err)
		}
	}
}
