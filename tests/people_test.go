//go:build integration

package tests

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/opencollective/community/tests/harness"
)

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- login ---

// TestLOGIN01_EmailCodeLogsAMemberIn pins LOGIN-01 including single-use.
func TestLOGIN01_EmailCodeLogsAMemberIn(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	h.Member("alice")

	client := h.Client()
	resp, _ := client.PostForm(h.Server.URL+"/login", url.Values{"email": {"alice@example.org"}})
	resp.Body.Close()
	code := h.Mailer.LastCodeTo("alice@example.org")

	resp, _ = client.PostForm(h.Server.URL+"/login",
		url.Values{"email": {"alice@example.org"}, "code": {code}})
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("login: want 303, got %d", resp.StatusCode)
	}
	resp, _ = client.Get(h.Server.URL + "/members")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("member page after login: want 200, got %d", resp.StatusCode)
	}

	// Single use: the same code fails for a second client.
	other := h.Client()
	resp, _ = other.PostForm(h.Server.URL+"/login",
		url.Values{"email": {"alice@example.org"}, "code": {code}})
	if !strings.Contains(body(t, resp), "wrong or expired") {
		t.Fatal("a consumed code must not log in again")
	}
}

// TestLOGIN04_UnknownEmailsRevealNothing pins LOGIN-04.
func TestLOGIN04_UnknownEmailsRevealNothing(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	before := len(h.Mailer.Messages)

	resp := h.PostForm("/login", url.Values{"email": {"nobody@example.org"}})
	unknown := body(t, resp)
	if len(h.Mailer.Messages) != before {
		t.Fatal("unknown email must trigger no send")
	}

	h.Member("bob")
	mark := len(h.Mailer.Messages)
	resp = h.PostForm("/login", url.Values{"email": {"bob@example.org"}})
	known := body(t, resp)
	if len(h.Mailer.Messages) != mark+1 {
		t.Fatal("known email must trigger exactly one send")
	}
	if known != strings.ReplaceAll(unknown, "nobody@example.org", "bob@example.org") {
		t.Fatal("known and unknown responses must be indistinguishable")
	}
}

// TestLOGIN05_CodeRequestsAreRateLimited pins LOGIN-05.
func TestLOGIN05_CodeRequestsAreRateLimited(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	h.Member("carol")
	// Member() consumed part of the window; reset by advancing an hour.
	h.Clock.Advance(61 * time.Minute)

	for i := 0; i < 3; i++ {
		resp := h.PostForm("/login", url.Values{"email": {"carol@example.org"}})
		resp.Body.Close()
	}
	resp := h.PostForm("/login", url.Values{"email": {"carol@example.org"}})
	if !strings.Contains(body(t, resp), "Too many codes") {
		t.Fatal("the 4th code request within an hour must be refused")
	}
	h.Clock.Advance(61 * time.Minute)
	resp = h.PostForm("/login", url.Values{"email": {"carol@example.org"}})
	if strings.Contains(body(t, resp), "Too many codes") {
		t.Fatal("the window must reopen after an hour")
	}
}

// TestLOGIN06_Logout pins the single-session half of LOGIN-06.
func TestLOGIN06_Logout(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	client := h.Member("dan")

	resp, _ := client.PostForm(h.Server.URL+"/logout", nil)
	resp.Body.Close()
	resp, _ = client.Get(h.Server.URL + "/members")
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Fatalf("after logout, member pages must redirect to /login, got %d %q", resp.StatusCode, loc)
	}
}

// --- follow ---

// TestFOLLOW01_FollowCreatesAPendingIdentity pins FOLLOW-01.
func TestFOLLOW01_FollowCreatesAPendingIdentity(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)

	resp := h.PostForm("/follow", url.Values{"email": {"marie@example.org"}})
	if !strings.Contains(body(t, resp), "Check your inbox") {
		t.Fatal("follow must ask for confirmation")
	}
	ident, err := h.Community().IdentityByEmail("marie@example.org")
	if err != nil || ident.Username != "marie" || ident.Status != "unconfirmed" {
		t.Fatalf("follower identity wrong: %+v %v", ident, err)
	}
	if len(ident.NsecEnc) == 0 {
		t.Fatal("follower must have an encrypted key")
	}
	msg, ok := h.Mailer.LastTo("marie@example.org")
	if !ok || !strings.Contains(msg.Text, "/follow/confirm?") || !strings.Contains(msg.Text, "@marie") {
		t.Fatal("confirmation email must carry the link and the username")
	}
}

// TestFOLLOW02_UsernameCollisionsResolve pins FOLLOW-02.
func TestFOLLOW02_UsernameCollisionsResolve(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)

	for i, email := range []string{"marie@a.test", "marie@b.test", "marie@c.test"} {
		resp := h.PostForm("/follow", url.Values{"email": {email}})
		resp.Body.Close()
		want := "marie"
		if i > 0 {
			want = fmt.Sprintf("marie%d", i)
		}
		ident, err := h.Community().IdentityByEmail(email)
		if err != nil || ident.Username != want {
			t.Fatalf("want %s, got %+v (%v)", want, ident, err)
		}
	}
}

// TestFOLLOW03_ConfirmationActivatesTheFollower pins FOLLOW-03's local half.
func TestFOLLOW03_ConfirmationActivatesTheFollower(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	resp := h.PostForm("/follow", url.Values{"email": {"marie@example.org"}})
	resp.Body.Close()

	msg, _ := h.Mailer.LastTo("marie@example.org")
	link := extractLink(t, msg.Text)
	resp = h.Get(link)
	if !strings.Contains(body(t, resp), "following as @marie") {
		t.Fatal("confirmation must show the username")
	}
	c := h.Community()
	ident, _ := c.IdentityByEmail("marie@example.org")
	if ident.Status != "active" || !ident.Newsletter {
		t.Fatalf("confirmed follower must be active with opt-in: %+v", ident)
	}
	roles, _ := c.RoleNames(ident.ID)
	if len(roles) != 1 || roles[0] != "follower" {
		t.Fatalf("confirmed follower must hold the follower role, got %v", roles)
	}
}

// TestFOLLOW04_UnconfirmedFollowsExpire pins FOLLOW-04.
func TestFOLLOW04_UnconfirmedFollowsExpire(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	resp := h.PostForm("/follow", url.Values{"email": {"ghost@example.org"}})
	resp.Body.Close()

	h.Clock.Advance(31 * 24 * time.Hour)
	// GC runs opportunistically on the next follow.
	resp = h.PostForm("/follow", url.Values{"email": {"other@example.org"}})
	resp.Body.Close()

	if _, err := h.Community().IdentityByEmail("ghost@example.org"); err == nil {
		t.Fatal("unconfirmed follower must be deleted after 30 days")
	}
}

// TestFOLLOW05_RefollowingDisclosesNothing pins FOLLOW-05.
func TestFOLLOW05_RefollowingDisclosesNothing(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	resp := h.PostForm("/follow", url.Values{"email": {"marie@example.org"}})
	first := body(t, resp)
	msg, _ := h.Mailer.LastTo("marie@example.org")
	resp = h.Get(extractLink(t, msg.Text))
	resp.Body.Close()

	resp = h.PostForm("/follow", url.Values{"email": {"marie@example.org"}})
	again := body(t, resp)
	if first != again {
		t.Fatal("re-following must render the identical page")
	}
	last, _ := h.Mailer.LastTo("marie@example.org")
	if !strings.Contains(last.Subject, "already follow") {
		t.Fatal("the existing follower must be told by email, not on the page")
	}
	var n int
	h.Community().DB.QueryRow(
		`SELECT COUNT(*) FROM identities WHERE email = 'marie@example.org'`).Scan(&n)
	if n != 1 {
		t.Fatalf("no second identity may exist, got %d", n)
	}
}

func extractLink(t *testing.T, text string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "/follow/confirm?") {
			return strings.TrimSpace(line)
		}
	}
	t.Fatal("no confirmation link in email")
	return ""
}
