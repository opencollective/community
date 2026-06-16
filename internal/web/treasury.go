package web

import (
	"context"

	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/identity"
	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// The treasury (docs/nostr/money.md § fiscal hosts, § the treasury). The
// platform records money, never moves it: fiscal hosts sign an append-only
// ledger of credits, debits and balance attestations; the treasury derives
// balances and surfaces — never hides — discrepancies. Public by default.

const setTreasuryVisibility = "treasury_visibility" // "members" hides it

type ledgerEntry struct {
	ID            string
	Type          string // credit | debit | attestation
	HostPK        string
	Host          string
	Amount        float64
	AmountStr     string
	Currency      string
	Source        string
	SourceType    string
	Earmark       string
	ExpenseID     string
	Proof         string
	StatedBalance string
	Mismatch      bool
	Memo          string
	Time          string
	CreatedAt     int64
}

// loadLedger reads ledger entries, keeping only those signed by current
// hold_funds holders (MONEY-06: forged entries are never indexed), sorted
// oldest-first, with running-balance mismatches flagged (MONEY-14).
func (a *App) loadLedger(c *store.Community) ([]*ledgerEntry, error) {
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
		Kinds: []int{publish.KindLedger}, Limit: 2000,
	})
	if err != nil {
		return nil, err
	}
	hosts, err := c.PubkeysWithPermission(store.PermHoldFunds)
	if err != nil {
		return nil, err
	}
	hostSet := map[string]bool{}
	for _, pk := range hosts {
		hostSet[pk] = true
	}

	var entries []*ledgerEntry
	for _, evt := range events {
		if !hostSet[evt.PubKey] {
			continue // not a current fiscal host
		}
		e := &ledgerEntry{
			ID: evt.ID, Type: tagVal(evt, "t"), HostPK: evt.PubKey,
			Currency: tagVal(evt, "currency"), Earmark: tagVal(evt, "earmark"),
			ExpenseID: tagVal(evt, "e"), Memo: evt.Content,
			StatedBalance: tagVal(evt, "balance"),
			AmountStr:     tagVal(evt, "amount"), Amount: parseAmount(tagVal(evt, "amount")),
			Time: formatTime(evt.CreatedAt), CreatedAt: int64(evt.CreatedAt),
		}
		if src := evt.Tags.GetFirst([]string{"source", ""}); src != nil && len(*src) > 1 {
			e.Source = (*src)[1]
			if len(*src) > 2 {
				e.SourceType = (*src)[2]
			}
		}
		if pr := evt.Tags.GetFirst([]string{"proof", ""}); pr != nil && len(*pr) > 2 {
			e.Proof = (*pr)[1] + ":" + (*pr)[2]
		}
		if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
			e.Host = ident.Username
		} else {
			e.Host = evt.PubKey[:8] + "…"
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt < entries[j].CreatedAt })

	// Running-balance reconciliation: a credit/debit that states a balance
	// is checked against the derived balance after applying it.
	running := map[string]float64{} // host|currency|earmark
	for _, e := range entries {
		if e.Type == "attestation" {
			continue
		}
		key := e.HostPK + "|" + e.Currency + "|" + e.Earmark
		if e.Type == "credit" {
			running[key] += e.Amount
		} else if e.Type == "debit" {
			running[key] -= e.Amount
		}
		if e.StatedBalance != "" && !almostEqual(parseAmount(e.StatedBalance), running[key]) {
			e.Mismatch = true
		}
	}
	return entries, nil
}

type balanceRow struct {
	Host       string
	Currency   string
	Earmark    string
	Derived    float64
	Attested   float64
	HasAttest  bool
	Discrepant bool
}

type totalRow struct {
	Host     string
	Currency string
	Amount   float64
}

// deriveTotals sums credits minus debits per host and currency across all
// earmarks (MONEY-07: the host's total held).
func deriveTotals(entries []*ledgerEntry) []totalRow {
	tot := map[string]float64{}
	for _, e := range entries {
		key := e.Host + "|" + e.Currency
		switch e.Type {
		case "credit":
			tot[key] += e.Amount
		case "debit":
			tot[key] -= e.Amount
		}
	}
	var rows []totalRow
	for key, amt := range tot {
		parts := strings.SplitN(key, "|", 2)
		rows = append(rows, totalRow{Host: parts[0], Currency: parts[1], Amount: amt})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Host < rows[j].Host })
	return rows
}

func deriveBalances(entries []*ledgerEntry) []balanceRow {
	derived := map[string]float64{}
	latestAttest := map[string]float64{}
	hasAttest := map[string]bool{}
	meta := map[string][3]string{}
	for _, e := range entries {
		key := e.Host + "|" + e.Currency + "|" + e.Earmark
		meta[key] = [3]string{e.Host, e.Currency, e.Earmark}
		switch e.Type {
		case "credit":
			derived[key] += e.Amount
		case "debit":
			derived[key] -= e.Amount
		case "attestation":
			latestAttest[key] = e.Amount
			hasAttest[key] = true
		}
	}
	var rows []balanceRow
	for key, m := range meta {
		row := balanceRow{Host: m[0], Currency: m[1], Earmark: m[2], Derived: derived[key]}
		if hasAttest[key] {
			row.HasAttest = true
			row.Attested = latestAttest[key]
			row.Discrepant = !almostEqual(row.Derived, row.Attested)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Host != rows[j].Host {
			return rows[i].Host < rows[j].Host
		}
		return rows[i].Earmark < rows[j].Earmark
	})
	return rows
}

// treasuryContributions combines confirmed member payments with credit
// sources (MONEY-07/13). Self-sourced credits count as the host's own
// contribution; identity sources are recognised parties; aggregates are
// plain-text lines.
func (a *App) treasuryContributions(c *store.Community, entries []*ledgerEntry) []contribution {
	agg := map[string]float64{}
	add := func(name, currency string, amt float64) {
		agg[name+"|"+currency] += amt
	}
	for _, ctr := range a.memberContributions(c) {
		add("@"+ctr.Contributor, ctr.Currency, ctr.Amount)
	}
	for _, e := range entries {
		if e.Type != "credit" {
			continue
		}
		switch e.SourceType {
		case "self":
			add("@"+e.Host, e.Currency, e.Amount)
		case "aggregate":
			add(e.Source, e.Currency, e.Amount)
		default: // identity
			add(e.Source, e.Currency, e.Amount)
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

func (a *App) treasuryPage(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	viewer := identityFrom(r)
	isMember := viewer != nil && a.memberLevel(c, viewer)
	if vis, _ := c.Setting(setTreasuryVisibility); vis == "members" && !isMember {
		http.NotFound(w, r)
		return
	}
	entries, err := a.loadLedger(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	canHost := false
	if viewer != nil {
		canHost, _ = c.HasPermission(viewer.ID, store.PermHoldFunds)
	}
	// Recent entries newest-first for display.
	recent := make([]*ledgerEntry, len(entries))
	for i := range entries {
		recent[i] = entries[len(entries)-1-i]
	}
	a.render(w, "treasury.html", map[string]any{
		"Title": "Treasury", "Totals": deriveTotals(entries),
		"Balances": deriveBalances(entries),
		"Ledger":   recent, "Contributions": a.treasuryContributions(c, entries),
		"CanHost": canHost,
	})
}

// requireHost gates ledger writes on hold_funds (MONEY-06).
func (a *App) requireHost(h func(http.ResponseWriter, *http.Request, *store.Community, *store.Identity)) http.HandlerFunc {
	return a.requireMember(func(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
		ok, _ := c.HasPermission(viewer.ID, store.PermHoldFunds)
		if !ok {
			http.NotFound(w, r)
			return
		}
		h(w, r, c, viewer)
	})
}

func (a *App) ledgerSubmit(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	entryType := r.FormValue("type")
	amount := strings.TrimSpace(r.FormValue("amount"))
	currency := strings.ToUpper(strings.TrimSpace(r.FormValue("currency")))
	earmark := strings.TrimSpace(r.FormValue("earmark"))
	balance := strings.TrimSpace(r.FormValue("balance"))
	memo := strings.TrimSpace(r.FormValue("memo"))
	var proof *publish.LedgerProof
	if pt := strings.TrimSpace(r.FormValue("proof_type")); pt != "" {
		proof = &publish.LedgerProof{Type: pt, Value: strings.TrimSpace(r.FormValue("proof_value"))}
	}
	if (entryType != "attestation" && !validAmount(amount)) || currency == "" {
		http.Redirect(w, r, "/treasury?err=amount", http.StatusSeeOther)
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

	var evt *nostr.Event
	switch entryType {
	case "credit":
		source := strings.TrimSpace(r.FormValue("source"))
		sourceType := r.FormValue("source_type") // identity | aggregate | self
		if source == "" {
			http.Redirect(w, r, "/treasury?err=source", http.StatusSeeOther)
			return
		}
		// A named third-party source becomes a claimable contributor
		// identity (MONEY-07, UNCL-01); aggregates and self stay labels.
		if sourceType == "identity" {
			a.ensureSourceIdentity(c, source, viewer.ID)
		}
		evt = publish.CreditEntry(amount, currency, source, sourceType, earmark, balance, memo, proof, a.Now())
	case "debit":
		evt = publish.DebitEntry(amount, currency, strings.TrimSpace(r.FormValue("expense")), earmark, balance, memo, proof, a.Now())
	case "attestation":
		evt = publish.AttestationEntry(amount, currency, earmark, memo,
			strings.TrimSpace(r.FormValue("pof_address")), strings.TrimSpace(r.FormValue("pof_sig")), a.Now())
	default:
		http.NotFound(w, r)
		return
	}
	if err := p.PublishAs(ctx, viewer, cl, evt); err != nil {
		a.Log.Error("ledger entry", "type", entryType, "err", err)
	}
	http.Redirect(w, r, "/treasury", http.StatusSeeOther)
}

var srcSlug = regexp.MustCompile(`[^a-z0-9]+`)

// ensureSourceIdentity find-or-creates an unclaimed identity for a named
// credit source (UNCL-01: creation by attribution). The attributing host
// becomes its first manager, so it is claimable later.
func (a *App) ensureSourceIdentity(c *store.Community, name string, creatorID int64) {
	base := strings.Trim(srcSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" || identity.ValidateUsername(base) != nil {
		return
	}
	if _, err := c.IdentityByUsername(base); err == nil {
		return // already exists
	}
	dek, ok := a.DEK(c)
	if !ok {
		return
	}
	kp, err := identity.Generate()
	if err != nil {
		return
	}
	enc, err := encryptSecret(dek, kp)
	if err != nil {
		return
	}
	ident, err := c.CreateIdentity(base, "", kp.PublicHex, enc, true, "unclaimed", a.Now())
	if err != nil {
		a.Log.Error("source identity", "name", name, "err", err)
		return
	}
	_ = c.UpdateProfile(ident.ID, name, false)
	_ = c.AddManager(ident.ID, creatorID, creatorID, a.Now())
}

func (a *App) settingsTreasury(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	vis := "public"
	if r.FormValue("visibility") == "members" {
		vis = "members"
	}
	if err := c.SetSetting(setTreasuryVisibility, vis); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/settings/community", http.StatusSeeOther)
}

func almostEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.005
}
