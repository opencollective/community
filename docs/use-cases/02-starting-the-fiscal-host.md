# Use case 2 — starting the fiscal host

Commons Hub is not a legal entity. It can't open a Stripe account, issue
invoices, or receive a grant by wire. **Citizen Spring ASBL** — a Belgian
nonprofit close to the community — agrees to hold money on the Hub's
behalf.

## Should a fiscal host be a collective itself?

It *may* be — and Commons Hub doesn't need to care. From the Hub's
perspective a fiscal host is exactly one thing: **a member npub holding
the `fiscal host` role**, whose signed ledger entries the treasury
recognizes ([nostr/money.md](../nostr/money.md)). What stands behind that
npub is the host's own business:

- **Simple form (start here):** Citizen Spring joins as an organization
  member account, operated by its treasurer An — either she logs in with
  her email, or, cleaner, she is a member herself **managing** the entity
  account, so every ledger entry it signs is stamped "via @an"
  ([identities.md](../nostr/identities.md) § managed accounts).
- **Full form (graduate when needed):** Citizen Spring runs a collective
  of its own — its own Community (a tenant on the same server, or its own
  box), with its own members, roles, expense flow and activity log. Useful
  the day it hosts *several* collectives or wants internal sign-off before
  money moves. In the fractal model this is the same composition as
  everything else: a collective being a member of a collective
  ([architecture/multi-tenancy.md](../architecture/multi-tenancy.md)).

The recommendation mirrors subgroup graduation: start simple, fork into a
full collective when the simple form starts to hurt. The npub in Commons
Hub's member list — and its signed ledger history — stays meaningful
either way.

> v1 note: in both forms the identity that joins Commons Hub is held by
> Commons Hub's bunker and operated through login. A host bringing its own
> externally-held key is a future ADR
> ([0013](../decisions/0013-expenses-contributions-fiscal-hosts.md) notes it).

## The walkthrough (simple form)

1. **Apply.** An fills `/join` as "Citizen Spring ASBL", username
   `citizenspring`, her email, motivation: "fiscal host for the Hub —
   Stripe, invoices, grants." *(flows/join.md; JOIN-01)*
2. **Approve.** Stewards Leen and Bob approve; the identity is flagged as
   an **organization**. `citizenspring@commonshub.brussels` resolves;
   the entity appears in the members directory. *(JOIN-05; identities.md § organizations)*
3. **Grant the role.** Xavier assigns the **fiscal host** role — the
   `hold_funds` permission. From this moment Citizen Spring's ledger
   entries are treasury-recognized; nothing else about the account is
   special. *(flows/roles.md; MONEY-06)*
4. **First attestation.** An signs a balance attestation: "€0 held for
   Commons Hub." An empty, signed starting point the whole community can
   see at `/treasury`. *(money.md § the ledger; MONEY-09)*

Citizen Spring is now equipped to receive money for the Hub — which is
exactly what happens next: [use case 3](03-grant-expense-payout.md).
