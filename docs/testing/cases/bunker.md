# Test cases — bunker and external apps

Flow reference: [architecture/bunker.md](../../architecture/bunker.md).
External clients are real go-nostr NIP-46 clients in tests.

### BUNKER-01 — a member generates a bunker URL
Given logged-in member @alice on `/settings/apps`
Then she receives `bunker://<her-bunker-pubkey>?relay=wss://<domain>/relay&secret=…`
And the secret is bound to her identity with a 10-minute expiry

### BUNKER-02 — connect consumes the secret and opens a session
Given an external client connects with a valid secret
Then a session is created and listed on `/settings/apps`
And the same secret cannot connect a second client
And an expired secret (fake clock +11 min) is rejected

### BUNKER-03 — a connected app signs through the bunker
Given a connected external client
When it requests `get_public_key` and `sign_event` (a kind 1 note)
Then it receives alice's npub and a validly signed event
And the signature verifies against her published profile key

### BUNKER-04 — no API ever returns a private key
Given any actor (admin, member, connected app)
Then no HTTP response and no NIP-46 response in any flow contains an nsec or raw key bytes
(asserted by a response-scanning middleware enabled in the test harness)

### BUNKER-05 — revocation stops signing immediately
Given a connected app
When @alice revokes its session on `/settings/apps`
Then the app's next request is refused
And other sessions of hers are unaffected

### BUNKER-06 — sessions are isolated per identity
Given a session opened against @alice's bunker pubkey
Then it cannot sign as @bob (distinct bunker keypair per identity)

### BUNKER-07 — sessions survive restarts
Given a connected app
When communityd restarts (auto-unlock mode)
Then the app keeps signing without reconnecting

### BUNKER-08 — encryption methods work for both generations
Given a connected app using NIP-44 (and another using legacy NIP-04)
Then `nip44_encrypt/decrypt` and `nip04_encrypt/decrypt` round-trip correctly
