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

// propose submits a post via /compose and returns its proposal event id.
func propose(t *testing.T, h *harness.H, client *http.Client, ctype, title, content string) string {
	t.Helper()
	resp, err := client.PostForm(h.Server.URL+"/compose", url.Values{
		"content_type": {ctype}, "title": {title}, "content": {content},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("compose: want 303, got %d", resp.StatusCode)
	}
	kind := 1
	if ctype != "announcement" {
		kind = 30023
	}
	events := h.QueryRelayAs("xavier", nostr.Filter{Kinds: []int{kind}})
	for _, evt := range events {
		if evt.Content == content && evt.Tags.GetFirst([]string{"proposal", ""}) != nil {
			return evt.ID
		}
	}
	t.Fatal("proposal not found on the relay")
	return ""
}

func decidePost(t *testing.T, h *harness.H, client *http.Client, id, action string, v url.Values) *http.Response {
	t.Helper()
	resp, err := client.PostForm(h.Server.URL+"/posts/pending/"+id+"/"+action, v)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func communityPub(t *testing.T, h *harness.H) string {
	t.Helper()
	c := h.Community()
	community, err := c.IdentityByUsername("community")
	if err != nil {
		t.Fatal(err)
	}
	return community.Pubkey
}

// TestPUB01_ComposingRequiresProposePermission pins PUB-01.
func TestPUB01_ComposingRequiresProposePermission(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")        // no propose_posts
	alice := h.Member("alice", "steward")

	resp, _ := dan.Get(h.Server.URL + "/compose")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("plain member /compose must 404, got %d", resp.StatusCode)
	}
	resp, _ = dan.PostForm(h.Server.URL+"/compose", url.Values{
		"content_type": {"announcement"}, "content": {"sneaky"},
	})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("forged compose must be rejected")
	}
	resp, _ = alice.Get(h.Server.URL + "/compose")
	if page := body(t, resp); !strings.Contains(page, "Announcement") || !strings.Contains(page, "Blog post") {
		t.Fatal("a steward must see the compose form")
	}
}

// TestPUB02_ProposedAnnouncementIsASignedEvent pins PUB-02.
func TestPUB02_ProposedAnnouncementIsASignedEvent(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	id := propose(t, h, alice, "announcement", "", "Server is live for testing.")

	c := h.Community()
	aliceIdent, _ := c.IdentityByUsername("alice")
	events := h.QueryRelayAs("alice", nostr.Filter{IDs: []string{id}})
	if len(events) != 1 {
		t.Fatal("proposal missing")
	}
	evt := events[0]
	if evt.Kind != 1 || evt.PubKey != aliceIdent.Pubkey {
		t.Fatal("proposal must be a kind 1 signed by alice")
	}
	if evt.Tags.GetFirst([]string{"a", ""}) == nil {
		t.Fatal("proposal must carry the community a-tag")
	}
	// Appears in pending, not on the public homepage.
	if page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending")); !strings.Contains(page, "Server is live") {
		t.Fatal("the proposal must list in pending posts")
	}
	if page := body(t, h.Get("/")); strings.Contains(page, "Server is live") {
		t.Fatal("an unapproved proposal must not appear on the homepage")
	}
}

// TestPUB03_ApprovalIsASignedKind4550 pins PUB-03 (newsletter, 2 required,
// so one approval shows "1 of 2").
func TestPUB03_ApprovalIsASignedKind4550(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	id := propose(t, h, alice, "newsletter", "June update", "Lots happened in June.")

	resp := decidePost(t, h, bob, id, "approve", nil)
	resp.Body.Close()

	bobIdent, _ := h.Community().IdentityByPubkey(communityPub(t, h))
	_ = bobIdent
	events := h.QueryRelayAs("alice", nostr.Filter{Kinds: []int{4550}})
	var approval *nostr.Event
	for _, evt := range events {
		if firstEvtTag(evt) == id {
			approval = evt
		}
	}
	if approval == nil {
		t.Fatal("a kind 4550 approval referencing the proposal must exist")
	}
	bi, _ := h.Community().IdentityByUsername("bob")
	if approval.PubKey != bi.Pubkey {
		t.Fatal("the approval must be signed by bob")
	}
	if !strings.Contains(approval.Content, "Lots happened in June.") {
		t.Fatal("the approval must embed the proposal content")
	}
	if page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending")); !strings.Contains(page, "1 of 2") {
		t.Fatal("pending must show 1 of 2 approvals")
	}
}

// TestPUB04_QuorumPublishesTheAnnouncement pins PUB-04.
func TestPUB04_QuorumPublishesTheAnnouncement(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	id := propose(t, h, alice, "announcement", "", "We launched today.")

	resp := decidePost(t, h, bob, id, "approve", nil) // announcement needs 1
	resp.Body.Close()

	aliceIdent, _ := h.Community().IdentityByUsername("alice")
	community := communityPub(t, h)
	events := h.QueryRelayAs("alice", nostr.Filter{Authors: []string{community}, Kinds: []int{1}})
	var pub *nostr.Event
	for _, evt := range events {
		if evt.Content == "We launched today." {
			pub = evt
		}
	}
	if pub == nil {
		t.Fatal("the community must publish its own kind 1")
	}
	if tag := pub.Tags.GetFirst([]string{"p", aliceIdent.Pubkey}); tag == nil {
		t.Fatal("the published event must credit the author")
	}
	if mention := firstMention(pub); mention != id {
		t.Fatal("the published event must reference the proposal")
	}
	if page := body(t, h.Get("/")); !strings.Contains(page, "We launched today.") {
		t.Fatal("the announcement must appear on the public homepage")
	}
	if page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending")); strings.Contains(page, "We launched today.") {
		t.Fatal("a published proposal must leave the pending queue")
	}
}

// TestPUB05_AdminPublishesAlone pins PUB-05.
func TestPUB05_AdminPublishesAlone(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	id := propose(t, h, alice, "newsletter", "Big news", "Even newsletters: admin alone.")

	resp := decidePost(t, h, h.Admin, id, "approve", nil)
	resp.Body.Close()

	community := communityPub(t, h)
	events := h.QueryRelayAs("alice", nostr.Filter{Authors: []string{community}, Kinds: []int{30023}})
	for _, evt := range events {
		if evt.Content == "Even newsletters: admin alone." {
			return
		}
	}
	t.Fatal("the admin's lone approval must publish even a 2-required newsletter")
}

// TestPUB06_ProposersOwnApprovalNeverCounts pins PUB-06.
func TestPUB06_ProposersOwnApprovalNeverCounts(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	id := propose(t, h, alice, "announcement", "", "Self approved?")

	resp := decidePost(t, h, alice, id, "approve", nil)
	resp.Body.Close()

	community := communityPub(t, h)
	events := h.QueryRelayAs("alice", nostr.Filter{Authors: []string{community}, Kinds: []int{1}})
	for _, evt := range events {
		if evt.Content == "Self approved?" {
			t.Fatal("the author's own approval must not publish")
		}
	}
	if page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending")); !strings.Contains(page, "Self approved?") {
		t.Fatal("the proposal must remain pending")
	}
}

// TestPUB09_BlogPostReachesBlogAndRSSNotInbox pins PUB-09.
func TestPUB09_BlogPostReachesBlogAndRSSNotInbox(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	before := len(h.Mailer.Messages)
	id := propose(t, h, alice, "blog", "Our first post", "# Hello\n\nThis is our blog.")

	resp := decidePost(t, h, bob, id, "approve", nil) // blog needs 1
	resp.Body.Close()

	page := body(t, h.Get("/"))
	if !strings.Contains(page, "Our first post") {
		t.Fatal("the blog post must appear on the homepage")
	}
	// /posts/{slug} renders the article.
	slug := extractSlug(t, page)
	if page := body(t, h.Get("/posts/"+slug)); !strings.Contains(page, "Hello") {
		t.Fatal("the blog post must render at its slug")
	}
	// RSS lists it.
	feed := body(t, h.Get("/feed.xml"))
	if !strings.Contains(feed, "Our first post") {
		t.Fatal("the blog post must appear in /feed.xml")
	}
	if len(h.Mailer.Messages) != before {
		t.Fatal("a blog post must not be emailed (MAIL-04)")
	}
}

// TestPUB11_PerSectionPolicyIsConfigurable pins PUB-11.
func TestPUB11_PerSectionPolicyIsConfigurable(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")

	// Raise announcements to 2.
	resp, _ := h.Admin.PostForm(h.Server.URL+"/settings/posts/announcement",
		url.Values{"approve_roles": {"steward"}, "approvals_required": {"2"}})
	resp.Body.Close()

	id := propose(t, h, alice, "announcement", "", "Now needs two.")
	r := decidePost(t, h, bob, id, "approve", nil)
	r.Body.Close()
	community := communityPub(t, h)
	for _, evt := range h.QueryRelayAs("alice", nostr.Filter{Authors: []string{community}, Kinds: []int{1}}) {
		if evt.Content == "Now needs two." {
			t.Fatal("one approval must not publish when 2 are required")
		}
	}
	if page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending")); !strings.Contains(page, "1 of 2") {
		t.Fatal("announcement must now show 1 of 2")
	}
}

func firstEvtTag(evt *nostr.Event) string  { return firstETagRaw(evt) }
func firstMention(evt *nostr.Event) string { return firstMentionRaw(evt) }

func firstETagRaw(evt *nostr.Event) string {
	if tag := evt.Tags.GetFirst([]string{"e", ""}); tag != nil && len(*tag) > 1 {
		return (*tag)[1]
	}
	return ""
}

func firstMentionRaw(evt *nostr.Event) string {
	for _, tag := range evt.Tags.GetAll([]string{"e", ""}) {
		if len(tag) >= 4 && tag[3] == "mention" {
			return tag[1]
		}
	}
	return ""
}

func extractSlug(t *testing.T, homepage string) string {
	t.Helper()
	i := strings.Index(homepage, "/posts/")
	if i < 0 {
		t.Fatal("no post link on the homepage")
	}
	rest := homepage[i+len("/posts/"):]
	end := strings.IndexAny(rest, `"<`)
	return rest[:end]
}
