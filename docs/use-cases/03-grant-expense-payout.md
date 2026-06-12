# Use case 3 — a grant, an expense, a payout

**Foundation for the Commons** awards **€5,000** to Commons Hub for its
events program. Foundations wire money to legal entities, so the grant
agreement names **Citizen Spring ASBL** — the Hub's fiscal host
([use case 2](02-starting-the-fiscal-host.md)) — as recipient.

## The grant arrives

1. **The wire lands** on Citizen Spring's bank account.
2. **An signs a credit** in the Hub's treasury:
   - amount `5000 EUR`, earmark `events`, memo "Grant agreement 2026-014";
   - **source: Foundation for the Commons** — An types the name, and an
     **unclaimed account** is created on the spot: a real npub for the
     foundation, badged unclaimed, editable only by An and the admin,
     claimable by the foundation whenever it cares to
     *(identities.md § unclaimed accounts; UNCL-01/02, MONEY-07)*;
   - proof: the bank transfer reference; running balance: `5000 EUR`.
     *(money.md § evidence; MONEY-14/15)*
3. **Recognition lands where it belongs.** The contributors page now
   shows **Foundation for the Commons: €5,000** (the donor) and
   **Citizen Spring ASBL** with the fiscal-host badge (the intermediary —
   not the donor). `/treasury` shows €5,000 held by Citizen Spring,
   earmarked *events*. *(MONEY-07/10/13)*

## The expense

4. **Leen files an expense** in the Expenses channel for the photographer
   who covered the opening event:
   - "Photography — opening event", amount **€1,210** including
     **€210 VAT (21%)** — the tax breakdown is a structured field, because
     Citizen Spring's accountant will need it;
   - category `events` (matching the earmark);
   - the photographer's invoice as receipt;
   - payout method: the photographer's IBAN.
   The thread is **pending**; the IBAN and the invoice render for members
   only, whatever the thread's visibility. *(money.md § expenses; MONEY-01)*
5. **Approval.** Expenses default to 2 steward approvals; Bob and Xavier
   sign (Leen, as author, couldn't count anyway). The expense is
   **approved** — visible on the treasury's expense list against the
   *events* earmark. *(MONEY-02; CHAN-15)*

## The payout

6. **An pays the invoice** from Citizen Spring's bank account, then signs
   a **debit**: `1210 EUR`, referencing the expense, proof = bank transfer
   reference, running balance `3790 EUR`. The debit is also a payment
   claim on the expense thread. *(money.md § payments, § the ledger; MONEY-08)*
7. **Leen confirms reception** — she checks with the photographer that the
   money arrived, then signs the confirmation and marks the expense
   settled. The expense is **paid**. *(MONEY-03/04)*
   > Variant, now specced: Leen could instead create a **managed account**
   > for the photographer and file the expense *as him* — author and
   > confirmation both his npub, every action stamped "via @leen"
   > ([identities.md](../nostr/identities.md) § managed accounts; MGD-02).
8. **Everything reconciles, publicly.** `/treasury` shows: Citizen Spring
   holding €3,790, all of it on the *events* earmark ("€3,790 left of
   €5,000"); the ledger reads credit €5,000 / debit €1,210 with matching
   running balances; the expense thread shows proposed → approved (Bob,
   Xavier) → paid (Citizen Spring) → confirmed (Leen). Every arrow is a
   signature, inspectable in `/log` down to the raw event JSON.
   *(MONEY-09/10/12; LOG-02/03)*

## What this bought everyone

- **The foundation** is publicly recognized as the donor — with an
  identity it can claim later, inheriting its giving history.
- **Citizen Spring** carries none of the bookkeeping ambiguity: its held
  balance, its earmark obligations and every movement are signed and
  visible — and its accountant has the VAT breakdown.
- **The Hub** got a photographer paid with two steward approvals and zero
  bank access of its own.
- **Anyone** — member, donor, future fiscal host — can audit the entire
  chain from grant to payout without trusting the server's database: the
  events are signed, and the database is just a cache of them.
