# 0002 — zooid provides the relay and Blossom server, pinned by commit

Status: accepted (2026-06-10)

## Context

The platform needs a Nostr relay and a Blossom media server on the
community's domain. Options: implement on khatru ourselves; embed an
existing implementation as a library; or run an existing server as a second
process. [zooid](https://gitea.coracle.social/coracle/zooid) (Coracle) is a
Go relay + Blossom + NIP-29 groups + NIP-42 auth + NIP-86 management server —
exactly our feature list — but it is designed as a standalone binary, not an
importable library, and currently has no version tags.

## Decision

Run zooid as a second systemd-managed process on `localhost:3334`, fronted by
communityd's reverse proxy. Depend on it by **git commit hash** recorded in
`ZOOID_VERSION`; our CI builds it from source for each release of ours
([operations/updating.md](../operations/updating.md)). communityd writes its
TOML config and administers it via the NIP-86 API; zooid's inotify config
hot-reload removes restart coordination.

## Consequences

- We inherit a maintained, spec-current relay (and NIP-29 — which gave us
  the #general channel nearly free) instead of owning thousands of lines of
  protocol code.
- Two processes instead of one: a second unit file and a localhost proxy —
  modest, contained complexity.
- Upstream is a moving target without releases; the pin + CI smoke test is
  the containment. If upstream diverges from our needs, the fallback is a
  fork (it's MIT-style licensed) or a khatru-based replacement behind the
  same proxy boundary.
- Updating zooid is a one-line change ([operations/updating.md](../operations/updating.md)).
