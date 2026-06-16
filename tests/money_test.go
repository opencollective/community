//go:build integration

package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/tests/harness"
)

func enableExpenses(t *testing.T, h *harness.H) {
	t.Helper()
	resp, err := h.Admin.PostForm(h.Server.URL+"/settings/channels/expenses", url.Values{
		"enabled": {"1"}, "approve_roles": {"steward"}, "approvals_required": {"2"},
		"default_visibility": {"members"}, "overridable": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	time.Sleep(300 * time.Millisecond)
}

func createExpense(t *testing.T, h *harness.H, client *http.Client, v url.Values) string {
	t.Helper()
	resp, err := client.PostForm(h.Server.URL+"/channels/expenses", v)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 303 {
		return ""
	}
	for _, evt := range h.QueryRelayAs("xavier", nostr.Filter{
		Kinds: []int{11}, Tags: nostr.TagMap{"h": []string{"expenses"}},
	}) {
		if tagOf(evt, "title") == v.Get("title") {
			return evt.ID
		}
	}
	t.Fatal("expense not found on the relay")
	return ""
}

func expenseForm(title, amount, currency string, extra url.Values) url.Values {
	v := url.Values{"title": {title}, "amount": {amount}, "currency": {currency}, "content": {"please reimburse"}}
	for k, vals := range extra {
		v[k] = vals
	}
	return v
}

func approveTwice(t *testing.T, h *harness.H, id string, approvers ...*http.Client) {
	t.Helper()
	for _, cl := range approvers {
		r, _ := cl.PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/approve", nil)
		r.Body.Close()
	}
}

// TestMONEY01_ExpenseFieldsPayoutMembersOnly pins MONEY-01.
func TestMONEY01_ExpenseFieldsPayoutMembersOnly(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableExpenses(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")

	id := createExpense(t, h, dan, expenseForm("Photographer", "1210", "EUR", url.Values{
		"tax_amount": {"210"}, "tax_rate": {"21"},
		"iban": {"BE71096123456769"}, "iban_holder": {"Jan"},
		"receipt": {"https://blossom/invoice.pdf"}, "visibility": {"public"},
	}))

	evt := h.QueryRelayAs("dan", nostr.Filter{IDs: []string{id}})[0]
	if tagOf(evt, "amount") != "1210" || tagOf(evt, "currency") != "EUR" {
		t.Fatal("the root must carry amount and currency")
	}
	if tax := evt.Tags.GetFirst([]string{"tax", ""}); tax == nil || (*tax)[1] != "210" {
		t.Fatal("the tax breakdown must be on the root")
	}

	// Approve it (pending threads are never public; ADR 0012). Then a
	// visitor sees a public expense's title and amount, not payout/receipt.
	approveTwice(t, h, id, alice, bob)
	page := body(t, h.Get("/channels/expenses/t/"+id))
	if !strings.Contains(page, "Photographer") {
		t.Fatal("a public expense's title must be visible")
	}
	if strings.Contains(page, "BE71096123456769") || strings.Contains(page, "invoice.pdf") {
		t.Fatal("payout details and receipts must be members-only")
	}
	// A member sees them.
	mpage := body(t, mustGet(t, dan, h.Server.URL+"/channels/expenses/t/"+id))
	if !strings.Contains(mpage, "BE71096123456769") {
		t.Fatal("members must see payout details")
	}

	// Tax not smaller than the total is rejected.
	resp, _ := dan.PostForm(h.Server.URL+"/channels/expenses",
		expenseForm("Bad tax", "100", "EUR", url.Values{"tax_amount": {"100"}}))
	if p := body(t, resp); !strings.Contains(p, "smaller than the total") {
		t.Fatal("tax >= total must be rejected")
	}
}

// TestMONEY02_ExpensesDefaultTwoStewards pins MONEY-02.
func TestMONEY02_ExpensesDefaultTwoStewards(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	c := h.Community()
	ch, err := c.ChannelBySlug("expenses")
	if err != nil || ch.ApprovalsRequired != 2 {
		t.Fatalf("expenses must default to 2 approvals, got %d (%v)", ch.ApprovalsRequired, err)
	}
}

// TestMONEY03_MemberPaymentClaimPlusConfirmation pins MONEY-03.
func TestMONEY03_MemberPaymentClaimPlusConfirmation(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableExpenses(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	id := createExpense(t, h, dan, expenseForm("Train tickets", "84.50", "EUR", nil))
	approveTwice(t, h, id, alice, bob)

	// Alice pays (a claim) — unconfirmed, no contribution yet.
	r, _ := alice.PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/pay",
		url.Values{"amount": {"84.50"}, "method": {"lightning"}})
	r.Body.Close()
	page := body(t, mustGet(t, dan, h.Server.URL+"/channels/expenses/t/"+id))
	if !strings.Contains(page, "unconfirmed") {
		t.Fatal("an unconfirmed claim must show as such")
	}
	if c := body(t, h.Get("/contributors")); strings.Contains(c, "@alice") {
		t.Fatal("an unconfirmed payment must not count as a contribution")
	}

	// The author confirms reception.
	claimID := h.QueryRelayAs("dan", nostr.Filter{
		Kinds: []int{1111}, Tags: nostr.TagMap{"h": []string{"expenses"}},
	})
	var cid string
	for _, evt := range claimID {
		if tagOf(evt, "t") == "payment-claim" {
			cid = evt.ID
		}
	}
	r, _ = dan.PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/confirm", url.Values{"claim": {cid}})
	r.Body.Close()

	if c := body(t, h.Get("/contributors")); !strings.Contains(c, "@alice") || !strings.Contains(c, "84.50") {
		t.Fatal("a confirmed payment must record alice as a contributor")
	}
	// A visitor cannot claim a payment.
	rr, _ := h.Client().PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/pay",
		url.Values{"amount": {"10"}})
	rr.Body.Close()
	if rr.StatusCode == 303 {
		t.Fatal("a visitor must not be able to claim a payment")
	}
}

// TestMONEY04_PartialPaymentsSettleOnAuthorConfirmation pins MONEY-04.
func TestMONEY04_PartialPaymentsSettleOnAuthorConfirmation(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableExpenses(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	id := createExpense(t, h, dan, expenseForm("Catering", "100", "EUR", nil))
	approveTwice(t, h, id, alice, bob)

	pay := func(cl *http.Client, amt string) {
		r, _ := cl.PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/pay", url.Values{"amount": {amt}})
		r.Body.Close()
	}
	pay(alice, "60")
	pay(bob, "40")

	// Confirm both claims.
	for _, evt := range h.QueryRelayAs("dan", nostr.Filter{Kinds: []int{1111}, Tags: nostr.TagMap{"h": []string{"expenses"}}}) {
		if tagOf(evt, "t") == "payment-claim" {
			r, _ := dan.PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/confirm", url.Values{"claim": {evt.ID}})
			r.Body.Close()
		}
	}

	// Still approved (not paid) until the author settles.
	if e := expenseStatus(t, h, dan, id); e != "approved" {
		t.Fatalf("expense must remain approved until settled, got %s", e)
	}
	r, _ := dan.PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/settle", nil)
	r.Body.Close()
	if e := expenseStatus(t, h, dan, id); e != "paid" {
		t.Fatalf("the author's settlement must mark it paid, got %s", e)
	}
	// Both payers recorded.
	c := body(t, h.Get("/contributors"))
	if !strings.Contains(c, "@alice") || !strings.Contains(c, "@bob") {
		t.Fatal("both payers must be recorded as contributors")
	}
}

// TestMONEY05_UnconfirmedClaimsAreFlagged pins MONEY-05.
func TestMONEY05_UnconfirmedClaimsAreFlagged(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableExpenses(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	bob := h.Member("bob", "steward")
	id := createExpense(t, h, dan, expenseForm("Supplies", "30", "EUR", nil))
	approveTwice(t, h, id, alice, bob)
	r, _ := alice.PostForm(h.Server.URL+"/channels/expenses/t/"+id+"/pay", url.Values{"amount": {"30"}})
	r.Body.Close()

	h.Clock.Advance(8 * 24 * time.Hour)
	page := body(t, mustGet(t, dan, h.Server.URL+"/channels/expenses/t/"+id))
	if !strings.Contains(page, "7+ days") {
		t.Fatal("an unconfirmed claim past the window must be flagged")
	}
	if c := body(t, h.Get("/contributors")); strings.Contains(c, "@alice") {
		t.Fatal("an unconfirmed claim must never count as a contribution")
	}
}

func expenseStatus(t *testing.T, h *harness.H, client *http.Client, id string) string {
	t.Helper()
	page := body(t, mustGet(t, client, h.Server.URL+"/channels/expenses/t/"+id))
	for _, s := range []string{"paid", "approved", "pending"} {
		if strings.Contains(page, `<span class="pill">`+s+`</span>`) {
			return s
		}
	}
	return "?"
}
