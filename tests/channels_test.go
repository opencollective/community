//go:build integration

package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/tests/harness"
)

// createThread posts a thread and returns its event id from the relay.
func createThread(t *testing.T, h *harness.H, client *http.Client, slug, title, content string, extra url.Values) string {
	t.Helper()
	v := url.Values{"title": {title}, "content": {content}}
	for k, vals := range extra {
		v[k] = vals
	}
	resp, err := client.PostForm(h.Server.URL+"/channels/"+slug, v)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("thread create: want 303, got %d", resp.StatusCode)
	}
	events := h.QueryRelayAs("xavier", nostr.Filter{
		Kinds: []int{11}, Tags: nostr.TagMap{"h": []string{slug}},
	})
	for _, evt := range events {
		if evt.Content == content {
			return evt.ID
		}
	}
	t.Fatal("created thread not found on the relay")
	return ""
}

func act(t *testing.T, h *harness.H, client *http.Client, slug, id, action string, v url.Values) *http.Response {
	t.Helper()
	resp, err := client.PostForm(h.Server.URL+"/channels/"+slug+"/t/"+id+"/"+action, v)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestCHAN01_DefaultsAfterSetup pins CHAN-01.
func TestCHAN01_DefaultsAfterSetup(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	c := h.Community()

	for slug, want := range map[string]bool{"general": true, "proposals": true, "requests": false} {
		ch, err := c.ChannelBySlug(slug)
		if err != nil || ch.Enabled != want {
			t.Fatalf("channel %s: enabled=%v want %v (%v)", slug, ch.Enabled, want, err)
		}
	}
	resp, _ := h.Admin.Get(h.Server.URL + "/settings/community")
	page := body(t, resp)
	for _, want := range []string{"Proposals", "Requests", "always on"} {
		if !strings.Contains(page, want) {
			t.Fatalf("settings must show %q", want)
		}
	}
}

// TestCHAN02_TogglingOffAndOn pins CHAN-02.
func TestCHAN02_TogglingOffAndOn(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice")
	id := createThread(t, h, alice, "proposals", "Solar panels", "Put panels on the roof.", nil)

	post := func(v url.Values) {
		resp, _ := h.Admin.PostForm(h.Server.URL+"/settings/channels/proposals", v)
		resp.Body.Close()
	}
	post(url.Values{}) // no enabled=1 → disabled

	resp, _ := alice.Get(h.Server.URL + "/channels/proposals")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("disabled channel must 404, got %d", resp.StatusCode)
	}
	resp, _ = alice.PostForm(h.Server.URL+"/channels/proposals",
		url.Values{"title": {"x"}, "content": {"y"}})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("writes to a disabled channel must be rejected")
	}

	post(url.Values{"enabled": {"1"}})
	resp, _ = alice.Get(h.Server.URL + "/channels/proposals/t/" + id)
	if page := body(t, resp); !strings.Contains(page, "Solar panels") {
		t.Fatal("re-enabling must restore prior threads intact")
	}
}

// TestCHAN03_ProposalsAreMembersOnly pins CHAN-03.
func TestCHAN03_ProposalsAreMembersOnly(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice")
	id := createThread(t, h, alice, "proposals", "Secret plan", "The members-only plan.", nil)

	for _, path := range []string{"/channels/proposals", "/channels/proposals/t/" + id} {
		resp := h.Get(path)
		page := body(t, resp)
		if strings.Contains(page, "Secret plan") {
			t.Fatalf("visitor must not see proposal content on %s", path)
		}
	}
	// And the relay leaks nothing to outsiders.
	ctx, cancel := contextWithTimeout(t)
	defer cancel()
	relay, err := nostr.RelayConnect(ctx, "ws"+strings.TrimPrefix(h.Server.URL, "http")+"/relay")
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	events, _ := relay.QuerySync(ctx, nostr.Filter{Kinds: []int{11}})
	if len(events) != 0 {
		t.Fatalf("outsiders must receive no thread events, got %d", len(events))
	}
}

// TestCHAN04_MemberStartsAThread pins CHAN-04 (and CHAN-17's labels).
func TestCHAN04_MemberStartsAThread(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	id := createThread(t, h, dan, "proposals", "Bike racks", "Racks by the entrance.", nil)

	c := h.Community()
	ident, _ := c.IdentityByUsername("dan")
	events := h.QueryRelayAs("dan", nostr.Filter{IDs: []string{id}})
	if len(events) != 1 || events[0].PubKey != ident.Pubkey {
		t.Fatal("the root must be signed by dan's key")
	}
	resp, _ := dan.Get(h.Server.URL + "/channels/proposals")
	page := body(t, resp)
	if !strings.Contains(page, "Bike racks") || !strings.Contains(page, "pending") {
		t.Fatal("the new thread must list as pending")
	}
}

// TestCHAN05_RepliesThreadUnderTheRoot pins CHAN-05.
func TestCHAN05_RepliesThreadUnderTheRoot(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice")
	id := createThread(t, h, dan, "proposals", "Compost", "A compost corner.", nil)

	resp := act(t, h, alice, "proposals", id, "reply", url.Values{"content": {"Strong yes from me."}})
	resp.Body.Close()

	resp2, _ := alice.Get(h.Server.URL + "/channels/proposals/t/" + id)
	page := body(t, resp2)
	if !strings.Contains(page, "Strong yes from me.") || !strings.Contains(page, "@alice") {
		t.Fatal("the reply must render under the root")
	}
	resp3, _ := alice.Get(h.Server.URL + "/channels/proposals")
	if page := body(t, resp3); !strings.Contains(page, "1 replies") {
		t.Fatal("the list must count replies")
	}
}

// TestCHAN06_ReactionsToggleAndGroup pins CHAN-06.
func TestCHAN06_ReactionsToggleAndGroup(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice")
	id := createThread(t, h, dan, "proposals", "Mural", "Paint the wall.", nil)

	react := func(cl *http.Client) {
		resp := act(t, h, cl, "proposals", id, "react", url.Values{"emoji": {"👍"}})
		resp.Body.Close()
	}
	react(alice)
	react(dan)
	resp, _ := alice.Get(h.Server.URL + "/channels/proposals/t/" + id)
	if page := body(t, resp); !strings.Contains(page, "👍 2") {
		t.Fatal("two members' identical reactions must group to 2")
	}
	react(alice) // toggle off
	resp2, _ := alice.Get(h.Server.URL + "/channels/proposals/t/" + id)
	if page := body(t, resp2); !strings.Contains(page, "👍 1") {
		t.Fatal("re-reacting must remove the reaction (count 1)")
	}
}

// TestCHAN13_TemplateValidationIsServerSide pins CHAN-13.
func TestCHAN13_TemplateValidationIsServerSide(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")

	resp, _ := dan.PostForm(h.Server.URL+"/channels/proposals",
		url.Values{"title": {""}, "content": {"No title given."}})
	if page := body(t, resp); !strings.Contains(page, "title is required") {
		t.Fatal("a proposal without a title must be rejected with the violation")
	}
	events := h.QueryRelayAs("dan", nostr.Filter{
		Kinds: []int{11}, Tags: nostr.TagMap{"h": []string{"proposals"}},
	})
	if len(events) != 0 {
		t.Fatal("nothing may reach the relay on a template violation")
	}
}

// TestCHAN15_DefaultPolicyOneStewardNotTheAuthor pins CHAN-15.
func TestCHAN15_DefaultPolicyOneStewardNotTheAuthor(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	id := createThread(t, h, alice, "proposals", "Library", "A lending library.", nil)

	// The author's own approval never counts.
	resp := act(t, h, alice, "proposals", id, "approve", nil)
	resp.Body.Close()
	resp2, _ := alice.Get(h.Server.URL + "/channels/proposals/t/" + id)
	if page := body(t, resp2); !strings.Contains(page, "pending") {
		t.Fatal("the author's approval must not count")
	}

	// One other steward approves.
	resp3 := act(t, h, bob, "proposals", id, "approve", nil)
	resp3.Body.Close()
	resp4, _ := alice.Get(h.Server.URL + "/channels/proposals/t/" + id)
	if page := body(t, resp4); !strings.Contains(page, "approved") {
		t.Fatal("one steward (not the author) must approve under the default policy")
	}

	// The admin alone also approves (fresh thread).
	id2 := createThread(t, h, alice, "proposals", "Garden", "A herb garden.", nil)
	resp5 := act(t, h, h.Admin, "proposals", id2, "approve", nil)
	resp5.Body.Close()
	resp6, _ := alice.Get(h.Server.URL + "/channels/proposals/t/" + id2)
	if page := body(t, resp6); !strings.Contains(page, "approved") {
		t.Fatal("the admin alone must approve")
	}
}

// TestCHAN16_PolicyIsConfigurablePerChannel pins CHAN-16.
func TestCHAN16_PolicyIsConfigurablePerChannel(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	carol := h.Member("carol", "moderator")
	dan := h.Member("dan")

	resp, _ := h.Admin.PostForm(h.Server.URL+"/settings/channels/requests", url.Values{
		"enabled": {"1"}, "approve_roles": {"steward,moderator"},
		"approvals_required": {"2"}, "default_visibility": {"public"}, "overridable": {"1"},
	})
	resp.Body.Close()

	id := createThread(t, h, dan, "requests", "", "Looking for a projector.", nil)

	r1 := act(t, h, carol, "requests", id, "approve", nil)
	r1.Body.Close()
	page := body(t, mustGet(t, dan, h.Server.URL+"/channels/requests/t/"+id))
	if !strings.Contains(page, "1 of 2") {
		t.Fatalf("one moderator approval must leave 1 of 2:\n%s", page)
	}

	// A plain member's approval attempt is rejected.
	r2 := act(t, h, dan, "requests", id, "approve", nil)
	r2.Body.Close()
	if r2.StatusCode != 404 {
		t.Fatalf("plain member approval must 404, got %d", r2.StatusCode)
	}

	r3 := act(t, h, alice, "requests", id, "approve", nil)
	r3.Body.Close()
	page = body(t, mustGet(t, dan, h.Server.URL+"/channels/requests/t/"+id))
	if !strings.Contains(page, "approved") {
		t.Fatal("a second distinct approver must complete the quorum")
	}

	// The Proposals policy is untouched.
	c := h.Community()
	ch, _ := c.ChannelBySlug("proposals")
	if ch.ApprovalsRequired != 1 || len(ch.ApproveRoles) != 1 || ch.ApproveRoles[0] != "steward" {
		t.Fatal("other channels' policies must be unaffected")
	}
}

// TestCHAN17_StatusFilter pins CHAN-17.
func TestCHAN17_StatusFilter(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	dan := h.Member("dan")
	idPending := createThread(t, h, dan, "proposals", "Pending one", "Still waiting.", nil)
	idApproved := createThread(t, h, dan, "proposals", "Approved one", "Got the nod.", nil)
	resp := act(t, h, alice, "proposals", idApproved, "approve", nil)
	resp.Body.Close()
	_ = idPending

	page := body(t, mustGet(t, dan, h.Server.URL+"/channels/proposals?status=pending"))
	if !strings.Contains(page, "Pending one") || strings.Contains(page, "Approved one") {
		t.Fatal("the pending filter must show only pending threads")
	}
	page = body(t, mustGet(t, dan, h.Server.URL+"/channels/proposals?status=approved"))
	if strings.Contains(page, "Pending one") || !strings.Contains(page, "Approved one") {
		t.Fatal("the approved filter must show only approved threads")
	}
}

// TestCHAN18_DecliningAThread pins CHAN-18 (member-author variant; the
// external email notification arrives with the requests milestone).
func TestCHAN18_DecliningAThread(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	dan := h.Member("dan")
	id := createThread(t, h, dan, "proposals", "Hot tub", "A hot tub in the office.", nil)

	resp := act(t, h, alice, "proposals", id, "decline", url.Values{"reason": {"Budget says no."}})
	resp.Body.Close()

	page := body(t, mustGet(t, dan, h.Server.URL+"/channels/proposals"))
	if strings.Contains(page, "Hot tub") {
		t.Fatal("declined threads must leave the default list")
	}
	page = body(t, mustGet(t, dan, h.Server.URL+"/channels/proposals/t/"+id))
	if !strings.Contains(page, "declined by @alice") || !strings.Contains(page, "Budget says no.") {
		t.Fatal("the decline and its reason must render")
	}
}

// TestCHAN19_VisibilityDefaultsAndOverrideLock pins CHAN-19.
func TestCHAN19_VisibilityDefaultsAndOverrideLock(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")
	resp, _ := h.Admin.PostForm(h.Server.URL+"/settings/channels/requests", url.Values{
		"enabled": {"1"}, "approvals_required": {"1"},
		"default_visibility": {"public"}, "overridable": {"1"},
	})
	resp.Body.Close()

	// Overridable channel honors a members-only choice.
	id := createThread(t, h, dan, "requests", "", "A quiet request.",
		url.Values{"visibility": {"members"}})
	events := h.QueryRelayAs("dan", nostr.Filter{IDs: []string{id}})
	if tag := events[0].Tags.GetFirst([]string{"visibility", ""}); tag == nil || (*tag)[1] != "members" {
		t.Fatal("the chosen visibility must be on the signed root")
	}

	// The locked Proposals channel rejects a forged override.
	resp2, _ := dan.PostForm(h.Server.URL+"/channels/proposals", url.Values{
		"title": {"Leak"}, "content": {"Make it public."}, "visibility": {"public"},
	})
	if page := body(t, resp2); !strings.Contains(page, "visibility is fixed") {
		t.Fatal("a forged visibility on a locked channel must be rejected")
	}
}

// TestCHAN20_VisibilityGatesRenderingAfterApproval pins CHAN-20.
func TestCHAN20_VisibilityGatesRenderingAfterApproval(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	dan := h.Member("dan")
	resp, _ := h.Admin.PostForm(h.Server.URL+"/settings/channels/requests", url.Values{
		"enabled": {"1"}, "approvals_required": {"1"},
		"default_visibility": {"public"}, "overridable": {"1"},
	})
	resp.Body.Close()

	idPublic := createThread(t, h, dan, "requests", "", "Public ask.", nil)
	idMembers := createThread(t, h, dan, "requests", "", "Private ask.",
		url.Values{"visibility": {"members"}})

	// Pending threads are invisible to visitors regardless of visibility.
	page := body(t, h.Get("/channels/requests"))
	if strings.Contains(page, "Public ask.") {
		t.Fatal("pending threads must be invisible to visitors")
	}

	for _, id := range []string{idPublic, idMembers} {
		r := act(t, h, alice, "requests", id, "approve", nil)
		r.Body.Close()
	}

	page = body(t, h.Get("/channels/requests"))
	if !strings.Contains(page, "Public ask.") || strings.Contains(page, "Private ask.") {
		t.Fatal("visitors must see approved public threads only")
	}
	resp2 := h.Get("/channels/requests/t/" + idMembers)
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatal("a members-only thread's direct URL must 404 for visitors")
	}
	page = body(t, mustGet(t, dan, h.Server.URL+"/channels/requests"))
	if !strings.Contains(page, "Private ask.") {
		t.Fatal("members must see both")
	}
}

func mustGet(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}


