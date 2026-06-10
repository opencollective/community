# Use cases

End-to-end narratives that exercise the specs with real names, real
amounts and real sequences. Each step references the spec doc it relies on
and the test case IDs that pin the behavior down — if a narrative step has
no spec or case behind it, that's a gap to fix in the specs, not in the
story.

1. [Starting Commons Hub](01-starting-commons-hub.md) — from a bare VPS to
   a living collective: setup wizard, first members, channels, first
   announcement.
2. [Starting the fiscal host](02-starting-the-fiscal-host.md) — Citizen
   Spring ASBL becomes Commons Hub's fiscal host, and the question of
   whether a fiscal host should be a collective itself.
3. [A grant, an expense, a payout](03-grant-expense-payout.md) — Foundation
   for the Commons funds Commons Hub through Citizen Spring; a member files
   a €1,210 photographer expense (incl. €210 VAT); the host pays it; the
   ledger, the earmark and the contributor recognition all line up.

Conventions: people and entities here are illustrative; amounts are chosen
to exercise edge cases (VAT breakdown, earmark depletion). Writing use
case 3 already earned its keep: it surfaced the missing tax-breakdown
field on the expense template, fixed in the same commit.
