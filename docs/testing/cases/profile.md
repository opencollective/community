# Test cases — community profile edits (linktree)

Flow reference: [nostr/publishing.md](../../nostr/publishing.md) § profile edits

### PROF-01 — any member proposes; the wrapper protects their own profile
Given plain member @dan (no propose-posts permission) submits a profile edit
Then a kind 30078 wrapper event signed by dan's key exists on the relay,
with the proposed profile JSON, a `k:0` tag and the community `a` tag
And **dan's own kind 0 profile is unchanged** on the relay
And the request appears in the pending queue

### PROF-02 — the pending queue shows a field-level diff
Given dan's edit changes the description and adds one link
Then the pending card shows old vs new description, the added link,
and "name and icon unchanged"

### PROF-03 — quorum publishes the new community profile
Given approvals from stewards @alice and @bob (kind 4550 referencing the wrapper)
Then the bunker signs a new community kind 0 containing the approved name,
about, picture and links array
And the homepage hero and linktree render the new values for visitors
And **no newsletter is sent**

### PROF-04 — admin approval decides alone
Given a pending profile edit with no approvals
When the admin approves it
Then the new kind 0 is published immediately

### PROF-05 — fields and links are validated server-side
Given an edit containing a non-whitelisted field, a `javascript:` link URL,
or more links than the cap
Then the submission is rejected with the specific violation and nothing
reaches the relay
And link URLs must be http(s)

### PROF-06 — editing a pending request resets approvals
Given dan's edit has one approval
When dan revises it (new wrapper event id)
Then prior approvals do not count toward the revised request

### PROF-07 — declines work as for posts
Given @alice declines dan's edit with a reason
Then a kind 1985 label signed by alice references the wrapper
And dan may revise and resubmit

### PROF-08 — stale proposals are flagged
Given two pending profile edits based on the same current profile
When one of them is approved and published
Then the other is marked stale ("the profile changed since this was proposed")
and shows its diff against the *new* current profile
