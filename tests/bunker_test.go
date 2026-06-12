//go:build integration

package tests

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip46"

	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/tests/harness"
)

var bunkerURLRe = regexp.MustCompile(`bunker://[0-9a-f]{64}\?relay=[^&\s<]+&amp;secret=[0-9a-f]{32}|bunker://[0-9a-f]{64}\?relay=[^&\s<]+&secret=[0-9a-f]{32}`)

// generateBunkerURL drives /settings/apps for a logged-in client and
// returns the bunker URL from the page.
func generateBunkerURL(t *testing.T, h *harness.H, client *http.Client) string {
	t.Helper()
	resp, err := client.PostForm(h.Server.URL+"/settings/apps/url", nil)
	if err != nil {
		t.Fatal(err)
	}
	page := body(t, resp)
	m := bunkerURLRe.FindString(page)
	if m == "" {
		t.Fatalf("no bunker URL on the page:\n%s", page)
	}
	return strings.ReplaceAll(m, "&amp;", "&")
}

func connect(t *testing.T, h *harness.H, clientSK, bunkerURL string) (*nip46.BunkerClient, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return nip46.ConnectBunker(ctx, clientSK, bunkerURL, nil, nil)
}

// TestBUNKER01_MemberGeneratesABunkerURL pins BUNKER-01.
func TestBUNKER01_MemberGeneratesABunkerURL(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice")

	u := generateBunkerURL(t, h, alice)
	if !strings.Contains(u, "relay="+h.RelayURL) {
		t.Fatalf("bunker URL must carry the community relay: %s", u)
	}
	c := h.Community()
	ident, _ := c.IdentityByUsername("alice")
	pk, _, err := c.BunkerKey(ident.ID)
	if err != nil || !strings.Contains(u, pk) {
		t.Fatalf("bunker URL must carry alice's bunker pubkey: %v", err)
	}
	if pk == ident.Pubkey {
		t.Fatal("the bunker transport key must differ from the identity key")
	}
}

// TestBUNKER02_ConnectConsumesTheSecret pins BUNKER-02.
func TestBUNKER02_ConnectConsumesTheSecret(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice")
	u := generateBunkerURL(t, h, alice)

	if _, err := connect(t, h, nostr.GeneratePrivateKey(), u); err != nil {
		t.Fatalf("first connect must succeed: %v", err)
	}
	sessions, _ := h.Community().BunkerSessions(identityID(t, h, "alice"))
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	// The same secret cannot connect a second client.
	if _, err := connect(t, h, nostr.GeneratePrivateKey(), u); err == nil {
		t.Fatal("a consumed secret must not connect another client")
	}
}

// TestBUNKER02_SecretsExpire pins the expiry half of BUNKER-02.
func TestBUNKER02_SecretsExpire(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice")
	u := generateBunkerURL(t, h, alice)

	h.Clock.Advance(11 * time.Minute)
	if _, err := connect(t, h, nostr.GeneratePrivateKey(), u); err == nil {
		t.Fatal("an expired secret must not connect")
	}
}

// TestBUNKER03_ConnectedAppsSignThroughTheBunker pins BUNKER-03, with the
// no-key-material check of BUNKER-04 folded in.
func TestBUNKER03_ConnectedAppsSignThroughTheBunker(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice")
	u := generateBunkerURL(t, h, alice)

	bc, err := connect(t, h, nostr.GeneratePrivateKey(), u)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := h.Community()
	ident, _ := c.IdentityByUsername("alice")

	pk, err := bc.GetPublicKey(ctx)
	if err != nil || pk != ident.Pubkey {
		t.Fatalf("get_public_key: want alice's npub, got %q (%v)", pk, err)
	}

	evt := &nostr.Event{
		Kind:      nostr.KindTextNote,
		CreatedAt: nostr.Timestamp(h.Clock.Now().Unix()),
		Content:   "hello from an external app",
	}
	if err := bc.SignEvent(ctx, evt); err != nil {
		t.Fatal(err)
	}
	if evt.PubKey != ident.Pubkey {
		t.Fatal("the signature must be alice's")
	}
	if ok, err := evt.CheckSignature(); !ok || err != nil {
		t.Fatalf("invalid signature: %v", err)
	}

	// BUNKER-04 (focused form): the response carries no key material.
	dek, _ := h.App.DEK(c)
	secret, _ := crypto.Decrypt(dek, ident.NsecEnc, []byte("nsec:"+ident.Pubkey))
	raw, _ := evt.MarshalJSON()
	if strings.Contains(string(raw), string(secret)) {
		t.Fatal("signed event leaked the secret key")
	}
}

// TestBUNKER05_RevocationStopsSigning pins BUNKER-05.
func TestBUNKER05_RevocationStopsSigning(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice")
	u := generateBunkerURL(t, h, alice)

	bc, err := connect(t, h, nostr.GeneratePrivateKey(), u)
	if err != nil {
		t.Fatal(err)
	}
	c := h.Community()
	id := identityID(t, h, "alice")
	sessions, _ := c.BunkerSessions(id)
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}

	resp, err := alice.PostForm(
		h.Server.URL+"/settings/apps/revoke/"+itoa(sessions[0].ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	evt := &nostr.Event{Kind: nostr.KindTextNote, CreatedAt: nostr.Timestamp(h.Clock.Now().Unix())}
	if err := bc.SignEvent(ctx, evt); err == nil {
		t.Fatal("a revoked session must not sign")
	}
}

// TestBUNKER06_SessionsAreIsolatedPerIdentity pins BUNKER-06.
func TestBUNKER06_SessionsAreIsolatedPerIdentity(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice")
	bob := h.Member("bob")

	aliceURL := generateBunkerURL(t, h, alice)
	bobURL := generateBunkerURL(t, h, bob)

	clientSK := nostr.GeneratePrivateKey()
	bc, err := connect(t, h, clientSK, aliceURL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pk, err := bc.GetPublicKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	aliceIdent, _ := h.Community().IdentityByUsername("alice")
	if pk != aliceIdent.Pubkey {
		t.Fatal("the session must sign as alice and only alice")
	}

	// The same client key, pointed at bob's bunker pubkey with alice's
	// (already consumed) secret, gets nothing.
	swapped := strings.Replace(bobURL, "secret="+lastParam(bobURL, "secret"), "secret="+lastParam(aliceURL, "secret"), 1)
	if _, err := connect(t, h, clientSK, swapped); err == nil {
		t.Fatal("a secret must not work across identities")
	}
}

// TestBUNKER07_SessionsSurviveRestarts pins BUNKER-07.
func TestBUNKER07_SessionsSurviveRestarts(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice")
	u := generateBunkerURL(t, h, alice)

	clientSK := nostr.GeneratePrivateKey()
	if _, err := connect(t, h, clientSK, u); err != nil {
		t.Fatal(err)
	}

	h.Restart()

	// The same client (same key, stale secret in its saved URL) keeps its
	// session: reconnect acks without a fresh secret.
	bc, err := connect(t, h, clientSK, u)
	if err != nil {
		t.Fatalf("a known client must reconnect after a restart: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	evt := &nostr.Event{Kind: nostr.KindTextNote, CreatedAt: nostr.Timestamp(h.Clock.Now().Unix()), Content: "still here"}
	if err := bc.SignEvent(ctx, evt); err != nil {
		t.Fatalf("signing after restart: %v", err)
	}
	if ok, _ := evt.CheckSignature(); !ok {
		t.Fatal("invalid signature after restart")
	}
}

// TestBUNKER08_BothEncryptionGenerationsWork pins BUNKER-08.
func TestBUNKER08_BothEncryptionGenerationsWork(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice")
	u := generateBunkerURL(t, h, alice)

	bc, err := connect(t, h, nostr.GeneratePrivateKey(), u)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	third := nostr.GeneratePrivateKey()
	thirdPK, _ := nostr.GetPublicKey(third)

	for _, gen := range []struct{ enc, dec string }{
		{"nip44_encrypt", "nip44_decrypt"},
		{"nip04_encrypt", "nip04_decrypt"},
	} {
		ct, err := bc.RPC(ctx, gen.enc, []string{thirdPK, "the plan is commons"})
		if err != nil {
			t.Fatalf("%s: %v", gen.enc, err)
		}
		pt, err := bc.RPC(ctx, gen.dec, []string{thirdPK, ct})
		if err != nil || pt != "the plan is commons" {
			t.Fatalf("%s round trip: %q %v", gen.dec, pt, err)
		}
	}
}

func identityID(t *testing.T, h *harness.H, username string) int64 {
	t.Helper()
	ident, err := h.Community().IdentityByUsername(username)
	if err != nil {
		t.Fatal(err)
	}
	return ident.ID
}

func lastParam(u, key string) string {
	i := strings.Index(u, key+"=")
	if i < 0 {
		return ""
	}
	v := u[i+len(key)+1:]
	if j := strings.IndexByte(v, '&'); j >= 0 {
		v = v[:j]
	}
	return v
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
