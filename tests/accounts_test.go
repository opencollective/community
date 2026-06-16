//go:build integration

package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/opencollective/community/tests/harness"
)

// createOnBehalf creates an unclaimed account managed by the given member.
func createOnBehalf(t *testing.T, h *harness.H, manager *http.Client, name, username string) {
	t.Helper()
	resp, err := manager.PostForm(h.Server.URL+"/accounts", url.Values{
		"name": {name}, "username": {username},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("create on behalf: want 303, got %d", resp.StatusCode)
	}
}

// TestUNCL02_OnlyCreatorAndAdminsControl pins UNCL-02.
func TestUNCL02_OnlyCreatorAndAdminsControl(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	marie := h.Member("marie")
	bob := h.Member("bob")
	createOnBehalf(t, h, marie, "Foundation Z", "foundationz")

	// The creator controls it.
	resp, _ := marie.Get(h.Server.URL + "/accounts/foundationz")
	if resp.StatusCode != 200 {
		t.Fatal("the creator must control the account")
	}
	resp.Body.Close()
	// The admin controls it.
	resp, _ = h.Admin.Get(h.Server.URL + "/accounts/foundationz")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("the admin must control the account")
	}
	// Another member cannot.
	resp, _ = bob.Get(h.Server.URL + "/accounts/foundationz")
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("a non-manager must not control the account")
	}
	resp, _ = bob.PostForm(h.Server.URL+"/accounts/foundationz/claim-email", url.Values{"email": {"x@y.z"}})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("a non-manager must not set the claim email")
	}
}

// TestUNCL03_ClaimingTransfersControl pins UNCL-03.
func TestUNCL03_ClaimingTransfersControl(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	marie := h.Member("marie")
	createOnBehalf(t, h, marie, "Foundation Z", "foundationz")

	c := h.Community()
	account, _ := c.IdentityByUsername("foundationz")
	npubBefore := account.Pubkey

	// Marie sends a claim code.
	resp, _ := marie.PostForm(h.Server.URL+"/accounts/foundationz/claim-email",
		url.Values{"email": {"foundation@example.org"}})
	resp.Body.Close()
	code := h.Mailer.LastCodeTo("foundation@example.org")
	if code == "" {
		t.Fatal("a claim code must be emailed")
	}

	// The recipient claims it.
	client := h.Client()
	resp, _ = client.PostForm(h.Server.URL+"/accounts/claim",
		url.Values{"email": {"foundation@example.org"}, "code": {code}})
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("claiming: want 303, got %d", resp.StatusCode)
	}

	account, _ = c.IdentityByUsername("foundationz")
	if account.Status != "active" || account.Pubkey != npubBefore {
		t.Fatal("claiming must activate the account while keeping its npub")
	}
	// Marie loses control.
	marieIdent, _ := c.IdentityByUsername("marie")
	managed, _ := c.IsManager(account.ID, marieIdent.ID)
	if managed {
		t.Fatal("management must be paused after a claim")
	}
	resp, _ = marie.Get(h.Server.URL + "/accounts/foundationz")
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("the former manager must lose control after the claim")
	}
}

// TestUNCL04_HandoverBeforeClaim pins UNCL-04.
func TestUNCL04_HandoverBeforeClaim(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	marie := h.Member("marie")
	createOnBehalf(t, h, marie, "Foundation Z", "foundationz")

	// First email, then replace it.
	resp, _ := marie.PostForm(h.Server.URL+"/accounts/foundationz/claim-email", url.Values{"email": {"old@example.org"}})
	resp.Body.Close()
	oldCode := h.Mailer.LastCodeTo("old@example.org")
	resp, _ = marie.PostForm(h.Server.URL+"/accounts/foundationz/claim-email", url.Values{"email": {"new@example.org"}})
	resp.Body.Close()
	newCode := h.Mailer.LastCodeTo("new@example.org")

	// The old address can no longer claim.
	client := h.Client()
	resp, _ = client.PostForm(h.Server.URL+"/accounts/claim", url.Values{"email": {"old@example.org"}, "code": {oldCode}})
	body := body(t, resp)
	if !strings.Contains(body, "wrong or expired") {
		t.Fatal("a replaced claim email must no longer claim")
	}
	// The new address can.
	resp, _ = client.PostForm(h.Server.URL+"/accounts/claim", url.Values{"email": {"new@example.org"}, "code": {newCode}})
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatal("the latest claim email must claim")
	}
}

// TestUNCL05_RolesAndHoldFunds pins UNCL-05.
func TestUNCL05_RolesAndHoldFunds(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	marie := h.Member("marie")
	createOnBehalf(t, h, marie, "Foundation Z", "foundationz")
	c := h.Community()
	account, _ := c.IdentityByUsername("foundationz")

	// The member role can be granted, but it grants no session/capability.
	resp, _ := h.Admin.PostForm(h.Server.URL+"/roles/member/members", url.Values{"username": {"foundationz"}})
	resp.Body.Close()

	// hold_funds (fiscal host) is refused while unclaimed AND unmanaged.
	// foundationz IS managed by marie, so granting it should succeed.
	resp, _ = h.Admin.PostForm(h.Server.URL+"/roles/fiscal host/members", url.Values{"username": {"foundationz"}})
	resp.Body.Close()
	held, _ := c.HasPermission(account.ID, "hold_funds")
	if !held {
		t.Fatal("a managed account may hold funds (operable)")
	}

	// An unclaimed AND unmanaged account is refused hold_funds.
	dek, _ := h.App.DEK(c)
	_ = dek
	// Create an unmanaged unclaimed account directly.
	if _, err := c.DB.Exec(`INSERT INTO identities (username, pubkey, nsec_enc, status, created_at)
		VALUES ('orphan', 'deadbeef', x'00', 'unclaimed', 0)`); err != nil {
		t.Fatal(err)
	}
	resp, _ = h.Admin.PostForm(h.Server.URL+"/roles/fiscal host/members", url.Values{"username": {"orphan"}})
	resp.Body.Close()
	orphan, _ := c.IdentityByUsername("orphan")
	if ok, _ := c.HasPermission(orphan.ID, "hold_funds"); ok {
		t.Fatal("an unclaimed, unmanaged account must be refused hold_funds")
	}
}

