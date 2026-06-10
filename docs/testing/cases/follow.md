# Test cases — follow

Flow reference: [flows/follow.md](../../flows/follow.md)

### FOLLOW-01 — following with an email creates a unique pending identity
Given a visitor submits `marie@example.org` at `/follow`
Then an identity with username `marie` is created with an encrypted nsec
And nothing is published to the relay yet
And a confirmation email is sent to `marie@example.org`

### FOLLOW-02 — username collisions are resolved automatically
Given identities `marie` and `marie1` already exist
When `marie@other.org` follows
Then the new identity gets username `marie2`

### FOLLOW-03 — confirmation publishes the follow
Given a pending follower clicks the confirmation link
Then their kind 0 profile and kind 3 (following the community npub) appear on the relay
And they hold the follower role with newsletter opt-in recorded
And the page confirms "you're following as @marie"

### FOLLOW-04 — unconfirmed follows expire
Given a follower who never confirms
When 30 days pass (fake clock)
Then the pending identity and its key material are deleted

### FOLLOW-05 — refollowing an existing email does not disclose it
Given `marie@example.org` already follows
When the same address is submitted again
Then the page response is identical to a first-time follow
And an email is sent saying they already follow (no new identity)

### FOLLOW-06 — a follower can unsubscribe without logging in
Given a confirmed follower with newsletter opt-in
When they use the unsubscribe link from any newsletter
Then the opt-in flag is cleared without authentication
And their kind 3 follow remains on the relay

### FOLLOW-07 — a follower who joins keeps their identity
Given a confirmed follower `marie`
When she applies to join with the same email and is approved
Then the same npub is upgraded to member (no new key)
And her username and follow history are preserved
