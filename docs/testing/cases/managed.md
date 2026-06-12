# Test cases — managed accounts

Flow reference: [nostr/identities.md](../../nostr/identities.md) § managed accounts

### MGD-01 — creating on behalf records the creator as manager
Given member @marie creates an account for "Jan the photographer"
Then the identity exists with @marie as manager, the grant logged with grantor and time
And the account shows the managed badge everywhere; manager names render members-only

### MGD-02 — acting-as stamps provenance into the signed event
Given @marie switches to acting as @jan
When she files an expense and later confirms reception
Then both events are signed by jan's key and carry a `managed-by` tag with marie's pubkey
And the thread and /log render "via @marie" on each action
And after wiping rebuildable indexes, the attribution renders identically (it lives in the events)

### MGD-03 — the bunker refuses non-managers
Given @bob is not a manager of @jan
Then any attempt by bob's session to act as jan signs nothing and is rejected
And there is no path to a signature for a managed account except through a manager's session

### MGD-04 — one human, one vote
Given @marie manages steward account @org, and @marie is herself a steward
When marie approves a pending post as herself and then approves it again acting as @org
Then only one approval counts toward the quorum
And the second is rejected with an explanation, not silently discarded
And an author's manager approving the author's own thread does not count

### MGD-05 — manager boundaries
Given @marie manages @jan
Then she cannot claim it, set or change its claim email, add or remove managers,
or generate a bunker URL for jan's key
And the admin (or the owner, once claimed) can do the first three

### MGD-06 — claiming pauses management
Given @jan claims his account via email code
Then marie's management is paused: acting-as is refused
When jan re-confirms marie as manager, acting-as works again
And had he declined (or done nothing), the grant would lapse

### MGD-07 — managed entities can hold funds
(updates UNCL-05 per ADR 0015: operable = claimed or managed)
Given unclaimed entity @citizenspring managed by member @an
Then granting it the fiscal host role succeeds
And ledger entries an signs acting as citizenspring carry her `managed-by` tag
And an unclaimed AND unmanaged account is still refused hold_funds
