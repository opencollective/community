# Test cases — unclaimed accounts

Flow reference: [nostr/identities.md](../../nostr/identities.md) § unclaimed accounts

### UNCL-01 — creation by attribution
Given fiscal host A signs a credit with a new source "Foundation Z"
Then an unclaimed identity exists for Foundation Z (npub, encrypted key, organization flag)
And it appears on the contributors page marked unclaimed
And its NIP-05 resolves

### UNCL-02 — only the creator and admins control it
Given the unclaimed Foundation Z account created by host A
Then A and the admin can edit its profile and set/replace its contact email
And other members (including stewards) cannot
And nobody can log in as it or make it sign anything

### UNCL-03 — claiming transfers control
Given the admin binds foundation-z@example.org to the account
When the recipient verifies the 6-digit code
Then the account is claimed: they hold it like any member account
And the creator and admins lose their edit and handover rights
And the npub, NIP-05 and contribution history are unchanged

### UNCL-04 — handover before claim
Given an unbound unclaimed account
When the creator sets one email, then replaces it with another before any claim
Then only the latest email's code can claim it
And codes sent to the replaced address are invalid

### UNCL-05 — unclaimed accounts and roles
(updated by ADR 0015: hold_funds requires operable — claimed or managed)
Given an unclaimed account granted the member role by the admin
Then it appears in the members directory with the unclaimed badge
And it gains no session of its own, and no capability until claimed or managed
And granting it hold_funds is refused while it has neither owner nor manager (MGD-07)
