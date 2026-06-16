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

func ledger(t *testing.T, h *harness.H, client *http.Client, v url.Values) *http.Response {
	t.Helper()
	resp, err := client.PostForm(h.Server.URL+"/treasury/ledger", v)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func treasury(t *testing.T, h *harness.H, client *http.Client) string {
	t.Helper()
	return body(t, mustGet(t, client, h.Server.URL+"/treasury"))
}

// TestMONEY06_FiscalHostIsAMemberWithHoldFunds pins MONEY-06.
func TestMONEY06_FiscalHostIsAMemberWithHoldFunds(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	an := h.Member("an")

	// No treasury powers before the role.
	resp := ledger(t, h, an, url.Values{"type": {"credit"}, "amount": {"100"}, "currency": {"EUR"}, "source": {"x"}, "source_type": {"aggregate"}})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("a non-host must not record ledger entries")
	}

	c := h.Community()
	anIdent, _ := c.IdentityByUsername("an")
	if err := c.AssignRole(anIdent.ID, "fiscal host"); err != nil {
		t.Fatal(err)
	}
	resp = ledger(t, h, an, url.Values{"type": {"credit"}, "amount": {"500"}, "currency": {"EUR"}, "source": {"donors"}, "source_type": {"aggregate"}})
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("a host must record entries, got %d", resp.StatusCode)
	}
	if !strings.Contains(treasury(t, h, h.Client()), "500.00") {
		t.Fatal("the host's credit must appear in the treasury")
	}

	// A forged ledger entry from a non-host is never indexed.
	h.Member("dan")
	h.PublishRelayAs(t, "dan", &nostr.Event{Kind: 3939, Tags: nostr.Tags{
		{"t", "credit"}, {"amount", "99999"}, {"currency", "EUR"}, {"source", "fake", "aggregate"},
	}})
	if strings.Contains(treasury(t, h, h.Client()), "99999") {
		t.Fatal("a forged entry from a non-host must never be indexed")
	}
}

func newHost(t *testing.T, h *harness.H, username string) *http.Client {
	t.Helper()
	cl := h.Member(username, "fiscal host")
	return cl
}

// TestMONEY07_CreditsAttributeSources pins MONEY-07.
func TestMONEY07_CreditsAttributeSources(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	a := newHost(t, h, "citizenspring")

	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"840"}, "currency": {"EUR"},
		"source": {"ticket sales June"}, "source_type": {"aggregate"}}).Body.Close()
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"10000"}, "currency": {"EUR"},
		"source": {"Foundation for the Commons"}, "source_type": {"identity"}, "earmark": {"events"}}).Body.Close()

	page := treasury(t, h, h.Client())
	if !strings.Contains(page, "10840.00") {
		t.Fatal("the treasury must show the host holding 10,840")
	}
	if !strings.Contains(page, "Foundation for the Commons") || !strings.Contains(page, "ticket sales June") {
		t.Fatal("both sources must appear as contributors")
	}
	// The named source became a claimable (unclaimed) identity.
	c := h.Community()
	src, err := c.IdentityByUsername("foundation-for-the-commons")
	if err != nil || src.Status != "unclaimed" {
		t.Fatalf("the named source must become an unclaimed identity: %v", err)
	}
}

// TestMONEY08_HostPaysAnExpensePlusDebit pins MONEY-08.
func TestMONEY08_HostPaysAnExpensePlusDebit(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableExpenses(t, h)
	a := newHost(t, h, "host")
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	dan := h.Member("dan")

	// Host holds grant money, earmark travel.
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"10000"}, "currency": {"EUR"},
		"source": {"Foundation"}, "source_type": {"identity"}, "earmark": {"travel"}}).Body.Close()

	id := createExpense(t, h, dan, expenseForm("Train", "1210", "EUR", url.Values{"category": {"travel"}}))
	approveTwice(t, h, id, alice, bob)

	// The host signs a debit referencing the expense (decreases balance).
	ledger(t, h, a, url.Values{"type": {"debit"}, "amount": {"1210"}, "currency": {"EUR"},
		"earmark": {"travel"}, "expense": {id}}).Body.Close()
	page := treasury(t, h, h.Client())
	if !strings.Contains(page, "8790.00") {
		t.Fatal("the debit must decrease the travel earmark to 8,790")
	}

	// The author confirms reception → settles → paid.
	r, _ := dan.PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/settle", nil)
	r.Body.Close()
	if expenseStatus(t, h, dan, id) != "paid" {
		t.Fatal("the expense must be paid after the author settles")
	}
}

// TestMONEY09_AttestationsReconcileVisibly pins MONEY-09.
func TestMONEY09_AttestationsReconcileVisibly(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	a := newHost(t, h, "host")
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"10000"}, "currency": {"EUR"}, "source": {"x"}, "source_type": {"self"}}).Body.Close()
	ledger(t, h, a, url.Values{"type": {"debit"}, "amount": {"840"}, "currency": {"EUR"}}).Body.Close()

	// Correct attestation: no discrepancy.
	ledger(t, h, a, url.Values{"type": {"attestation"}, "amount": {"9160"}, "currency": {"EUR"}}).Body.Close()
	if strings.Contains(treasury(t, h, h.Client()), "discrepancy") {
		t.Fatal("a matching attestation must show no discrepancy")
	}
	// Wrong attestation: discrepancy displayed.
	ledger(t, h, a, url.Values{"type": {"attestation"}, "amount": {"9000"}, "currency": {"EUR"}}).Body.Close()
	if !strings.Contains(treasury(t, h, h.Client()), "discrepancy") {
		t.Fatal("a mismatched attestation must display the discrepancy")
	}
}

// TestMONEY10_EarmarkAmountLeft pins MONEY-10.
func TestMONEY10_EarmarkAmountLeft(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	a := newHost(t, h, "host")
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"10000"}, "currency": {"EUR"}, "source": {"F"}, "source_type": {"identity"}, "earmark": {"travel"}}).Body.Close()
	ledger(t, h, a, url.Values{"type": {"debit"}, "amount": {"840"}, "currency": {"EUR"}, "earmark": {"travel"}}).Body.Close()
	ledger(t, h, a, url.Values{"type": {"debit"}, "amount": {"1200"}, "currency": {"EUR"}, "earmark": {"travel"}}).Body.Close()

	page := treasury(t, h, h.Client())
	if !strings.Contains(page, "7960.00") || !strings.Contains(page, "travel") {
		t.Fatal("the travel earmark must show 7,960 left")
	}
}

// TestMONEY11_TreasuryVisibility pins MONEY-11.
func TestMONEY11_TreasuryVisibility(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)

	// Public by default.
	resp := h.Get("/treasury")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("the treasury must be public by default")
	}
	r, _ := h.Admin.PostForm(h.Server.URL+"/settings/treasury", url.Values{"visibility": {"members"}})
	r.Body.Close()
	resp = h.Get("/treasury")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("members-only treasury must 404 for visitors")
	}
}

// TestMONEY12_RebuildableFromTheRelay pins MONEY-12.
func TestMONEY12_RebuildableFromTheRelay(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	a := newHost(t, h, "host")
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"1234"}, "currency": {"EUR"}, "source": {"x"}, "source_type": {"self"}}).Body.Close()

	before := treasury(t, h, h.Client())
	h.Restart()
	after := treasury(t, h, h.Client())
	if !strings.Contains(before, "1234.00") || !strings.Contains(after, "1234.00") {
		t.Fatal("the treasury must render identically after a restart (relay-derived)")
	}
}

// TestMONEY13_SelfSourcedIsTheHostsDonation pins MONEY-13.
func TestMONEY13_SelfSourcedIsTheHostsDonation(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	a := newHost(t, h, "nonprofita")
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"500"}, "currency": {"EUR"}, "source": {"Nonprofit A"}, "source_type": {"self"}}).Body.Close()
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"800"}, "currency": {"EUR"}, "source": {"Someone Else"}, "source_type": {"identity"}}).Body.Close()

	page := treasury(t, h, h.Client())
	if !strings.Contains(page, "@nonprofita") || !strings.Contains(page, "500.00") {
		t.Fatal("a self-sourced credit must count as the host's own contribution")
	}
}

// TestMONEY14_RunningBalancesReconcile pins MONEY-14.
func TestMONEY14_RunningBalancesReconcile(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	a := newHost(t, h, "host")
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"9360"}, "currency": {"EUR"}, "source": {"x"}, "source_type": {"self"}, "balance": {"9360"}}).Body.Close()
	// Correct running balance after a debit: 9360-200=9160.
	ledger(t, h, a, url.Values{"type": {"debit"}, "amount": {"200"}, "currency": {"EUR"}, "balance": {"9160"}}).Body.Close()
	if strings.Contains(treasury(t, h, h.Client()), "running-balance mismatch") {
		t.Fatal("a correct running balance must not be flagged")
	}
	// Wrong running balance on a credit.
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"100"}, "currency": {"EUR"}, "source": {"x"}, "source_type": {"self"}, "balance": {"9500"}}).Body.Close()
	if !strings.Contains(treasury(t, h, h.Client()), "running-balance mismatch") {
		t.Fatal("a wrong running balance must be flagged")
	}
}

// TestMONEY15_ProofsAttach pins MONEY-15.
func TestMONEY15_ProofsAttach(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	a := newHost(t, h, "host")
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"840"}, "currency": {"EUR"}, "source": {"x"}, "source_type": {"aggregate"},
		"proof_type": {"stripe"}, "proof_value": {"ch_123"}}).Body.Close()
	ledger(t, h, a, url.Values{"type": {"debit"}, "amount": {"100"}, "currency": {"EUR"},
		"proof_type": {"txhash"}, "proof_value": {"0xabc"}}).Body.Close()
	ledger(t, h, a, url.Values{"type": {"credit"}, "amount": {"5"}, "currency": {"EUR"}, "source": {"y"}, "source_type": {"aggregate"},
		"proof_type": {"wise"}, "proof_value": {"w-9"}}).Body.Close()

	page := treasury(t, h, h.Client())
	for _, want := range []string{"stripe:ch_123", "txhash:0xabc", "wise:w-9"} {
		if !strings.Contains(page, want) {
			t.Fatalf("proof %q must render (unknown types generically)", want)
		}
	}
}

// TestMONEY17_IndividualCanBeAFiscalHost pins MONEY-17.
func TestMONEY17_IndividualCanBeAFiscalHost(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	carol := h.Member("carol", "fiscal host") // a person, no organization flag

	ledger(t, h, carol, url.Values{"type": {"credit"}, "amount": {"200"}, "currency": {"EUR"},
		"source": {"cash jar June"}, "source_type": {"aggregate"}}).Body.Close()
	ledger(t, h, carol, url.Values{"type": {"attestation"}, "amount": {"200"}, "currency": {"EUR"}}).Body.Close()

	page := treasury(t, h, h.Client())
	if !strings.Contains(page, "@carol") || !strings.Contains(page, "200.00") {
		t.Fatal("an individual must be able to run a cash ledger")
	}
}
