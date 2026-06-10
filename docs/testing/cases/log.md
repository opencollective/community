# Test cases — admin activity log

Flow reference: [decisions/0011](../../decisions/0011-content-governance-rss-log.md).
The log renders the relay; it has no storage of its own.

### LOG-01 — the log is admin-only
Given the admin, a steward and a member
Then `/log` renders for the admin
And stewards and members get the same not-found as any unauthorized page

### LOG-02 — entries are human-readable renderings of signed events
Given a session of activity (a join approved, an event approved, an
announcement published, a chat message removed, a profile edit declined)
Then `/log` shows each as a plain-English line with actor, action, object
and relative time ("@alice approved @dan's event 'Community call' · 2h")
And an event of an unrecognized kind renders as a generic line, not hidden

### LOG-03 — the JSON inspector shows the exact event
Given any log entry
When the admin expands it
Then the raw Nostr event JSON is shown (id, kind, pubkey, tags, content, sig)
And the id and signature verify against the displayed content

### LOG-04 — filters and pagination
Given hundreds of events
Then the log paginates
And filtering by category (publishing / members / channels / profile) and
by author narrows the list correctly

### LOG-05 — the log survives the app database
Given a populated log
When rebuildable caches are wiped and communityd restarts
Then `/log` renders the same entries, reconstructed from the relay
