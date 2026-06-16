//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/tests/harness"
)

// TestLOG01_LogIsAdminOnly pins LOG-01.
func TestLOG01_LogIsAdminOnly(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	steward := h.Member("alice", "steward")
	member := h.Member("dan")

	resp, _ := h.Admin.Get(h.Server.URL + "/log")
	if resp.StatusCode != 200 {
		t.Fatalf("admin must see the log, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	for _, cl := range []*http.Client{steward, member, h.Client()} {
		resp, _ := cl.Get(h.Server.URL + "/log")
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Fatal("non-admins must not see the log")
		}
	}
}

// TestLOG02_HumanReadableRenderings pins LOG-02.
func TestLOG02_HumanReadableRenderings(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")

	// Generate a published announcement (proposal + approval + community publish).
	id := propose(t, h, alice, "announcement", "", "Logged announcement.")
	r, _ := h.Admin.PostForm(h.Server.URL+"/posts/pending/"+id+"/approve", nil)
	r.Body.Close()

	page := body(t, mustGet(t, h.Admin, h.Server.URL+"/log"))
	wants := []string{
		"@alice proposed an announcement",
		"the community published an announcement",
		"@alice followed the community",
		"approved a proposal",
	}
	for _, want := range wants {
		if !strings.Contains(page, want) {
			t.Fatalf("the log must render %q", want)
		}
	}
}

// TestLOG03_JSONInspector pins LOG-03.
func TestLOG03_JSONInspector(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)

	page := body(t, mustGet(t, h.Admin, h.Server.URL+"/log"))
	// The raw JSON inspector exposes signed events (HTML-escaped).
	if !strings.Contains(page, "pubkey") || !strings.Contains(page, "sig") {
		t.Fatal("the log must expose raw event JSON fields")
	}
	// The exact community kind 0 event (id + valid signature) must appear.
	community := communityPub(t, h)
	events := h.QueryRelayAs("xavier", nostr.Filter{Authors: []string{community}, Kinds: []int{0}})
	if len(events) == 0 {
		t.Fatal("no community event to verify")
	}
	if ok, _ := events[0].CheckSignature(); !ok {
		t.Fatal("logged events must carry valid signatures")
	}
	if !strings.Contains(page, events[0].ID) {
		t.Fatal("the JSON inspector must show the exact event by id")
	}
	_ = json.Marshal
}

// TestLOG04_Filters pins the filter half of LOG-04.
func TestLOG04_Filters(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	id := propose(t, h, alice, "announcement", "", "Filterable.")
	r, _ := h.Admin.PostForm(h.Server.URL+"/posts/pending/"+id+"/approve", nil)
	r.Body.Close()

	// The members filter shows follows, not publishing lines.
	page := body(t, mustGet(t, h.Admin, h.Server.URL+"/log?"+url.Values{"category": {"members"}}.Encode()))
	if !strings.Contains(page, "followed the community") {
		t.Fatal("the members filter must show membership activity")
	}
	if strings.Contains(page, "published an announcement") {
		t.Fatal("the members filter must exclude publishing activity")
	}
	// Author filter.
	page = body(t, mustGet(t, h.Admin, h.Server.URL+"/log?author=alice"))
	if !strings.Contains(page, "@alice") {
		t.Fatal("the author filter must show that author's activity")
	}
}

// TestLOG05_SurvivesTheAppDatabase pins LOG-05.
func TestLOG05_SurvivesTheAppDatabase(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	id := propose(t, h, alice, "announcement", "", "Durable line.")
	r, _ := h.Admin.PostForm(h.Server.URL+"/posts/pending/"+id+"/approve", nil)
	r.Body.Close()

	before := body(t, mustGet(t, h.Admin, h.Server.URL+"/log"))
	h.Restart()
	after := body(t, mustGet(t, h.Admin, h.Server.URL+"/log"))
	if !strings.Contains(before, "published an announcement") ||
		!strings.Contains(after, "published an announcement") {
		t.Fatal("the log must render the same after a restart (it is the relay)")
	}
}
