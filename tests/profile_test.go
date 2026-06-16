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

// proposeProfile submits a profile edit and returns the wrapper event id.
func proposeProfile(t *testing.T, h *harness.H, client *http.Client, v url.Values) string {
	t.Helper()
	resp, err := client.PostForm(h.Server.URL+"/profile/edit", v)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 303 {
		return "" // validation error: the form re-rendered (200)
	}
	// Match by the proposed name (the fake clock is frozen, so timestamps
	// don't disambiguate concurrent wrappers).
	want := `"name":"` + v.Get("name") + `"`
	events := h.QueryRelayAs("xavier", nostr.Filter{Kinds: []int{30078}})
	for _, evt := range events {
		if evt.Tags.GetFirst([]string{"proposal", "profile"}) != nil &&
			strings.Contains(evt.Content, want) {
			return evt.ID
		}
	}
	t.Fatal("profile wrapper not found on the relay")
	return ""
}

func communityProfile(t *testing.T, h *harness.H) map[string]any {
	t.Helper()
	community := communityPub(t, h)
	events := h.QueryRelayAs("xavier", nostr.Filter{Authors: []string{community}, Kinds: []int{0}})
	if len(events) == 0 {
		t.Fatal("community kind 0 missing")
	}
	// newest
	newest := events[0]
	for _, e := range events {
		if e.CreatedAt > newest.CreatedAt {
			newest = e
		}
	}
	var m map[string]any
	json.Unmarshal([]byte(newest.Content), &m)
	return m
}

// TestPROF01_AnyMemberProposesWrapperProtectsOwnProfile pins PROF-01.
func TestPROF01_AnyMemberProposesWrapperProtectsOwnProfile(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan") // plain member, no propose_posts

	id := proposeProfile(t, h, dan, url.Values{
		"name": {"Commons Hub Renamed"}, "about": {"New about."},
		"link_label": {"Website"}, "link_url": {"https://commons.example.org"},
	})
	if id == "" {
		t.Fatal("any member must be able to propose a profile edit")
	}
	c := h.Community()
	danIdent, _ := c.IdentityByUsername("dan")
	events := h.QueryRelayAs("dan", nostr.Filter{IDs: []string{id}})
	if events[0].Kind != 30078 || events[0].PubKey != danIdent.Pubkey {
		t.Fatal("the proposal must be a kind 30078 wrapper signed by dan")
	}
	// Dan's OWN kind 0 must be unchanged (still name=dan).
	danEvents := h.QueryRelayAs("dan", nostr.Filter{Authors: []string{danIdent.Pubkey}, Kinds: []int{0}})
	for _, e := range danEvents {
		if strings.Contains(e.Content, "Commons Hub Renamed") {
			t.Fatal("the wrapper must NOT have replaced dan's own profile")
		}
	}
}

// TestPROF03_QuorumPublishesNewCommunityProfile pins PROF-03 (no
// newsletter).
func TestPROF03_QuorumPublishesNewCommunityProfile(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	confirmFollower(t, h, "marie@example.org")
	mailBefore := h.Mailer.Count("marie@example.org")

	id := proposeProfile(t, h, dan, url.Values{
		"name": {"Open Collective Commons"}, "about": {"Now with shared ownership."},
		"link_label": {"GitHub"}, "link_url": {"https://github.com/oc-commons"},
	})
	r1, _ := alice.PostForm(h.Server.URL+"/profile/pending/"+id+"/approve", nil)
	r1.Body.Close()
	r2, _ := bob.PostForm(h.Server.URL+"/profile/pending/"+id+"/approve", nil)
	r2.Body.Close()

	prof := communityProfile(t, h)
	if prof["name"] != "Open Collective Commons" {
		t.Fatalf("the community kind 0 must carry the approved name, got %v", prof["name"])
	}
	// Links present.
	if !strings.Contains(body(t, h.Get("/")), "https://github.com/oc-commons") {
		t.Fatal("the homepage linktree must show the new link")
	}
	// No newsletter on a profile edit.
	if h.Mailer.Count("marie@example.org") != mailBefore {
		t.Fatal("a profile edit must not trigger the newsletter")
	}
}

// TestPROF04_AdminDecidesAlone pins PROF-04.
func TestPROF04_AdminDecidesAlone(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	id := proposeProfile(t, h, dan, url.Values{"name": {"Admin Approved Name"}, "about": {"x"}})

	r, _ := h.Admin.PostForm(h.Server.URL+"/profile/pending/"+id+"/approve", nil)
	r.Body.Close()
	if communityProfile(t, h)["name"] != "Admin Approved Name" {
		t.Fatal("the admin's lone approval must publish the profile (2-required section)")
	}
}

// TestPROF05_FieldsAndLinksValidatedServerSide pins PROF-05.
func TestPROF05_FieldsAndLinksValidatedServerSide(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")

	resp, _ := dan.PostForm(h.Server.URL+"/profile/edit", url.Values{
		"name": {"Hijack"}, "link_label": {"Evil"}, "link_url": {"javascript:alert(1)"},
	})
	if page := body(t, resp); !strings.Contains(page, "http(s) URL") {
		t.Fatal("a javascript: link must be rejected")
	}
	// Nothing reached the relay.
	for _, evt := range h.QueryRelayAs("dan", nostr.Filter{Kinds: []int{30078}}) {
		if strings.Contains(evt.Content, "javascript:") {
			t.Fatal("an invalid link must never be signed")
		}
	}
}

// TestPROF06_EditingResetsApprovals pins PROF-06.
func TestPROF06_EditingResetsApprovals(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")

	id1 := proposeProfile(t, h, dan, url.Values{"name": {"First Try"}, "about": {"v1"}})
	r, _ := alice.PostForm(h.Server.URL+"/profile/pending/"+id1+"/approve", nil)
	r.Body.Close()

	// Dan revises (a new wrapper, distinct id).
	id2 := proposeProfile(t, h, dan, url.Values{"name": {"Second Try"}, "about": {"v2"}})
	if id2 == id1 {
		t.Fatal("a revision must be a new event id")
	}
	page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending"))
	// The revised edit carries no approvals yet.
	if !strings.Contains(page, "0 of 2") {
		t.Fatal("the revised edit must start with 0 approvals")
	}
}

// TestPROF07_DeclinesWork pins PROF-07.
func TestPROF07_DeclinesWork(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	id := proposeProfile(t, h, dan, url.Values{"name": {"Rejected Name"}, "about": {"no"}})

	r, _ := alice.PostForm(h.Server.URL+"/profile/pending/"+id+"/decline",
		url.Values{"reason": {"Off brand."}})
	r.Body.Close()

	page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending"))
	if strings.Contains(page, "Rejected Name") {
		t.Fatal("a declined profile edit must leave the pending queue")
	}
	if communityProfile(t, h)["name"] == "Rejected Name" {
		t.Fatal("a declined edit must not be published")
	}
}

// TestPROF08_StaleProposalsAreFlagged pins PROF-08.
func TestPROF08_StaleProposalsAreFlagged(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	eve := h.Member("eve")
	alice := h.Member("alice", "steward")

	// Two pending edits based on the same current profile.
	idA := proposeProfile(t, h, dan, url.Values{"name": {"Name A"}, "about": {"a"}})
	idB := proposeProfile(t, h, eve, url.Values{"name": {"Name B"}, "about": {"b"}})
	_ = idB

	// Approve A alone (admin) → publishes → profile changes.
	r, _ := h.Admin.PostForm(h.Server.URL+"/profile/pending/"+idA+"/approve", nil)
	r.Body.Close()
	if communityProfile(t, h)["name"] != "Name A" {
		t.Fatal("edit A must have published")
	}

	// B is now stale (its base differs from the new current).
	page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending"))
	if !strings.Contains(page, "Name B") || !strings.Contains(page, "stale") {
		t.Fatal("the surviving edit must be flagged stale after the profile changed")
	}
}
