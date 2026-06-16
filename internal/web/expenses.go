package web

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// The Expenses channel (docs/nostr/money.md): an expense is a thread with
// money fields; payment is a payer-signed claim plus an author-signed
// reception confirmation; the expense is paid when the author settles.
// The platform records money, it never moves it.

const expensesSlug = "expenses"

// unconfirmedWindow flags a claim the author hasn't confirmed (MONEY-05).
const unconfirmedWindow = 7 * 24 * time.Hour

type payout struct{ Type, Value, Extra string }

type payment struct {
	ClaimID   string
	Payer     string
	PayerPK   string
	Amount    string
	Currency  string
	Method    string
	Confirmed bool
	Stale     bool // unconfirmed past the window
	Time      string
}

type expense struct {
	ID         string
	Title      string
	Content    string
	Amount     string
	Currency   string
	Tax        string
	Category   string
	Payouts    []payout // members-only at render time
	Receipts   []string // members-only at render time
	Author     string
	AuthorPK   string
	Visibility string
	Status     string // pending | approved | paid
	Approvers  []string
	Required   int
	Replies    []reply
	Payments   []payment
	raw        *nostr.Event
}

func (a *App) expensesChannel(c *store.Community) (*store.Channel, bool) {
	ch, err := c.ChannelBySlug(expensesSlug)
	if err != nil || !ch.Enabled || ch.Template != "expense" {
		return nil, false
	}
	return ch, true
}

func (a *App) loadExpenses(c *store.Community) ([]*expense, error) {
	ch, ok := a.expensesChannel(c)
	if !ok {
		return nil, nil
	}
	p, ok := a.publisher(c)
	if !ok {
		return nil, nil
	}
	community, err := a.communityIdentity(c)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		return nil, err
	}
	events, err := p.QueryAs(ctx, community, claim, nostr.Filter{
		Kinds: []int{publish.KindThreadRoot, publish.KindComment,
			publish.KindApproval, publish.KindLabel},
		Tags:  nostr.TagMap{"h": []string{expensesSlug}},
		Limit: 1000,
	})
	if err != nil {
		return nil, err
	}

	byKind := map[int][]*nostr.Event{}
	for _, evt := range events {
		byKind[evt.Kind] = append(byKind[evt.Kind], evt)
	}

	exp := map[string]*expense{}
	var order []string
	for _, root := range byKind[publish.KindThreadRoot] {
		e := &expense{
			ID: root.ID, Content: root.Content, AuthorPK: root.PubKey,
			Visibility: ch.DefaultVisibility, Status: "pending",
			Required: ch.ApprovalsRequired, raw: root,
		}
		e.Title = tagVal(root, "title")
		e.Amount = tagVal(root, "amount")
		e.Currency = tagVal(root, "currency")
		e.Category = tagVal(root, "category")
		if v := tagVal(root, "visibility"); v != "" {
			e.Visibility = v
		}
		if tax := root.Tags.GetFirst([]string{"tax", ""}); tax != nil && len(*tax) > 1 {
			e.Tax = (*tax)[1]
			if len(*tax) > 2 && (*tax)[2] != "" {
				e.Tax += " (" + (*tax)[2] + "%)"
			}
		}
		for _, tag := range root.Tags.GetAll([]string{"payout", ""}) {
			po := payout{Type: at(tag, 1), Value: at(tag, 2), Extra: at(tag, 3)}
			e.Payouts = append(e.Payouts, po)
		}
		for _, tag := range root.Tags.GetAll([]string{"receipt", ""}) {
			if len(tag) > 1 {
				e.Receipts = append(e.Receipts, tag[1])
			}
		}
		if ident, err := c.IdentityByPubkey(root.PubKey); err == nil {
			e.Author = ident.Username
		} else {
			e.Author = root.PubKey[:8] + "…"
		}
		exp[root.ID] = e
		order = append(order, root.ID)
	}

	// Comments: ordinary replies, payment claims, and confirmations.
	confirmedClaims := map[string]bool{} // claim id -> author-confirmed
	for _, evt := range byKind[publish.KindComment] {
		root := firstETag(evt)
		e := exp[root]
		if e == nil {
			continue
		}
		switch tagVal(evt, "t") {
		case "payment-confirm":
			// Only the expense author's confirmation counts (MONEY-03).
			if evt.PubKey == e.AuthorPK {
				confirmedClaims[tagVal(evt, "claim")] = true
			}
		case "payment-claim":
			// handled in a second pass (needs confirmedClaims)
		default:
			r := reply{ID: evt.ID, Content: evt.Content, Time: formatTime(evt.CreatedAt)}
			if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
				r.Author = ident.Username
			}
			e.Replies = append(e.Replies, r)
		}
	}
	now := a.Now()
	for _, evt := range byKind[publish.KindComment] {
		if tagVal(evt, "t") != "payment-claim" {
			continue
		}
		e := exp[firstETag(evt)]
		if e == nil {
			continue
		}
		pm := payment{
			ClaimID: evt.ID, PayerPK: evt.PubKey,
			Amount: tagVal(evt, "amount"), Currency: tagVal(evt, "currency"),
			Method: tagVal(evt, "method"), Time: formatTime(evt.CreatedAt),
			Confirmed: confirmedClaims[evt.ID],
		}
		if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
			pm.Payer = ident.Username
		} else {
			pm.Payer = evt.PubKey[:8] + "…"
		}
		if !pm.Confirmed && now.Sub(time.Unix(int64(evt.CreatedAt), 0)) > unconfirmedWindow {
			pm.Stale = true
		}
		e.Payments = append(e.Payments, pm)
	}

	// Approvals (channel quorum) and the author's settled label.
	for _, evt := range byKind[publish.KindApproval] {
		e := exp[firstETag(evt)]
		if e == nil || evt.PubKey == e.AuthorPK {
			continue
		}
		ident, err := c.IdentityByPubkey(evt.PubKey)
		if err != nil {
			continue
		}
		isAdmin := a.isAdmin(c, ident.ID)
		holds, _ := c.HasAnyRole(ident.ID, ch.ApproveRoles)
		if !isAdmin && !holds {
			continue
		}
		if !contains(e.Approvers, ident.Username) {
			e.Approvers = append(e.Approvers, ident.Username)
		}
		if isAdmin || len(e.Approvers) >= ch.ApprovalsRequired {
			if e.Status == "pending" {
				e.Status = "approved"
			}
		}
	}
	for _, evt := range byKind[publish.KindLabel] {
		if evt.Tags.GetFirst([]string{"l", "paid"}) == nil {
			continue
		}
		e := exp[firstETag(evt)]
		if e != nil && evt.PubKey == e.AuthorPK {
			e.Status = "paid"
		}
	}

	out := make([]*expense, 0, len(order))
	for i := len(order) - 1; i >= 0; i-- {
		out = append(out, exp[order[i]])
	}
	return out, nil
}

func at(tag nostr.Tag, i int) string {
	if len(tag) > i {
		return tag[i]
	}
	return ""
}

// --- handlers ---

func (a *App) expenseList(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	ch, ok := a.expensesChannel(c)
	if c == nil || !ok {
		http.NotFound(w, r)
		return
	}
	viewer := identityFrom(r)
	isMember := viewer != nil && a.memberLevel(c, viewer)
	expenses, err := a.loadExpenses(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	var visible []*expense
	for _, e := range expenses {
		if !isMember && (e.Status == "pending" || e.Visibility != "public") {
			continue
		}
		if isMember && status != "" && e.Status != status {
			continue
		}
		visible = append(visible, e)
	}
	a.render(w, "expenses_list.html", map[string]any{
		"Title": ch.Name, "Channel": ch, "Expenses": visible,
		"IsMember": isMember, "Status": status,
	})
}

func (a *App) expenseNewForm(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	ch, ok := a.expensesChannel(c)
	if c == nil || !ok {
		http.NotFound(w, r)
		return
	}
	viewer := identityFrom(r)
	if viewer == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if !a.memberLevel(c, viewer) {
		http.NotFound(w, r)
		return
	}
	a.render(w, "expense_new.html", map[string]any{"Title": "New expense", "Channel": ch, "Error": ""})
}

func (a *App) expenseCreate(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	ch, ok := a.expensesChannel(c)
	if c == nil || !ok {
		http.NotFound(w, r)
		return
	}
	viewer := identityFrom(r)
	if viewer == nil || !a.memberLevel(c, viewer) {
		http.NotFound(w, r)
		return
	}
	fail := func(msg string) {
		a.render(w, "expense_new.html", map[string]any{"Title": "New expense", "Channel": ch, "Error": msg})
	}
	f := publish.ExpenseFields{
		Title:     strings.TrimSpace(r.FormValue("title")),
		Content:   strings.TrimSpace(r.FormValue("content")),
		Amount:    strings.TrimSpace(r.FormValue("amount")),
		Currency:  strings.ToUpper(strings.TrimSpace(r.FormValue("currency"))),
		TaxAmount: strings.TrimSpace(r.FormValue("tax_amount")),
		TaxRate:   strings.TrimSpace(r.FormValue("tax_rate")),
		Category:  strings.TrimSpace(r.FormValue("category")),
	}
	if f.Title == "" || !validAmount(f.Amount) || f.Currency == "" {
		fail("A title, a valid amount and a currency are required.")
		return
	}
	if f.TaxAmount != "" && !lessThanTotal(f.TaxAmount, f.Amount) {
		fail("The tax amount must be smaller than the total.")
		return
	}
	if iban := strings.TrimSpace(r.FormValue("iban")); iban != "" {
		f.Payouts = append(f.Payouts, publish.PayoutMethod{Type: "iban", Value: iban,
			Extra: strings.TrimSpace(r.FormValue("iban_holder"))})
	}
	if ln := strings.TrimSpace(r.FormValue("lightning")); ln != "" {
		f.Payouts = append(f.Payouts, publish.PayoutMethod{Type: "lightning", Value: ln})
	}
	if rc := strings.TrimSpace(r.FormValue("receipt")); rc != "" {
		f.Receipts = append(f.Receipts, rc)
	}
	visibility := r.FormValue("visibility")
	if visibility == "" {
		visibility = ch.DefaultVisibility
	} else if visibility != "public" && visibility != "members" {
		fail("Unknown visibility.")
		return
	} else if !ch.Overridable && visibility != ch.DefaultVisibility {
		fail("This channel's visibility is fixed.")
		return
	}

	p, _ := a.publisher(c)
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	cl, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	evt := publish.ExpenseRootEvent(expensesSlug, visibility, f, a.Now())
	if err := p.PublishAs(ctx, viewer, cl, evt); err != nil {
		a.Log.Error("expense create", "err", err)
		fail("Could not publish — try again.")
		return
	}
	http.Redirect(w, r, "/channels/expenses", http.StatusSeeOther)
}

func (a *App) findExpense(w http.ResponseWriter, r *http.Request) (*store.Community, *store.Channel, *store.Identity, *expense, bool) {
	c := communityFrom(r)
	ch, ok := a.expensesChannel(c)
	if c == nil || !ok {
		http.NotFound(w, r)
		return nil, nil, nil, nil, false
	}
	viewer := identityFrom(r)
	isMember := viewer != nil && a.memberLevel(c, viewer)
	expenses, err := a.loadExpenses(c)
	if err != nil {
		a.internalError(w, err)
		return nil, nil, nil, nil, false
	}
	id := r.PathValue("id")
	for _, e := range expenses {
		if e.ID != id {
			continue
		}
		if !isMember && (e.Status == "pending" || e.Visibility != "public") {
			break
		}
		return c, ch, viewer, e, isMember
	}
	http.NotFound(w, r)
	return nil, nil, nil, nil, false
}

func (a *App) expenseView(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, e, isMember := a.findExpense(w, r)
	if e == nil {
		return
	}
	canDecide := false
	if viewer != nil {
		canDecide = a.isAdmin(c, viewer.ID)
		if !canDecide {
			canDecide, _ = c.HasAnyRole(viewer.ID, ch.ApproveRoles)
		}
	}
	isAuthor := viewer != nil && viewer.Pubkey == e.AuthorPK
	// Payout details and receipts are members-only regardless of the
	// thread's visibility (MONEY-01).
	view := *e
	if !isMember {
		view.Payouts = nil
		view.Receipts = nil
	}
	a.render(w, "expense_thread.html", map[string]any{
		"Title": e.Title, "Channel": ch, "E": &view,
		"IsMember": isMember, "CanDecide": canDecide, "IsAuthor": isAuthor,
	})
}

func (a *App) expenseAct(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, e, isMember := a.findExpense(w, r)
	if e == nil {
		return
	}
	if viewer == nil || !isMember {
		http.NotFound(w, r)
		return
	}
	action := r.PathValue("action")
	p, _ := a.publisher(c)
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	cl, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	path := "/channels/expenses/t/" + e.ID
	isAuthor := viewer.Pubkey == e.AuthorPK

	var evt *nostr.Event
	switch action {
	case "reply":
		content := strings.TrimSpace(r.FormValue("content"))
		if content == "" {
			http.Redirect(w, r, path, http.StatusSeeOther)
			return
		}
		evt = publish.ReplyEvent(expensesSlug, e.ID, content, a.Now())
	case "pay":
		// A payment claim (MONEY-03). Any member can reimburse.
		amount := strings.TrimSpace(r.FormValue("amount"))
		if !validAmount(amount) {
			http.Redirect(w, r, path, http.StatusSeeOther)
			return
		}
		currency := strings.ToUpper(strings.TrimSpace(r.FormValue("currency")))
		if currency == "" {
			currency = e.Currency
		}
		evt = publish.PaymentClaimEvent(expensesSlug, e.ID, amount, currency,
			strings.TrimSpace(r.FormValue("method")), strings.TrimSpace(r.FormValue("note")), a.Now())
	case "confirm":
		if !isAuthor {
			http.NotFound(w, r)
			return
		}
		evt = publish.PaymentConfirmEvent(expensesSlug, e.ID, r.FormValue("claim"), a.Now())
	case "settle":
		if !isAuthor {
			http.NotFound(w, r)
			return
		}
		evt = publish.SettledLabelEvent(expensesSlug, e.ID, a.Now())
	case "approve", "decline":
		isAdmin := a.isAdmin(c, viewer.ID)
		holds, _ := c.HasAnyRole(viewer.ID, ch.ApproveRoles)
		if !isAdmin && !holds {
			http.NotFound(w, r)
			return
		}
		if action == "approve" {
			if isAuthor {
				http.Redirect(w, r, path+"?err=own", http.StatusSeeOther)
				return
			}
			community, _ := a.communityIdentity(c)
			evt = publish.ApprovalEvent(expensesSlug, e.raw, community.Pubkey, c.Slug, a.Now())
		} else {
			evt = publish.DeclineEvent(expensesSlug, e.ID, strings.TrimSpace(r.FormValue("reason")), a.Now())
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err := p.PublishAs(ctx, viewer, cl, evt); err != nil {
		a.Log.Error("expense action", "action", action, "err", err)
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// --- contributions ---

type contribution struct {
	Contributor string
	Amount      float64
	Currency    string
}

// memberContributions aggregates confirmed expense payments per payer and
// currency (MONEY-03/04). Fiscal-host credit sources are folded in by the
// treasury (commit 2).
func (a *App) memberContributions(c *store.Community) []contribution {
	expenses, err := a.loadExpenses(c)
	if err != nil {
		return nil
	}
	agg := map[string]float64{} // "user|currency" -> amount
	for _, e := range expenses {
		for _, pm := range e.Payments {
			if !pm.Confirmed {
				continue
			}
			agg[pm.Payer+"|"+pm.Currency] += parseAmount(pm.Amount)
		}
	}
	var out []contribution
	for k, amt := range agg {
		parts := strings.SplitN(k, "|", 2)
		out = append(out, contribution{Contributor: parts[0], Amount: amt, Currency: parts[1]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Amount > out[j].Amount })
	return out
}

func (a *App) contributorsPage(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, "contributors.html", map[string]any{
		"Title": "Contributors", "Contributions": a.memberContributions(c),
	})
}

func validAmount(s string) bool {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil && v > 0
}

func parseAmount(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func lessThanTotal(tax, total string) bool {
	t, total2 := parseAmount(tax), parseAmount(total)
	return t > 0 && t < total2
}
