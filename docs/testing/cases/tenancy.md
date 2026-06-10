# Test cases — multi-community servers, subgroups, graduation

Flow reference: [architecture/multi-tenancy.md](../../architecture/multi-tenancy.md).
v1 ships single-community, but the conventions these cases pin down apply
from the first PR; the harness provisions extra communities directly
(provisioning UI is a later milestone).

### TEN-01 — host resolution dispatches communities
Given communities `alpha.test` and `beta.test` on one server
Then each hostname serves its own homepage, theme colors and name
And `/.well-known/nostr.json` on each host resolves only that community's identities
And an unknown Host gets a neutral not-found, leaking nothing

### TEN-02 — communities are isolated namespaces
Given username `marie` exists in both communities (distinct people, distinct keys)
Then each NIP-05 resolves to its own npub
And a member of alpha, logged in on alpha, gets no access to beta's member pages
And alpha's relay subscription never returns beta's events (separate virtual relays)

### TEN-03 — encryption is per community
Given both communities are populated
When alpha's admin rotates alpha's master password
Then beta's password and signing are unaffected
And with alpha in strict mode after a restart, alpha is locked while beta keeps signing

### TEN-04 — one database per community, no cross-references
Given any populated server
Then each community directory's `app.db` contains that community's rows only
And `server.db` contains registry rows and no community content

### TEN-05 — a subgroup is a full community
Given subgroup `circle.alpha.test` created under alpha
Then it has its own channels (#general, Proposals), roles, settings and database
And its community npub is enrolled as a member of alpha
And the registry records alpha as its parent
And a person in both keeps one keypair, enrolled in each database

### TEN-06 — graduation round-trip
Given subgroup `circle` with members, threads, posts and media
When `community export circle` is imported on a fresh server and DNS is repointed (test: new harness instance)
Then members log in and sign on the new server (admin unlocks with the same master password)
And threads, posts, media and the approval trail render identically
And NIP-05 resolves from the new server

### TEN-07 — adding a community disturbs nothing
Given a populated single-community server
When a second community is provisioned
Then the first community's pages, relay and sessions behave identically before and after
