# 0014 — unclaimed accounts, and evidence on the ledger

Status: accepted (2026-06-10)

## Context

Two gaps in ADR 0013. First, recognition: naming a grant issuer as a text
string makes the *intermediary* the only real identity — but asking a
foundation to sign up and verify an email just to be thanked is a
non-starter. Second, evidence: ledger entries asserted amounts with no
hook for the receipts the external systems already produce (lightning
preimages, tx hashes, Stripe charge ids), no cheap per-entry
consolidation, and no way for a chain-based host to prove funds. Also,
ADR 0013 over-restricted fiscal hosts to organizations — an individual
holding cash for the collective is a legitimate host.

## Decision

- **Unclaimed accounts** ([identities.md](../nostr/identities.md)): full
  identities created for someone by the admin or implied by a member's
  action (a host attributing a credit). Editable and hand-over-able
  (email set/replaced) only by creator + admins until **claimed** via the
  standard email-code flow, at which point control transfers entirely and
  the npub, NIP-05 and history continue. No login, no signing while
  unclaimed; always visibly marked. Credit sources are identities by
  default (existing or unclaimed); plain-text labels remain for genuine
  aggregates ("ticket buyers").
- **Ledger evidence** ([money.md](../nostr/money.md) § evidence), all
  optional: typed **proofs** on ledger entries and payment claims
  (lightning receipt, tx hash, Stripe charge id; extensible set);
  **running balance** on credits/debits as inline mini-attestations
  reconciled like full attestations; **proof of funds** on attestations
  (chain address + BIP-322/EIP-191 ownership signature, verified and
  explorer-linked).
- **Fiscal hosts can be individuals** — the organization flag is
  rendering, not a prerequisite.

## Consequences

- The recognition graph becomes real: a contributor page entry is an npub
  someone can claim years later and walk away with — consistent with
  "members own their identity".
- Unclaimed accounts are a new lifecycle state (unclaimed → claimed)
  alongside pending/active, with an edit-rights inversion at claim time —
  the cases pin this down (UNCL-*).
- A typed-proof registry mirrors the email-provider pattern: adding
  "wise transfer id" later is one entry, not a redesign.
- Evidence is optional by design — the cash host remains first-class; the
  system's floor is still confirmations + consistency + visibility, and
  proofs only raise the ceiling.
- Abuse surface noted: creating identities for real-world entities has
  defamation/squatting potential; mitigations are the visible "unclaimed"
  marker, creator+admin-only control, and admin takeover/handover.
