# Architecture overview

Two processes on one machine, one domain, no external services except an email
provider.

```
                         ┌─────────────────────────────────────────────┐
                         │  your-community.org (ports 80/443)          │
                         │                                             │
   browser / nostr app ──┤  communityd (Go, single binary)             │
                         │  ├── TLS termination (Let's Encrypt)        │
                         │  ├── web app (server-rendered HTML + htmx)  │
                         │  ├── bunker (NIP-46 remote signer)          │
                         │  ├── NIP-05 (/.well-known/nostr.json)       │
                         │  ├── email (Resend, pluggable)              │
                         │  ├── newsletter (kind 30023 → email)        │
                         │  ├── app db (SQLite)                        │
                         │  │                                          │
                         │  └── reverse proxy ──► zooid (localhost:3334)
                         │                        ├── nostr relay      │
                         │                        ├── Blossom media    │
                         │                        ├── NIP-29 groups    │
                         │                        └── SQLite + ./media │
                         └─────────────────────────────────────────────┘
```

## communityd

Our binary. Go, standard library HTTP server, `html/template` views embedded
with `go:embed`, htmx for interactivity. It owns:

- **TLS** — `golang.org/x/crypto/acme/autocert`. Serves the setup wizard over
  plain HTTP until a domain is configured, then obtains a certificate and
  redirects everything to HTTPS.
- **The web app** — every route in [design/screens.md](../design/screens.md).
- **The bunker** — see [bunker.md](bunker.md). Holds every private key,
  encrypted at rest (see [key-management.md](key-management.md)). Nothing else
  in the system ever sees a key.
- **Email** — see [email.md](email.md).
- **The app database** — SQLite, see [storage.md](storage.md).
- **Supervision glue** — communityd does not supervise zooid; systemd runs
  both. communityd writes zooid's TOML config and relies on zooid's inotify
  hot-reload to pick up changes without restarts.

## zooid

A pinned upstream dependency ([decision 0002](../decisions/0002-zooid-for-relay-and-blossom.md)),
built from source in our CI and shipped in our releases. It provides the Nostr
relay (khatru-based), the Blossom media server, NIP-29 groups, NIP-42 relay
auth and the NIP-86 management API. It listens on `localhost:3334` only;
communityd is the sole way in from the network.

## Request routing

communityd terminates TLS for the whole domain and routes by path:

| Path | Handler |
|---|---|
| `/relay` (websocket upgrade) | proxy to zooid (the community data relay) |
| `/bunker` (websocket upgrade) | communityd's embedded NIP-46 transport relay (kind 24133 only, storage-free) |
| `PUT /upload`, `GET /list/*`, `/mirror`, `HEAD\|GET /{sha256}` | proxy to zooid (Blossom) |
| `/.well-known/nostr.json` | communityd (NIP-05) |
| everything else | communityd web app |

The split between `/relay` and `/bunker` is load-bearing: zooid requires
NIP-42 membership to write, which external apps' ephemeral NIP-46 client
keys can never satisfy — so signer traffic gets its own embedded,
ephemeral, auth-free relay restricted to kind 24133.

Blossom blob paths are 64 lowercase hex characters, which cannot collide with
any web route. The proxy preserves the `Host` header because zooid dispatches
virtual relays by exact host match.

## The loop that makes it elegant

The bunker uses the community's own domain as its NIP-46 transport. External
apps connect with `bunker://<pubkey>?relay=wss://your-community.org/bunker&secret=…` —
the signing conversation never leaves the community's infrastructure. No
third-party relays are required for anything.

## Many communities, one server

A server hosts any number of communities — each with its own hostname,
virtual relay (zooid is multi-tenant by design), self-contained SQLite
database, media namespace and encryption key. Subgroups of a community are
themselves full communities, and a community can graduate to its own server
by exporting its directory. See [multi-tenancy.md](multi-tenancy.md) — the
conventions apply from the first PR even though v1 ships single-community.

## Why this shape

See the ADRs: [Go + htmx single binary](../decisions/0001-go-htmx-single-binary.md),
[zooid](../decisions/0002-zooid-for-relay-and-blossom.md),
[systemd + prebuilt releases](../decisions/0006-systemd-prebuilt-releases.md).
