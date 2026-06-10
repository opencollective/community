# Test cases — #general chat

Flow reference: [nostr/chat.md](../../nostr/chat.md)

### CHAT-01 — a member's message is a signed group event
Given logged-in member @dan sends "hello" in #general
Then a kind 9 event signed by *dan's key* with the group's `h` tag exists on the relay
And it renders in the channel with dan's name and badges

### CHAT-02 — chat is invisible to outsiders
Given a visitor (not logged in) and a follower (not a member)
Then the homepage shows them the locked #general teaser, no messages
And an unauthenticated relay subscription receives no group events (NIP-42)

### CHAT-03 — history survives communityd restarts
Given messages were posted
When communityd restarts
Then the channel renders the same history (the relay is the storage)

### CHAT-04 — a moderator removes a message
Given @carol holds moderate-chat and @dan posted a message
When @carol removes it
Then the removal is a moderation event + deletion on the relay, signed by carol's key
And the message disappears from the rendered channel
And @dan (no permission) cannot remove @carol's messages

### CHAT-05 — a muted member cannot post
Given @dan was muted by a moderator
When he submits a message
Then it is rejected with an explanatory notice and nothing reaches the relay

### CHAT-06 — an external NIP-29 client sees the same channel
Given @dan connects a go-nostr test client through his bunker session
Then he can read the same history and post a message
And the message appears in the web channel
(covers interop promised in [chat.md](../../nostr/chat.md))

### CHAT-07 — new members gain access automatically
Given a freshly approved member (JOIN-05)
Then they can read and post in #general without further steps
