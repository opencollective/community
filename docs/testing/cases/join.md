# Test cases — join and membership approval

Flow reference: [flows/join.md](../../flows/join.md)

### JOIN-01 — applying creates a signed application
Given a visitor completes `/join` (name, free username, email, motivation) and verifies their email by code
Then an identity exists with status pending
And the application is signed by the applicant's new key (its first signature)
And the applicant receives an acknowledgment email

### JOIN-02 — taken usernames are caught live
Given username `alice` exists
When an applicant types `alice`
Then the form shows it is taken and suggests an available variant

### JOIN-03 — pending applications are members-only
Given a pending application
Then `/members/pending` requires a member login
And the application is invisible on every public page

### JOIN-04 — first steward approval shows progress, decides nothing
Given steward @alice approves a pending application
Then the application remains pending, showing "1 of 2"
And @alice cannot approve it a second time

### JOIN-05 — second distinct steward approval activates membership
Given approvals from stewards @alice and @bob
Then the applicant becomes an active member with the member role
And their kind 0 and kind 3 are published, NIP-05 resolves
And they are added to the relay member list and the #general group
And a decision email with a login link is sent

### JOIN-06 — admin approval decides alone
Given a pending application and no steward approvals
When the admin approves it
Then the applicant immediately becomes an active member (as in JOIN-05)

### JOIN-07 — members without the permission cannot approve
Given a plain member (no approve-members permission)
Then `/members/pending` shows applications without approve/decline actions
And a forged approval request is rejected

### JOIN-08 — decline closes the application with a reason
Given the admin (or two permission holders) declines with a reason
Then the applicant is emailed the outcome including the reason
And the identity remains, as non-member
And reapplying is blocked for 30 days, then possible again (fake clock)

### JOIN-09 — duplicate applications are prevented
Given a pending application for `marie@example.org`
When the same email applies again
Then no second application or identity is created
