//go:build integration

package tests

import (
	"net/url"
	"strings"
	"testing"

	"github.com/opencollective/community/tests/harness"
)

// confirmFollower follows + confirms an email so it is an opted-in
// newsletter recipient.
func confirmFollower(t *testing.T, h *harness.H, email string) {
	t.Helper()
	resp := h.PostForm("/follow", url.Values{"email": {email}})
	resp.Body.Close()
	msg, ok := h.Mailer.LastTo(email)
	if !ok {
		t.Fatalf("no confirmation email for %s", email)
	}
	resp = h.Get(extractLink(t, msg.Text))
	resp.Body.Close()
}

// TestMAIL01_NewsletterEmailedToOptedIn pins MAIL-01 and PUB-13.
func TestMAIL01_NewsletterEmailedToOptedIn(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	confirmFollower(t, h, "marie@example.org")

	// A follower who opts out receives nothing.
	confirmFollower(t, h, "leen@example.org")
	c := h.Community()
	leen, _ := c.IdentityByEmail("leen@example.org")
	c.UpdateProfile(leen.ID, leen.Name, false)

	id := propose(t, h, alice, "newsletter", "June digest", "Everything from June.")
	r1 := decidePost(t, h, bob, id, "approve", nil)
	r1.Body.Close()
	r2 := decidePost(t, h, h.Admin, id, "approve", nil) // admin completes
	r2.Body.Close()

	if h.Mailer.Count("marie@example.org") == 0 {
		t.Fatal("the opted-in follower must receive the newsletter")
	}
	msg, _ := h.Mailer.LastTo("marie@example.org")
	if msg.Subject != "June digest" {
		t.Fatalf("subject must be the title, got %q", msg.Subject)
	}
	// Opted-out follower gets nothing newsletter-shaped.
	if m, ok := h.Mailer.LastTo("leen@example.org"); ok && m.Subject == "June digest" {
		t.Fatal("an opted-out follower must not receive the newsletter")
	}
	// Not in the blog RSS.
	if feed := body(t, h.Get("/feed.xml")); strings.Contains(feed, "June digest") {
		t.Fatal("newsletters must not appear in /feed.xml")
	}
	_ = alice
}

// TestMAIL02_NewsletterHasTextAndHTMLParts pins MAIL-02.
func TestMAIL02_NewsletterHasTextAndHTMLParts(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	confirmFollower(t, h, "marie@example.org")

	md := "# Big news\n\nVisit [our site](https://example.org) and <script>alert(1)</script> stays out."
	id := propose(t, h, alice, "newsletter", "Formatted", md)
	decidePost(t, h, bob, id, "approve", nil).Body.Close()
	decidePost(t, h, h.Admin, id, "approve", nil).Body.Close()

	msg, ok := h.Mailer.LastTo("marie@example.org")
	if !ok {
		t.Fatal("no newsletter delivered")
	}
	if msg.Text == "" || !strings.Contains(msg.Text, "Big news") {
		t.Fatal("the text part must carry readable content")
	}
	if !strings.Contains(msg.HTML, "<h1") || !strings.Contains(msg.HTML, "<a href=") {
		t.Fatal("the HTML part must render markdown")
	}
	if strings.Contains(msg.HTML, "<script") {
		t.Fatal("scripts must be sanitized out of the HTML part")
	}
	if !strings.Contains(msg.Text, "/posts/") || !strings.Contains(msg.HTML, "Unsubscribe") {
		t.Fatal("both parts must carry the read-online and unsubscribe links")
	}
	if msg.ListUnsubscribe == "" {
		t.Fatal("List-Unsubscribe header must be set")
	}
}

// TestMAIL05_SendingIsAtMostOnce pins MAIL-05.
func TestMAIL05_SendingIsAtMostOnce(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	confirmFollower(t, h, "marie@example.org")

	id := propose(t, h, alice, "newsletter", "Once only", "Send me once.")
	decidePost(t, h, bob, id, "approve", nil).Body.Close()
	decidePost(t, h, h.Admin, id, "approve", nil).Body.Close()
	first := h.Mailer.Count("marie@example.org")

	// A redundant approval (or re-trigger) must not re-send.
	carol := h.Member("carol", "steward")
	decidePost(t, h, carol, id, "approve", nil).Body.Close()
	if h.Mailer.Count("marie@example.org") != first {
		t.Fatal("the newsletter must be sent at most once")
	}
}

// TestMAIL08_TransactionalEmailsFlowRegardlessOfOptIn pins MAIL-08.
func TestMAIL08_TransactionalEmailsFlowRegardlessOfOptIn(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	dan := h.Member("dan")
	c := h.Community()
	ident, _ := c.IdentityByUsername("dan")
	c.UpdateProfile(ident.ID, ident.Name, false) // opt out of newsletter

	resp := h.PostForm("/login", url.Values{"email": {"dan@example.org"}})
	resp.Body.Close()
	if h.Mailer.LastCodeTo("dan@example.org") == "" {
		t.Fatal("login codes must flow even to opted-out members")
	}
	_ = dan
}
