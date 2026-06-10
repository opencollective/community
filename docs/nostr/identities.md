# Identities

Every person and the community itself is a real Nostr identity: a keypair
generated on the server, held by the bunker, encrypted at rest.

## The community identity

Created at setup step 6. Its kind 0 profile carries the community's name,
description, icon (a Blossom URL on the same domain) and a `links` array
rendered as the homepage linktree. Profile changes are proposed by any
member and published through the approval quorum
([publishing.md](publishing.md) § profile edits). It:

- authors announcements (kind 1) and blog posts (kind 30023) — only through
  the approval quorum ([publishing.md](publishing.md));
- publishes the NIP-72 community definition (kind 34550) listing stewards as
  moderators;
- is the npub that members and followers follow (kind 3);
- has NIP-05 `community@<domain>` (name configurable), plus `_@<domain>`
  resolving to it as the domain-level identifier.

## Member identities

Created when a join application is **submitted** (so the application itself
can be signed by the applicant's key — its first signature). Activated when
approved. Properties:

- username, unique per community, NIP-05 `username@<community-domain>`;
- kind 0 profile with their name (member-editable later);
- kind 3 follow of the community npub;
- usable from any Nostr app via the bunker.

## Follower identities

Created silently when someone follows with just an email:

- username auto-derived from the email local part, made unique
  (`marie`, `marie1`, …) — shown to them in the confirmation email;
- kind 0 profile + kind 3 follow of the community npub, published after the
  email is confirmed (double opt-in);
- email + newsletter opt-in stored in the app database.

A follower can later apply to join; the same identity is upgraded — no new
key, history preserved.

## Organizations (member entities)

A member identity can be flagged as an **organization** — a nonprofit, a
company, a sister collective. Entities join through the exact same
application flow as people, hold roles like anyone, and their key works the
same way. The flag changes rendering (no person avatar conventions) and is
a prerequisite for the fiscal-host role ([money.md](money.md)). In the
fractal model, another community's npub joining as a member is the same
mechanism ([architecture/multi-tenancy.md](../architecture/multi-tenancy.md)).

## External identities

Created when a non-member participates in a public channel (e.g. posting to
the Requests channel, [channels.md](channels.md)) after verifying their email
by code:

- same key handling as every identity (generated, encrypted, bunker-held);
- username auto-derived like followers; no kind 3 follow, no newsletter;
- relay access scoped to the public channel groups they participate in;
- notified of replies by email;
- upgradeable to follower or member, keeping the same key.

## NIP-05

`/.well-known/nostr.json?name=<username>` is served by communityd straight
from the `identities` table, with the community relay advertised in the
`relays` field. CORS is open, responses are cacheable for 5 minutes.

## Username rules

Lowercase `a–z0–9_.-`, 2–30 chars, uniqueness case-insensitive **per
community** (each community is its own namespace and NIP-05 domain). Reserved:
`admin`, `community`, `_`, `root`, `www`, plus route names (`members`,
`settings`, `posts`, `roles`, …).

## Key lifecycle

- Generation: `nostr-tools`-compatible secp256k1 via go-nostr, sourced from
  `crypto/rand`.
- Storage: encrypted with the DEK
  ([architecture/key-management.md](../architecture/key-management.md)).
- Export: not in v1; planned as a member-initiated, password-gated flow so
  people can leave with their identity
  ([design/principles.md](../design/principles.md) §5).
- Deletion: removing an identity deletes the ciphertext; its events remain on
  the relay (Nostr has no true delete) but NIP-05 stops resolving.
