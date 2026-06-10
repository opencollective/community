# 0013 — expenses, contributions and fiscal hosts: signed claims, no money rails

Status: accepted (2026-06-10)

## Context

Collectives need to reimburse expenses, recognize contributors, and hold
balances via entities that can touch the traditional financial system
(Stripe, bank accounts, grants) — without the platform becoming a payment
processor. The trust questions — who asked, approved, paid, received,
holds — are exactly what signed events with role-gated quorums already
answer elsewhere in the system.

## Decision

([nostr/money.md](../nostr/money.md))

- **The platform records money, never moves it.** All money facts are
  signed events on the community relay.
- **Uniform payment rule**: payer (member or fiscal host) signs a claim;
  the expense author signs a reception confirmation; only confirmed
  payments count; partials allowed; *paid* = the author confirms
  settlement (avoiding FX adjudication). Confirmed member payments are
  contributions by the payer.
- **Fiscal hosts are members with one extra permission.** Entities join
  through the normal application flow (identity flagged organization); a
  default `fiscal host` role carries the new `hold_funds` permission,
  which makes the holder's **ledger entries** treasury-recognized: credits
  (with mandatory source attribution and optional earmark), debits
  (payments against expenses, author-confirmed), and balance attestations
  as reconciliation checkpoints whose mismatches are displayed, not hidden.
- **Source attribution is the recognition mechanism**: grant issuers and
  ticket buyers named in credits appear as contributors (lightweight
  external identities); the host is recognized as the contributing
  intermediary; host money counts as its donation only when self-sourced.
- **Earmarks are informational in v1** — "amount left per earmark" is
  derived and displayed; qualification stays the host's judgment.
- **Privacy floor**: payout method details and receipts are members-only
  regardless of thread visibility. **Treasury defaults to public**
  (toggleable) — transparency of flows is the point of signing them.
- Expenses-channel approval default: 2 stewards.

## Consequences

- Zero payment-rails liability and no custody, while the audit trail is
  stronger than most accounting software: every step attributable,
  portable, rebuildable from the relay, and visible in /log.
- One new application event kind (ledger entry) to be registered at
  implementation; payment claims/confirmations reuse kind 1111 with
  machine tags so threads render as readable conversations.
- Honest limits, documented: disputes are social (visible unconfirmed
  claims), not adjudicated; a malicious host can sign false attestations —
  but falsity is then permanently signed, and the displayed
  derived-vs-attested mismatch is the alarm.
- The fractal model composes: a subgroup's treasury is its own; a parent
  community can act as its subgroup's fiscal host by being a member-entity
  of it.
- Future path (separate ADRs): zap/NIP-60 integration to automate claim
  creation; bring-your-own-key entities; cryptographic earmarks.
