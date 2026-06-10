# Test cases — publishing as the community

Flow reference: [nostr/publishing.md](../../nostr/publishing.md).
Default policy throughout: "admin alone, or 2 distinct stewards".

### PUB-01 — composing requires the propose permission
Given a plain member without propose-posts
Then `/compose` is not offered and a forged proposal request is rejected
Given steward @alice
Then `/compose` offers announcement and blog post

### PUB-02 — a proposed announcement is a signed Nostr event
Given @alice submits an announcement draft
Then a kind 1 event signed by *alice's key* exists on the relay
And it carries the community `a` tag (NIP-72)
And it appears in `/posts/pending` as "proposed by @alice", visible to all members
And nothing appears on the public homepage

### PUB-03 — an approval is a signed kind 4550
Given @bob (approve-posts) approves alice's proposal
Then a kind 4550 event signed by bob's key exists on the relay, embedding the exact proposal
And `/posts/pending` shows "approved by @bob · 1 of 2"

### PUB-04 — quorum publishes the announcement as the community
Given approvals from stewards @bob and @carol
Then the bunker signs a kind 1 event with the *community key*
And it credits @alice (`p` tag) and references the proposal (`e` tag)
And the announcement appears on the public homepage
And the proposal leaves `/posts/pending`

### PUB-05 — the admin publishes alone
Given a pending proposal with no approvals
When the admin approves it
Then it is published immediately (as in PUB-04)

### PUB-06 — the proposer's own approval never counts
Given @alice approves her own proposal
Then quorum remains "0 of 2" and nothing is published

### PUB-07 — editing a proposal resets approvals
Given alice's proposal has one approval from @bob
When @alice edits the draft (new event id)
Then the pending entry shows the new content with "0 of 2"
And bob's old kind 4550 does not count toward the new event

### PUB-08 — a decline is a signed label
Given @bob declines alice's proposal with a reason
Then a kind 1985 `declined` label signed by bob's key references the proposal
And the proposal shows as declined with the reason
And @alice may revise and resubmit (fresh proposal, fresh approvals)

### PUB-09 — a blog post reaches the blog and the RSS feed, not inboxes
(updated by ADR 0011)
Given a kind 30023 blog-post proposal by @alice, approved per the blog policy
Then the community publishes its own kind 30023 (community-owned d-tag, credit tags)
And the post renders at `/posts/{slug}` and on the homepage blog section
And it appears in `/feed.xml`
And **no email is sent** (MAIL-04)

### PUB-10 — losing the role between approval and quorum invalidates the approval
Given @bob approved, then lost the steward role
When @carol's approval would complete quorum
Then bob's approval no longer counts and the proposal stays pending ("1 of 2")

### PUB-11 — each section's policy is independent and configurable
(updated by ADR 0011: per-section roles + count replace the single enum)
Given announcements set to {steward} × 1 and newsletter left at its default {steward} × 2
Then one steward approval publishes an announcement
And one steward approval leaves a newsletter pending ("1 of 2")
Given a section's approver roles set to none
Then only the admin can publish it

### PUB-13 — a newsletter is emailed and archived
Given a newsletter proposal approved per the newsletter policy (2 stewards by default)
Then the community publishes a kind 30023 carrying the `newsletter` self-label
And it renders in the site archive
And the email send is triggered (MAIL-01)
And it does **not** appear in the blog's `/feed.xml`

### PUB-15 — announcements carry visibility
Given a published members-only announcement and a published public one
Then visitors see only the public one on the homepage
And members see both
And the relay events carry the `visibility` tag chosen at compose

### PUB-14 — the RSS feed is valid and public
Given published blog posts and pending proposals
When `/feed.xml` is fetched without authentication
Then it is valid RSS listing the published posts (title, link, date)
And contains nothing pending and no newsletters

### PUB-12 — the trail is reconstructable from the relay alone
Given a published announcement (PUB-04)
When the `proposals_index` table is wiped and communityd restarts
Then `/posts/pending` and the published post's provenance render identically,
rebuilt from relay events
