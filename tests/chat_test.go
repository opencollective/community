//go:build integration

package tests

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/tests/harness"
)

func requireZooid(t *testing.T, h *harness.H) {
	t.Helper()
	if !h.HasZooid {
		t.Skip("bin/zooid missing — run `make zooid`")
	}
}

// TestCHAT01_MessagesAreSignedGroupEvents pins CHAT-01.
func TestCHAT01_MessagesAreSignedGroupEvents(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")

	resp, err := dan.PostForm(h.Server.URL+"/chat", url.Values{"content": {"hello commons"}})
	if err != nil {
		t.Fatal(err)
	}
	page := body(t, resp)
	if !strings.Contains(page, "hello commons") || !strings.Contains(page, "@dan") {
		t.Fatal("the message must render with dan's name")
	}

	c := h.Community()
	ident, _ := c.IdentityByUsername("dan")
	events := h.QueryRelayAs("dan", nostr.Filter{
		Kinds: []int{nostr.KindSimpleGroupChatMessage},
		Tags:  nostr.TagMap{"h": []string{"general"}},
	})
	var found *nostr.Event
	for _, evt := range events {
		if evt.Content == "hello commons" {
			found = evt
		}
	}
	if found == nil {
		t.Fatal("kind 9 message missing from the relay")
	}
	if found.PubKey != ident.Pubkey {
		t.Fatal("the message must be signed with dan's own key")
	}
	if ok, _ := found.CheckSignature(); !ok {
		t.Fatal("invalid signature")
	}
}

// TestCHAT02_ChatIsInvisibleToOutsiders pins CHAT-02.
func TestCHAT02_ChatIsInvisibleToOutsiders(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	resp, _ := dan.PostForm(h.Server.URL+"/chat", url.Values{"content": {"members only secret"}})
	resp.Body.Close()

	// Visitors get the teaser, not the channel.
	resp = h.Get("/")
	if page := body(t, resp); strings.Contains(page, "members only secret") ||
		!strings.Contains(page, "apply to join the conversation") {
		t.Fatal("visitors must see the teaser, never messages")
	}
	resp = h.Get("/chat")
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Fatalf("anonymous /chat must redirect to login, got %d %q", resp.StatusCode, loc)
	}

	// An unauthenticated relay subscription receives nothing.
	ctx, cancel := contextWithTimeout(t)
	defer cancel()
	relay, err := nostr.RelayConnect(ctx, "ws"+strings.TrimPrefix(h.Server.URL, "http")+"/relay")
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	events, _ := relay.QuerySync(ctx, nostr.Filter{
		Kinds: []int{nostr.KindSimpleGroupChatMessage},
	})
	if len(events) != 0 {
		t.Fatalf("outsiders must receive no group events, got %d", len(events))
	}
}

// TestCHAT03_HistorySurvivesRestarts pins CHAT-03.
func TestCHAT03_HistorySurvivesRestarts(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	resp, _ := dan.PostForm(h.Server.URL+"/chat", url.Values{"content": {"before the restart"}})
	resp.Body.Close()

	h.Restart()

	// A fresh login (the old cookie client survives, but be thorough).
	resp2, err := dan.Get(h.Server.URL + "/chat")
	if err != nil {
		t.Fatal(err)
	}
	if page := body(t, resp2); !strings.Contains(page, "before the restart") {
		t.Fatal("history must survive a communityd restart — the relay is the storage")
	}
}

var msgIDRe = regexp.MustCompile(`/chat/delete/([0-9a-f]{64})`)

// TestCHAT04_ModeratorsRemoveMessages pins CHAT-04.
func TestCHAT04_ModeratorsRemoveMessages(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	carol := h.Member("carol", "moderator")

	resp, _ := dan.PostForm(h.Server.URL+"/chat", url.Values{"content": {"regrettable take"}})
	resp.Body.Close()

	// Carol sees a remove control; extract the event id from her view.
	resp2, _ := carol.Get(h.Server.URL + "/chat")
	page := body(t, resp2)
	m := msgIDRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("moderator must see remove controls:\n%s", page)
	}
	eventID := m[1]

	// Dan (no permission) cannot remove.
	resp3, _ := dan.PostForm(h.Server.URL+"/chat/delete/"+eventID, nil)
	resp3.Body.Close()
	if resp3.StatusCode != 404 {
		t.Fatalf("non-moderator removal must 404, got %d", resp3.StatusCode)
	}

	// Carol removes it — signed with her key.
	resp4, _ := carol.PostForm(h.Server.URL+"/chat/delete/"+eventID, nil)
	resp4.Body.Close()
	resp5, _ := carol.Get(h.Server.URL + "/chat")
	if page := body(t, resp5); strings.Contains(page, "regrettable take") {
		t.Fatal("the removed message must disappear from the channel")
	}
}

// TestCHAT05_MutedMembersCannotPost pins CHAT-05.
func TestCHAT05_MutedMembersCannotPost(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	carol := h.Member("carol", "moderator")

	resp, _ := carol.PostForm(h.Server.URL+"/chat/mute/dan", url.Values{"muted": {"1"}})
	resp.Body.Close()

	resp2, _ := dan.PostForm(h.Server.URL+"/chat", url.Values{"content": {"can you hear me"}})
	if page := body(t, resp2); !strings.Contains(page, "muted") {
		t.Fatal("a muted member must be told")
	}
	events := h.QueryRelayAs("carol", nostr.Filter{
		Kinds: []int{nostr.KindSimpleGroupChatMessage},
		Tags:  nostr.TagMap{"h": []string{"general"}},
	})
	for _, evt := range events {
		if evt.Content == "can you hear me" {
			t.Fatal("the muted message must never reach the relay")
		}
	}
	// Unmute restores posting.
	resp3, _ := carol.PostForm(h.Server.URL+"/chat/mute/dan", url.Values{"muted": {"0"}})
	resp3.Body.Close()
	resp4, _ := dan.PostForm(h.Server.URL+"/chat", url.Values{"content": {"back again"}})
	if page := body(t, resp4); !strings.Contains(page, "back again") {
		t.Fatal("unmuting must restore posting")
	}
}

// TestCHAT06_ExternalNIP29ClientSeesTheSameChannel pins CHAT-06: a plain
// go-nostr client (standing in for Flotilla) authed as a member reads and
// writes the same group through the public relay endpoint.
func TestCHAT06_ExternalNIP29ClientSeesTheSameChannel(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	resp, _ := dan.PostForm(h.Server.URL+"/chat", url.Values{"content": {"hello from the web"}})
	resp.Body.Close()

	// Read the same history as dan over raw nostr.
	events := h.QueryRelayAs("dan", nostr.Filter{
		Kinds: []int{nostr.KindSimpleGroupChatMessage},
		Tags:  nostr.TagMap{"h": []string{"general"}},
	})
	var sawWeb bool
	for _, evt := range events {
		if evt.Content == "hello from the web" {
			sawWeb = true
		}
	}
	if !sawWeb {
		t.Fatal("the external client must see web messages")
	}

	// Post over raw nostr; the web UI must show it.
	h.PublishRelayAs(t, "dan", &nostr.Event{
		Kind:    nostr.KindSimpleGroupChatMessage,
		Tags:    nostr.Tags{{"h", "general"}},
		Content: "hello from flotilla",
	})
	resp2, _ := dan.Get(h.Server.URL + "/chat")
	if page := body(t, resp2); !strings.Contains(page, "hello from flotilla") {
		t.Fatal("the web channel must show externally posted messages")
	}
}

// TestCHAT07_NewMembersGainAccessAutomatically pins CHAT-07.
func TestCHAT07_NewMembersGainAccessAutomatically(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	marie := h.Member("marie")

	resp, _ := marie.PostForm(h.Server.URL+"/chat", url.Values{"content": {"fresh member here"}})
	if page := body(t, resp); !strings.Contains(page, "fresh member here") {
		t.Fatal("a freshly approved member must post without further steps")
	}
}
