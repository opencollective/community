# Test environment

## Prerequisites

- Go 1.25+ and a C toolchain (`gcc`) — SQLite needs CGO
- `make`
- No network access required: the zooid test binary is built once from the
  commit pinned in [`ZOOID_VERSION`](../../ZOOID_VERSION) by
  `make zooid` (cached under `.cache/zooid-<commit>/`), and everything else
  is local

## Running

```sh
make test          # unit tests: go test ./...
make zooid         # build the pinned zooid once (needed by integration)
make test-it       # integration: go test -tags integration ./tests/...
make test-all      # both + the case-coverage check
make test-coverage-cases   # every case ID in docs/testing/cases has a test
```

Any single case: `go test -tags integration -run TestPUB04 ./tests/...`

## What test mode changes

communityd accepts a test configuration (used by the harness, also handy for
local hacking) that swaps exactly four things — nothing else differs from
production:

| concern | production | test |
|---|---|---|
| TLS / ACME | autocert on 80/443 | plain HTTP on an ephemeral port; "domain" is `127.0.0.1:<port>` |
| email | configured provider (Resend) | `FakeMailer`: captures every message in memory for assertions; its `Verify` is scriptable (verified / missing-DNS-records) |
| clock | wall clock | injectable, so code expiry, GC windows ("30 days") and session lifetimes are tested by advancing time, never by sleeping |
| argon2id parameters | 64 MiB / 3 iterations | minimal legal parameters, so key wrapping stays real but fast |

The bunker, the relay, SQLite, signatures, NIP-46/29/72 — always real.

## The integration harness

`tests/harness` boots the world once per test binary and gives each test an
isolated community:

```go
h := harness.New(t)                  // tmpdir, app.db, zooid on an ephemeral port,
                                     // communityd via httptest, FakeMailer, fake clock
h.CompleteSetup()                    // runs the actual wizard over HTTP
                                     // (skipped only by tests that test the wizard)
alice := h.Member("alice", harness.Steward)   // application + approval via real flows,
                                              // returns a logged-in HTTP client
nostr := h.NostrClient(alice)        // go-nostr client: NIP-42 auth'd websocket
                                     // to the relay, NIP-46 session to the bunker
msg := h.Mailer.LastTo("a@b.c")      // captured email: .Subject, .Text, .HTML
h.Clock.Advance(10 * time.Minute)
```

Principles:

- **Fixtures go through the front door.** `h.Member()` submits a real
  application and approves it through real handlers — so fixtures break when
  flows break, which is the point. A test that needs 50 members may use
  `h.SeedMembers(50)`, which inserts rows directly and says so in its name.
- **One zooid process per test binary**, one *schema* (virtual relay) per
  test — zooid is multi-tenant by config, which keeps integration tests
  parallel and fast.
- **Assertions on protocol, not internals**: tests read events back via a
  real relay subscription, not by querying zooid's database.

## CI

GitHub Actions runs `make test-all` on every PR (amd64; arm64 build-only),
plus the release smoke test on tags
([operations/updating.md](../operations/updating.md)). The zooid build is
cached on `ZOOID_VERSION`'s hash, so bumping the pin is what invalidates it.
