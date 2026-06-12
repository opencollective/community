//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/opencollective/community/tests/harness"
)

// apply submits a join application and verifies the email, returning the
// applicant's (not yet member) client.
func apply(t *testing.T, h *harness.H, username string) {
	t.Helper()
	email := username + "@example.org"
	resp := h.PostForm("/join", url.Values{
		"name": {username}, "username": {username}, "email": {email},
		"motivation": {"I build commons."},
	})
	resp.Body.Close()
	resp = h.PostForm("/join/verify", url.Values{
		"email": {email}, "code": {h.Mailer.LastCodeTo(email)},
	})
	resp.Body.Close()
}

// TestJOIN01_ApplicationRequiresEmailVerification pins JOIN-01's local
// half: identity created, application pending only after the code,
// acknowledgment sent.
func TestJOIN01_ApplicationRequiresEmailVerification(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	c := h.Community()

	resp := h.PostForm("/join", url.Values{
		"name": {"Marie Curie"}, "username": {"marie"}, "email": {"marie@example.org"},
		"motivation": {"I run the open hardware collective in Ghent."},
	})
	resp.Body.Close()

	app, err := c.OpenApplicationByEmail("marie@example.org")
	if err != nil || app.Status != "awaiting_email" {
		t.Fatalf("application must await email verification: %+v %v", app, err)
	}
	if apps, _ := c.PendingApplications(); len(apps) != 0 {
		t.Fatal("unverified applications must not reach the review queue")
	}

	resp = h.PostForm("/join/verify", url.Values{
		"email": {"marie@example.org"}, "code": {h.Mailer.LastCodeTo("marie@example.org")},
	})
	resp.Body.Close()
	app, _ = c.ApplicationByID(app.ID)
	if app.Status != "pending" {
		t.Fatalf("verified application must be pending, got %s", app.Status)
	}
	if msg, ok := h.Mailer.LastTo("marie@example.org"); !ok ||
		!strings.Contains(msg.Subject, "Application received") {
		t.Fatal("the applicant must get an acknowledgment email")
	}
}

// TestJOIN02_UsernameAvailabilityProbe pins JOIN-02.
func TestJOIN02_UsernameAvailabilityProbe(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	h.Member("alice")

	resp := h.Get("/join/check?username=alice")
	if got := body(t, resp); !strings.Contains(got, "taken") || !strings.Contains(got, "alice1") {
		t.Fatalf("taken username must suggest a variant, got %q", got)
	}
	resp = h.Get("/join/check?username=brandnew")
	if got := body(t, resp); got != "available" {
		t.Fatalf("want available, got %q", got)
	}
	resp = h.Get("/join/check?username=admin")
	if got := body(t, resp); !strings.Contains(got, "reserved") {
		t.Fatalf("reserved username must say so, got %q", got)
	}
}

// TestJOIN03_PendingApplicationsAreMembersOnly pins JOIN-03.
func TestJOIN03_PendingApplicationsAreMembersOnly(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	apply(t, h, "marie")

	// Anonymous → login.
	resp := h.Get("/members/pending")
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Fatalf("anonymous must be sent to login, got %d %q", resp.StatusCode, loc)
	}
	// The application text appears on no public page.
	for _, path := range []string{"/", "/join", "/follow"} {
		resp := h.Get(path)
		if strings.Contains(body(t, resp), "open hardware") {
			t.Fatalf("application content leaked on %s", path)
		}
	}
}

// TestJOIN04_FirstApprovalDecidesNothing pins JOIN-04.
func TestJOIN04_FirstApprovalDecidesNothing(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice", "steward")
	apply(t, h, "marie")
	c := h.Community()
	app, _ := c.OpenApplicationByEmail("marie@example.org")

	resp, _ := alice.PostForm(h.Server.URL+fmt.Sprintf("/members/pending/%d", app.ID),
		url.Values{"decision": {"approve"}})
	resp.Body.Close()

	app, _ = c.ApplicationByID(app.ID)
	if app.Status != "pending" {
		t.Fatalf("one steward approval must not decide, got %s", app.Status)
	}
	// The same steward cannot count twice.
	resp, _ = alice.PostForm(h.Server.URL+fmt.Sprintf("/members/pending/%d", app.ID),
		url.Values{"decision": {"approve"}})
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "already") {
		t.Fatalf("double approval must be flagged, got %q", loc)
	}
}

// TestJOIN05_TwoStewardsActivateMembership pins JOIN-05's local half.
func TestJOIN05_TwoStewardsActivateMembership(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	apply(t, h, "marie")
	c := h.Community()
	app, _ := c.OpenApplicationByEmail("marie@example.org")

	for _, cl := range []*http.Client{alice, bob} {
		resp, _ := cl.PostForm(h.Server.URL+fmt.Sprintf("/members/pending/%d", app.ID),
			url.Values{"decision": {"approve"}})
		resp.Body.Close()
	}

	ident, _ := c.IdentityByUsername("marie")
	if ident.Status != "active" {
		t.Fatalf("two steward approvals must activate, got %s", ident.Status)
	}
	roles, _ := c.RoleNames(ident.ID)
	if !contains(roles, "member") {
		t.Fatalf("approved member must hold the member role, got %v", roles)
	}
	if msg, ok := h.Mailer.LastTo("marie@example.org"); !ok ||
		!strings.Contains(msg.Text, "/login") {
		t.Fatal("the decision email must carry a login link")
	}
}

// TestJOIN06_AdminDecidesAlone pins JOIN-06 (already exercised by the
// harness Member fixture; asserted explicitly here).
func TestJOIN06_AdminDecidesAlone(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	apply(t, h, "marie")
	c := h.Community()
	app, _ := c.OpenApplicationByEmail("marie@example.org")

	resp, _ := h.Admin.PostForm(h.Server.URL+fmt.Sprintf("/members/pending/%d", app.ID),
		url.Values{"decision": {"approve"}})
	resp.Body.Close()
	ident, _ := c.IdentityByUsername("marie")
	if ident.Status != "active" {
		t.Fatal("the admin's approval must decide alone")
	}
}

// TestJOIN07_PlainMembersCannotDecide pins JOIN-07.
func TestJOIN07_PlainMembersCannotDecide(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	dan := h.Member("dan") // no approve_members
	apply(t, h, "marie")
	c := h.Community()
	app, _ := c.OpenApplicationByEmail("marie@example.org")

	// He can see the queue…
	resp, _ := dan.Get(h.Server.URL + "/members/pending")
	if got := body(t, resp); !strings.Contains(got, "marie") {
		t.Fatal("members must see pending applications")
	}
	// …but a forged decision is rejected.
	resp, _ = dan.PostForm(h.Server.URL+fmt.Sprintf("/members/pending/%d", app.ID),
		url.Values{"decision": {"approve"}})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("forged decision must 404, got %d", resp.StatusCode)
	}
	if app, _ := c.ApplicationByID(app.ID); app.Status != "pending" {
		t.Fatal("nothing may have changed")
	}
}

// TestJOIN08_DeclineWithReasonAndReapplyWindow pins JOIN-08.
func TestJOIN08_DeclineWithReasonAndReapplyWindow(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	apply(t, h, "marie")
	c := h.Community()
	app, _ := c.OpenApplicationByEmail("marie@example.org")

	resp, _ := h.Admin.PostForm(h.Server.URL+fmt.Sprintf("/members/pending/%d", app.ID),
		url.Values{"decision": {"decline"}, "reason": {"Introduce yourself in person first."}})
	resp.Body.Close()

	if msg, ok := h.Mailer.LastTo("marie@example.org"); !ok ||
		!strings.Contains(msg.Text, "Introduce yourself in person first.") {
		t.Fatal("the decline email must carry the reason")
	}

	// Reapplying inside 30 days is blocked…
	resp = h.PostForm("/join", url.Values{
		"name": {"Marie"}, "username": {"marie"}, "email": {"marie@example.org"},
		"motivation": {"Round two."},
	})
	if !strings.Contains(body(t, resp), "reapply after 30 days") {
		t.Fatal("reapplying inside the window must be blocked")
	}
	// …and possible after.
	h.Clock.Advance(31 * 24 * time.Hour)
	resp = h.PostForm("/join", url.Values{
		"name": {"Marie"}, "username": {"marie"}, "email": {"marie@example.org"},
		"motivation": {"Round two."},
	})
	if got := body(t, resp); !strings.Contains(got, "sent a 6-digit code") && !strings.Contains(got, "Enter the code") {
		t.Fatalf("reapplying after the window must proceed to verification")
	}
}

// TestJOIN09_OneOpenApplicationPerEmail pins JOIN-09.
func TestJOIN09_OneOpenApplicationPerEmail(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	apply(t, h, "marie")

	resp := h.PostForm("/join", url.Values{
		"name": {"Imposter"}, "username": {"marie2"}, "email": {"marie@example.org"},
		"motivation": {"Again."},
	})
	if !strings.Contains(body(t, resp), "already an application") {
		t.Fatal("a second open application for the same email must be refused")
	}
	var n int
	h.Community().DB.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&n)
	if n != 1 {
		t.Fatalf("want 1 application, got %d", n)
	}
}

// TestFOLLOW07_FollowerUpgradesKeepTheirIdentity pins FOLLOW-07.
func TestFOLLOW07_FollowerUpgradesKeepTheirIdentity(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	c := h.Community()

	resp := h.PostForm("/follow", url.Values{"email": {"marie@example.org"}})
	resp.Body.Close()
	msg, _ := h.Mailer.LastTo("marie@example.org")
	resp = h.Get(extractLink(t, msg.Text))
	resp.Body.Close()
	follower, _ := c.IdentityByEmail("marie@example.org")

	apply(t, h, "marie")
	app, _ := c.OpenApplicationByEmail("marie@example.org")
	resp2, _ := h.Admin.PostForm(h.Server.URL+fmt.Sprintf("/members/pending/%d", app.ID),
		url.Values{"decision": {"approve"}})
	resp2.Body.Close()

	member, _ := c.IdentityByEmail("marie@example.org")
	if member.ID != follower.ID || member.Pubkey != follower.Pubkey {
		t.Fatal("the upgrade must reuse the same identity and key")
	}
	roles, _ := c.RoleNames(member.ID)
	if !contains(roles, "member") || !contains(roles, "follower") {
		t.Fatalf("upgraded identity keeps follower and gains member, got %v", roles)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
