//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/tests/harness"
)

// TestSETUP11_CommunityEventsReachTheRelay pins SETUP-11's relay half:
// the community's kind 0 profile and NIP-72 definition are queryable from
// the data relay through the production path (proxy + auth + membership).
func TestSETUP11_CommunityEventsReachTheRelay(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	if !h.HasZooid {
		t.Skip("bin/zooid missing — run `make zooid`")
	}
	c := h.Community()
	community, err := c.IdentityByUsername("community")
	if err != nil {
		t.Fatal(err)
	}

	events := h.QueryRelayAs("xavier", nostr.Filter{
		Authors: []string{community.Pubkey},
		Kinds:   []int{nostr.KindProfileMetadata, publish.KindCommunityDefinition},
	})
	kinds := map[int]*nostr.Event{}
	for _, evt := range events {
		kinds[evt.Kind] = evt
	}
	profile := kinds[nostr.KindProfileMetadata]
	if profile == nil {
		t.Fatal("community kind 0 missing from the relay")
	}
	var meta struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(profile.Content), &meta); err != nil || meta.Name != "Commons Hub" {
		t.Fatalf("community profile content wrong: %s", profile.Content)
	}
	def := kinds[publish.KindCommunityDefinition]
	if def == nil {
		t.Fatal("kind 34550 community definition missing from the relay")
	}
	if tag := def.Tags.GetFirst([]string{"name", ""}); tag == nil || (*tag)[1] != "Commons Hub" {
		t.Fatal("definition must carry the community name")
	}
}

// TestJOIN05_MemberEventsReachTheRelay pins JOIN-05's relay half: an
// approved member's kind 0 and kind 3 (following the community) exist.
func TestJOIN05_MemberEventsReachTheRelay(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	if !h.HasZooid {
		t.Skip("bin/zooid missing — run `make zooid`")
	}
	h.Member("alice")
	c := h.Community()
	alice, _ := c.IdentityByUsername("alice")
	community, _ := c.IdentityByUsername("community")

	events := h.QueryRelayAs("alice", nostr.Filter{
		Authors: []string{alice.Pubkey},
		Kinds:   []int{nostr.KindProfileMetadata, nostr.KindFollowList},
	})
	var sawProfile, followsCommunity bool
	for _, evt := range events {
		switch evt.Kind {
		case nostr.KindProfileMetadata:
			sawProfile = true
		case nostr.KindFollowList:
			if evt.Tags.GetFirst([]string{"p", community.Pubkey}) != nil {
				followsCommunity = true
			}
		}
	}
	if !sawProfile || !followsCommunity {
		t.Fatalf("member relay events incomplete: profile=%v follow=%v", sawProfile, followsCommunity)
	}
}

// TestFOLLOW03_FollowerEventsReachTheRelay pins FOLLOW-03's relay half.
func TestFOLLOW03_FollowerEventsReachTheRelay(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	if !h.HasZooid {
		t.Skip("bin/zooid missing — run `make zooid`")
	}
	resp := h.PostForm("/follow", url.Values{"email": {"marie@example.org"}})
	resp.Body.Close()
	msg, _ := h.Mailer.LastTo("marie@example.org")
	resp = h.Get(extractLink(t, msg.Text))
	resp.Body.Close()

	c := h.Community()
	marie, _ := c.IdentityByEmail("marie@example.org")
	community, _ := c.IdentityByUsername("community")

	events := h.QueryRelayAs("xavier", nostr.Filter{
		Authors: []string{marie.Pubkey},
		Kinds:   []int{nostr.KindFollowList},
	})
	if len(events) == 0 || events[0].Tags.GetFirst([]string{"p", community.Pubkey}) == nil {
		t.Fatal("the confirmed follower's kind 3 must follow the community")
	}
}

// TestNIP05Resolution pins the NIP-05 halves of SETUP-11 and JOIN-05:
// username@domain resolves for the community (_), the admin and members.
func TestNIP05Resolution(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	h.Member("alice")
	c := h.Community()

	get := func(name string) map[string]any {
		t.Helper()
		resp := h.Get("/.well-known/nostr.json?name=" + name)
		defer resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("NIP-05 content type: %s", ct)
		}
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	community, _ := c.IdentityByUsername("community")
	alice, _ := c.IdentityByUsername("alice")

	names := get("_")["names"].(map[string]any)
	if names["_"] != community.Pubkey {
		t.Fatalf("_ must resolve to the community: %v", names)
	}
	names = get("alice")["names"].(map[string]any)
	if names["alice"] != alice.Pubkey {
		t.Fatalf("alice must resolve: %v", names)
	}
	// Pending and unconfirmed identities don't resolve.
	resp := h.PostForm("/follow", url.Values{"email": {"ghost@example.org"}})
	resp.Body.Close()
	ghost, _ := c.IdentityByEmail("ghost@example.org")
	names = get(ghost.Username)["names"].(map[string]any)
	if _, ok := names[ghost.Username]; ok {
		t.Fatal("unconfirmed identities must not resolve")
	}
}

// TestRelayWriteRequiresMembership probes the access model end to end
// (the CHAT-02 / TEN-02 mechanism): a non-member key cannot write through
// the proxy.
func TestRelayWriteRequiresMembership(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	if !h.HasZooid {
		t.Skip("bin/zooid missing — run `make zooid`")
	}

	ctx, cancel := contextWithTimeout(t)
	defer cancel()
	relayURL := "ws" + strings.TrimPrefix(h.Server.URL, "http") + "/relay"
	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()

	sk := nostr.GeneratePrivateKey()
	evt := nostr.Event{Kind: nostr.KindTextNote, Content: "outsider", CreatedAt: nostr.Now()}
	if err := evt.Sign(sk); err != nil {
		t.Fatal(err)
	}
	err = relay.Publish(ctx, evt)
	if err == nil {
		// Maybe accepted pre-auth; authenticate as the outsider and retry.
		_ = relay.Auth(ctx, func(e *nostr.Event) error { return e.Sign(sk) })
		err = relay.Publish(ctx, evt)
	}
	if err == nil || !strings.Contains(err.Error(), "restricted") &&
		!strings.Contains(err.Error(), "auth-required") {
		t.Fatalf("an outsider's write must be rejected, got %v", err)
	}
}

func contextWithTimeout(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 15*time.Second)
}
