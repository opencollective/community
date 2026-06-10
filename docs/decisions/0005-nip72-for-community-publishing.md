# 0005 — NIP-72 proposals and approvals for posting as the community

Status: accepted (2026-06-10)

## Context

Posting as the community npub requires approval (admin, or two stewards —
configurable). The naive design — unsigned drafts and approval rows in the
app database — works but loses exactly the data worth keeping: *who*
proposed and *who* approved, attributably and durably. The question raised
in design review: is there a Nostr-native mechanism?

## Decision

Use NIP-72 (moderated communities) on our own relay
([nostr/publishing.md](../nostr/publishing.md)):

- the community publishes a kind 34550 definition; stewards are its
  moderators;
- a proposal is the draft event (kind 1 / 30023) signed by the **proposer's
  own key** with the community `a` tag;
- each approval is a kind 4550 signed by a steward, embedding the exact
  proposal;
- declines are NIP-32 kind 1985 labels;
- on quorum, the bunker signs the final event with the community key,
  crediting author and proposal.

The app database keeps only a rebuildable rendering index.

## Consequences

- The audit trail is cryptographically attributable, survives the app
  database, and is readable by NIP-72-aware clients.
- Approvals bind to exact content: any edit produces a new event id and
  naturally voids prior approvals.
- Pending proposals are visible to all members (the relay is members-only) —
  embraced as transparency; private drafts would require NIP-59 and are
  deferred.
- Quorum enforcement stays inside the bunker; the relay events are evidence,
  the signer is the gate.
- Sets a pattern we can extend to membership applications later.
