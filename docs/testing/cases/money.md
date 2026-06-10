# Test cases — expenses, payments, fiscal hosts, treasury

Flow reference: [nostr/money.md](../../nostr/money.md).
Expenses inherit all CHAN-* behavior; these cases cover the money layer.

### MONEY-01 — an expense carries money fields; payout details are members-only
Given member @dan submits an expense (€84.50, IBAN + lightning payout, receipt)
Then the thread root carries amount, currency and payout tags, signed by dan
And with the thread public, a visitor sees title, amount and status
But payout details and the receipt render only for members — in the thread,
the channel list, and every feed

### MONEY-02 — expenses default to 2 steward approvals
Given a fresh Expenses channel
Then its approval policy defaults to {steward} × 2, author excluded, admin alone sufficient

### MONEY-03 — member payment: claim plus confirmation
Given an approved expense by @dan
When member @alice signs a payment claim (€84.50, lightning)
Then the thread shows the claim as unconfirmed; the expense is not paid
When @dan signs a reception confirmation referencing the claim
Then the payment counts, and @alice appears on the contributors page (+€84.50)
And visitors and externals cannot sign claims

### MONEY-04 — partial payments settle on the author's confirmation
Given an approved €100 expense
When @alice pays €60 (confirmed) and @bob pays €40 (confirmed) and @dan confirms settlement
Then the expense is paid, and both payers' contributions are recorded
And before the settlement confirmation it remained approved-not-paid

### MONEY-05 — unconfirmed claims are flagged
Given a claim with no confirmation
When 7 days pass (fake clock)
Then the thread and the claimer's view flag it as unconfirmed
And it never counted toward contributions or settlement

### MONEY-06 — a fiscal host is a member with hold_funds
Given entity "Nonprofit A" approved through the normal join flow (organization flag)
Then it has no treasury powers
When the admin grants it the fiscal host role
Then its ledger entries are treasury-recognized
And a regular member's forged ledger entry is rejected and never indexed

### MONEY-07 — credits attribute sources and create recognition
Given host A signs a credit "€840 · source: ticket sales (June meetup)"
And a credit "€10,000 · source: Foundation Z · earmark: travel"
Then the treasury shows A holding €10,840
And "Foundation Z" and "ticket sales (June meetup)" appear as contributors
(external identities, no login)
And host A appears as a fiscal-host member, not as the donor of those amounts

### MONEY-08 — a host pays an expense like anyone, plus a debit
Given an approved travel expense by @dan and host A's travel earmark
When A signs a debit referencing the expense and @dan confirms reception
Then the expense is paid, A's balance and the travel earmark decrease
And the unconfirmed debit decreased nothing

### MONEY-09 — balance attestations reconcile visibly
Given derived balance €9,160 for host A
When A attests €9,160, the treasury shows the checkpoint silently
When A attests €9,000 instead
Then the treasury displays the €160 discrepancy prominently — it is never hidden

### MONEY-10 — earmark "amount left" is correct
Given the €10,000 travel credit and two confirmed travel debits (€840, €1,200)
Then the treasury shows €7,960 left on the travel earmark
And expenses filtered by the matching category list against it

### MONEY-11 — treasury visibility
Given default settings
Then `/treasury` (balances, ledger, contributors) renders for visitors
When the admin toggles it members-only
Then visitors get not-found
And payout details and receipts never appear in either mode

### MONEY-12 — the money record is rebuildable from the relay
Given expenses, claims, confirmations, credits, debits and attestations
When ledger and contribution indexes are wiped and communityd restarts
Then the treasury, contributors page and every expense timeline render identically

### MONEY-13 — the host's own donation is self-sourced
Given host A signs a credit "€500 · source: Nonprofit A"
Then €500 counts as A's own contribution on the contributors page
And third-party-sourced credits never do
