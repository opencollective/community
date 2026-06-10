# Testing

The files in [cases/](cases) are the behavioral contract of the platform,
written in plain English. Code implements the cases; tests cite them.

## How cases are written

Every case has a **stable ID** (`SETUP-03`, `PUB-04`, …) and Given/When/Then
structure:

```
### PUB-04 — two steward approvals publish the announcement
Given a pending announcement proposed by @alice
And kind 4550 approvals signed by stewards @bob and @carol
Then the bunker signs a kind 1 event with the community key
And the event credits @alice and references the proposal
And the announcement appears on the homepage
```

Rules:
- IDs are never reused or renumbered. A removed case keeps its ID with a
  `(removed: reason)` note.
- Cases describe **observable behavior** (HTTP responses, relay events,
  emails captured by the fake mailer, database effects) — never internals.
- New behavior gets its case *before* the implementing PR; a behavior change
  updates the case in the same PR that changes the code.

## How tests cite cases

Go tests reference IDs in their names: `TestPUB04_QuorumPublishesAnnouncement`.
One case may be covered by several tests (unit + integration); every case
must be covered by at least one, enforced by `make test-coverage-cases`,
which greps test names against the case index and fails on uncovered or
unknown IDs.

## Test layers

| layer | what runs | what's real | what's fake |
|---|---|---|---|
| unit | `go test ./...` | one package | everything external |
| integration | `go test -tags integration ./tests/...` | communityd (full HTTP stack via httptest) **and a real zooid** on ephemeral ports, real SQLite, real go-nostr clients over websocket | mailer (in-memory capture), ACME (test mode), clock (injectable) |
| smoke | CI release pipeline | both release binaries booted under systemd-run in a container | DNS/ACME |

Server-rendered HTML + htmx means integration tests exercise real pages with
plain HTTP assertions (status, HTML fragments, headers) — no browser driver
is required for the contract. A Playwright layer may be added later for
JS-dependent surfaces (chat websocket, code inputs); it would cite the same
case IDs.

## Case files

| file | prefix | flow |
|---|---|---|
| [cases/setup.md](cases/setup.md) | SETUP | the six-step wizard |
| [cases/follow.md](cases/follow.md) | FOLLOW | following, double opt-in |
| [cases/join.md](cases/join.md) | JOIN | applications and membership approval |
| [cases/login.md](cases/login.md) | LOGIN | email-code login, web sessions |
| [cases/chat.md](cases/chat.md) | CHAT | #general messages and moderation |
| [cases/channels.md](cases/channels.md) | CHAN | typed channels: Proposals, Requests, threads, reactions |
| [cases/publishing.md](cases/publishing.md) | PUB | announcements and blog posts: propose, approve, publish |
| [cases/newsletter.md](cases/newsletter.md) | MAIL | email delivery: text + HTML, unsubscribe, at-most-once |
| [cases/profile.md](cases/profile.md) | PROF | community profile edits: linktree, diffs, approval |
| [cases/roles.md](cases/roles.md) | ROLE | roles, permissions, badges |
| [cases/bunker.md](cases/bunker.md) | BUNKER | bunker URLs, NIP-46 sessions, external apps |
| [cases/keys.md](cases/keys.md) | KEY | encryption, rotation, strict mode |
| [cases/tenancy.md](cases/tenancy.md) | TEN | multi-community servers, subgroups, graduation |

Environment setup and commands: [environment.md](environment.md).
