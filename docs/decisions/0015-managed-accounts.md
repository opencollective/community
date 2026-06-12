# 0015 — managed accounts: bunker-stamped provenance, effective-actor quorums

Status: accepted (2026-06-10)

## Context

People need accounts operated for them: a member without a smartphone, a
vendor who should author and confirm their own expense, a donor, an entity
operated by its treasurer. Requirements: anyone can create such an
account; the record of who created it and **every action taken on its
behalf** must be kept, attributably. Candidate mechanisms: shared
credentials (no attribution, revocation nightmare), NIP-26 delegation
(deprecated, poor client support, wrong direction — it delegates *from*
the key owner, who here doesn't exist yet), or something native to our
architecture.

## Decision

([nostr/identities.md](../nostr/identities.md) § managed accounts)

- **Management is a relationship** (`account_managers`: managed identity,
  manager, granted-by, since), orthogonal to the claimed/unclaimed state.
  Creating an account on behalf of someone makes the creator its first
  manager; the grant itself is logged.
- **Acting-as goes through the bunker**: a manager switches into the
  account from their own session; the bunker stamps a `managed-by` tag
  (manager's pubkey) into every event **before signing with the managed
  key**, and refuses to sign for managed accounts through any
  non-manager session. Provenance is therefore inside the signed event —
  permanent, portable, database-independent — not in an app table.
- **Effective-actor quorum rule**: for any approval quorum, an identity
  counts only if neither it, nor its acting manager, nor another identity
  that manager acted through, already appears in the quorum as author or
  approver. One human, one vote.
- **Manager boundaries**: no claiming, no claim-email changes, no
  manager management, no external bunker URLs for the managed key.
- **On claim**, management grants pause pending the new owner's explicit
  re-confirmation.
- `hold_funds` requires the account to be **operable** — claimed or
  managed (supersedes UNCL-05's blanket refusal for unclaimed accounts).

## Consequences

- Resolves the use-case-3 gap: a vendor can be the *author* of their
  expense (filed and confirmed "via" the managing member) with payout to
  their own IBAN — instead of the filing member vouching informally.
- Fiscal-host hygiene improves: entity accounts managed by treasurer
  members attribute every ledger entry to a human, while the entity npub
  keeps the clean protocol history.
- The `managed-by` tag's truthfulness rests on the bunker, like every
  signature in the system — stated plainly. External verifiers trust the
  community's signer, not the manager's honesty.
- Rendering debt: every surface showing authorship (threads, chat,
  ledger, /log) must show the "via @manager" chip; manager lists on
  profiles are members-only, the managed badge is public.
- The effective-actor rule adds a join to every quorum computation —
  cheap, but it must be implemented *everywhere* quorums are counted
  (posts, profile edits, channel approvals, member applications).
