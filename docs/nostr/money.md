# Money: expenses, contributions, fiscal hosts

The platform **never moves money**. Money moves elsewhere — banks, Stripe,
chains, cash. What the platform keeps is an *attributable record of claims
about money*: who asked, who approved, who paid, who confirmed receiving,
who holds what on whose behalf. Every fact is a signed event on the
community's relay, like everything else
([decision 0013](../decisions/0013-expenses-contributions-fiscal-hosts.md)).

## Expenses

An expense is a thread in the Expenses channel
([channels.md](channels.md)) — a proposal with money fields:

| field | notes |
|---|---|
| title, description | what and why |
| amount + currency | decimal string + ISO 4217 code or asset code (`EUR`, `USD`, `BTC`, `USDC`) |
| category | optional; matches credit earmarks (below) |
| receipt(s) | Blossom uploads |
| payout method(s) | one or more of: `iban` (+ holder name, BIC), `bitcoin` address, `lightning` address, `eth`/stablecoin (chain + asset + address), `other` (free text) |

**Privacy floor**: payout method details and receipts are **always
members-only**, regardless of the thread's visibility — they carry IBANs,
names, home addresses. A public expense shows title, amount, status and
discussion; the rest renders for members only. (Public is web-layer
rendering; the relay is members-scoped, so this is enforceable.)

Lifecycle: **pending → approved → paid**, plus declined/cancelled. Approval
is the channel's policy like any thread (ADR 0010); the Expenses channel
defaults to **2 steward approvals** — it's money. The thread shows the full
timeline: proposed, approved by whom, each payment, each confirmation.

## Payments: claim + confirmation

One rule for everyone, member or fiscal host:

1. **The payer signs a payment claim** — a structured comment (kind 1111 on
   the expense root) with machine tags: amount, currency, method used.
   "Any member can reimburse an expense" is exactly this. Members only.
2. **The author signs a reception confirmation** referencing the claim.
   Unconfirmed claims don't count and are flagged in the thread after
   7 days.
3. Partial payments are normal (several members can split an expense). The
   expense becomes **paid** when the author confirms settlement — the
   author's judgment, which also sidesteps FX math when an expense is
   reimbursed in a different currency than requested.

**Every confirmed payment by a member is a contribution to the collective**
by that payer, recorded on the contributors page.

## Fiscal hosts

A fiscal host is whoever accepts money *on behalf of* the collective and
keeps a balance for it — a nonprofit with a Stripe account for ticket
sales, an association receiving grants and wire transfers, **or an
individual member holding cash**. The role carries no organizational
prerequisite.

- **It joins like anyone.** An entity applies through the normal join flow
  and is approved like any member (its identity flagged as an organization,
  [identities.md](identities.md)); a person is already a member.
- The admin grants it the **fiscal host** role, which carries the
  `hold_funds` permission: its signed **ledger entries** are recognized by
  the treasury. Nothing else about it is special.

### The ledger

Three entry types, each an immutable event signed by the host's npub with
the community `a` tag (a dedicated application kind; exact number fixed at
implementation after a kind-registry scan):

| entry | content | effect |
|---|---|---|
| **credit** | amount, currency, **source**, optional earmark, memo, optional proof, optional running balance | balance up: money came in for the collective |
| **debit** | amount, currency, reference to the expense + payment claim, optional proof, optional running balance | balance down: the host paid an expense — confirmed by the author like any payment |
| **balance attestation** | "I hold X for this collective", per currency/earmark, optional proof of funds | a checkpoint. If it disagrees with the derived balance, **the discrepancy is displayed**, not hidden — that is the audit mechanism |

Balances are derived per host × currency × earmark from the log,
reconciled against attestations.

### Evidence: proofs and running balances

All optional — a host holding cash has none of these, and that's fine:

- **Proof on an entry or payment claim** — a typed reference to the
  external system that moved the money: a lightning receipt
  (invoice + preimage), an on-chain tx hash (chain + txid, linked to an
  explorer), a Stripe charge id, or another typed reference (the set is
  extensible like email providers). Proofs attach to ledger entries *and*
  to ordinary payment claims by members.
- **Running balance** — a credit or debit may state the post-entry balance,
  an inline mini-attestation. If it disagrees with the derived balance,
  the mismatch is displayed exactly like attestation discrepancies. Cheap
  consolidation: every entry can double as a checkpoint.
- **Proof of funds on an attestation** — for hosts whose balance lives on
  a chain: address(es) plus an ownership signature binding the address to
  the host's npub (BIP-322 for bitcoin, EIP-191 for EVM chains). Anyone
  can then check the chain against the attested amount; communityd
  verifies the ownership signature and links the explorer. Deliberately
  optional — the cash-holding individual and the Stripe-account nonprofit
  prove themselves through confirmations and consistency instead.

### Source attribution

Every credit names where the money came from:

- a third party — "€10,000 · source: Foundation Z · earmark: travel". The
  source is an **identity**: an existing member/follower npub, or an
  **unclaimed account** created on the spot
  ([identities.md](identities.md) § unclaimed accounts) — so the grant
  issuer is recognized as a contributing member with a real, portable
  identity, without ever being asked to create an account or confirm an
  email. If Foundation Z later wants in, it *claims* the account and
  inherits its whole contribution history;
- a genuine aggregate — "ticket sales (June meetup)" — may stay a
  plain-text source label, rendered as a non-identity contributor line
  (an identity would be fiction there);
- the host itself — then and only then it counts as the host's own
  donation.

The host is recognized regardless, as what it is: a contributing member
with the fiscal-host badge and held/processed stats — the intermediary,
not the donor.

### Earmarks

An earmark is a tag on credits ("travel", "grant-2026") matched by the
category on expenses. The treasury shows **amount left per earmark**.
Whether a given expense qualifies remains the host's human judgment when it
signs the debit — earmark enforcement is informational in v1, deliberately.

## Contributions and recognition

The contributors page aggregates, per identity and per currency:

- members' confirmed expense payments and direct donations
  (a donation = a credit with the member as source);
- external sources from host credits (grant issuers, ticket buyers);
- fiscal hosts (badge + held/processed; own donations only when
  self-sourced).

## The treasury

`/treasury` shows balances per host/currency/earmark, the recent ledger,
expenses by status, and contributors. **Public by default** — transparency
of money flows is the point of signing them — toggleable to members-only in
settings. Payout details and receipts are never public regardless.

## The two canonical walkthroughs

**Ticket sales.** Nonprofit A joins as a member, gets the fiscal-host role.
The collective sells tickets on lu.ma via A's Stripe. A periodically signs
credits ("€840 · source: ticket sales (June meetup)"). The treasury shows
€840 held by A; "ticket buyers (June meetup)" appears among contributors.
Event expenses get approved; A signs debits; authors confirm; the balance
runs down — every step signed and on the relay.

**A grant.** Nonprofit B receives €10,000 for the collective from
Foundation Z. B signs one credit with source "Foundation Z", earmark
"travel". Foundation Z appears as a contributor (external identity); B as
the hosting member. Travel expenses flow through approval; B reimburses the
ones the grant covers and signs debits with the earmark; authors confirm;
the treasury shows the amount left on the grant at all times.

## Deliberately out of scope (v1)

- Moving money (Stripe integration, lightning payouts): the record, not
  the rails. NIP-57 zaps / NIP-60 could later *automate claims*, not
  replace them.
- Cryptographic earmark enforcement.
- FX conversion: balances are per currency; settlement is the author's
  confirmation.
- Dispute arbitration: an unconfirmed or contested claim is visible and
  social, not adjudicated by the protocol.
