# Test cases — typed channels (Proposals, Requests)

Flow reference: [nostr/channels.md](../../nostr/channels.md)

### CHAN-01 — defaults after setup
Given a completed setup
Then #general (chat) and Proposals (threads) are enabled, Requests exists disabled
And `/settings/community` shows toggles for Proposals and Requests
And #general has no off switch

### CHAN-02 — toggling a channel off and on
Given the admin disables Proposals
Then its tab disappears and new threads/replies are rejected
When the admin re-enables it
Then the tab returns with all prior threads intact (history lives on the relay)

### CHAN-03 — proposals are members-only
Given an enabled Proposals channel with threads
Then visitors, followers and external identities cannot see the tab content
And a non-member relay subscription receives no Proposals events

### CHAN-04 — a member starts a proposal thread
Given member @dan submits a new proposal (title, body)
Then a kind 11 event signed by dan's key with the channel's `h` tag exists on the relay
And the thread appears at the top of the Proposals list

### CHAN-05 — replies thread under the root
Given dan's proposal thread
When @alice replies
Then a kind 1111 event signed by alice references the root
And the thread view shows the reply; the list shows an updated reply count

### CHAN-06 — emoji reactions toggle and group
Given a thread root and a reply
When @alice reacts with one emoji and @bob reacts with the same emoji
Then kind 7 events signed by each exist and the UI shows that emoji with count 2
When @alice reacts with the same emoji again
Then her reaction is deleted (count 1) — one active reaction per identity, emoji and target

### CHAN-07 — requests stay closed until enabled
Given a fresh setup (Requests disabled)
Then `/channels/requests` is not linked and returns not-found
And no external request form is reachable

### CHAN-08 — an external posts a request
(updated by ADR 0010: requests start pending)
Given Requests is enabled
When a visitor submits name, email and request text, then enters the emailed 6-digit code
Then an external identity is created with an encrypted key
And a kind 11 event signed by that identity exists in the Requests group
And the thread is **pending**: visible to members and (via its link) the author
And it becomes visible to logged-out visitors only once approved (CHAN-15)

### CHAN-09 — external identities are scoped
Given an external identity from CHAN-08
Then it can reply and react in its own channel
And it cannot read or post in #general or Proposals, nor access `/members`
And its relay access is limited to the Requests group

### CHAN-10 — externals are notified of replies
Given an external's request thread
When member @alice replies
Then the external receives an email with the reply and a link to the thread
And subsequent replies within a short window are batched, not one email each

### CHAN-11 — unverified external requests are never posted
Given a visitor who submits a request but never enters the code
Then nothing reaches the relay and no thread renders
And the pending submission expires like an unconfirmed follow

### CHAN-12 — moderation works across channels
Given moderator @carol
When she removes a request thread's root
Then the whole thread disappears from the list and view
And a member muted by a moderator cannot post threads or replies in any channel

### CHAN-13 — template validation is server-side
Given a thread-creation request whose payload violates the channel template
(missing required field, unverified external author, oversized content)
Then the bunker signs nothing and the relay receives nothing
And the form shows the specific violation

### CHAN-14 — thread lists are rebuildable from the relay
Given channels with threads, replies and reactions (including their statuses)
When the `threads_index` cache is wiped and communityd restarts
Then channel lists, counts, statuses and thread views render identically

### CHAN-15 — default approval policy: one steward, not the author
Given steward @alice starts a thread in any thread channel
Then it is pending, and alice's own approval does not count
When steward @bob signs a kind 4550 approval referencing the root
Then the thread is approved
And the admin approving alone (no steward) also approves any pending thread

### CHAN-16 — approval policy is configurable per channel
Given the admin sets Requests to approvers {steward, moderator} with 2 required
Then one moderator approval leaves a request pending ("1 of 2")
And a second approval from a *distinct* steward or moderator approves it
And a plain member's approval attempt is rejected
And the Proposals channel policy is unaffected

### CHAN-17 — status filter
Given a channel with pending and approved threads
Then the list defaults to all, labeled by status
And the pending filter shows only pending, the approved filter only approved

### CHAN-18 — declining a thread
Given a pending thread
When an approver declines it with a reason (kind 1985)
Then it leaves the pending list, the author is notified (email for externals)
And the author may revise and resubmit (new root, approvals reset)

### CHAN-19 — channel visibility defaults and override lock
Given the Events channel (default public, override allowed)
Then the new-thread form offers a visibility choice defaulting to public
And a member can create a members-only event
Given the Proposals channel (default members, override locked)
Then the form offers no visibility choice
And a forged request with a `visibility: public` tag is rejected at submission

### CHAN-20 — visibility gates rendering after approval
Given an approved public thread and an approved members-only thread in the same channel
Then a visitor sees only the public one (list and direct URL)
And a member sees both
And a pending public thread is invisible to visitors until approved
When an approver flips the public thread to members-only (label)
Then visitors lose access on next render, and the flip appears in /log
