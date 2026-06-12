# The bunker (NIP-46 remote signer)

communityd is a [NIP-46](https://github.com/nostr-protocol/nips/blob/master/46.md)
bunker for every identity it hosts. A private key is generated on the server,
encrypted at rest, and never leaves the process — clients open *sessions* and
ask the bunker to sign on their behalf.

## Two kinds of clients

1. **The web app itself.** When a logged-in member posts in #general or a
   steward approves a proposal, the web handler asks the bunker (an in-process
   call, same Go binary) to sign with that member's key. The web session
   cookie is the authorization.
2. **External Nostr apps** (Coracle, Flotilla, Amethyst, …). A member
   generates a `bunker://` URL at `/settings/apps`; the app then talks NIP-46
   over the community's own relay.

## Bunker URL and connect flow

```
bunker://<bunker-pubkey>?relay=wss://<domain>/bunker&secret=<one-time-secret>
```

The transport is communityd's **embedded** relay at `/bunker` — restricted
to kind 24133, storage-free, no auth — not zooid: the data relay requires
NIP-42 membership to write, which external apps' ephemeral client keys can
never hold (see [overview.md](overview.md) § request routing).

- One bunker keypair per *member identity* (not one global): the
  `<bunker-pubkey>` identifies which member's signer an app is talking to,
  and per-identity isolation means a leaked session secret cannot be replayed
  against another member.
- The `secret` is single-use, expires after 10 minutes, and is bound to the
  identity that generated it.
- On `connect`, the secret is consumed and a **session** is created: the
  client's pubkey is now authorized for this identity. Sessions are persisted
  (they survive restarts), listed at `/settings/apps`, and revocable there.

## Supported methods

`connect`, `get_public_key`, `sign_event`, `nip04_encrypt`, `nip04_decrypt`,
`nip44_encrypt`, `nip44_decrypt`, `ping` — requests and responses are kind
24133 events over the local relay, encrypted with NIP-44 (NIP-04 accepted for
older clients).

## Policy enforcement at the signer

Because every signature physically happens here, policy lives here too:

- **Community key quorum.** The community identity's key signs a publishable
  event only when the NIP-72 approval policy is satisfied
  ([nostr/publishing.md](../nostr/publishing.md)). There is no code path that
  signs as the community without quorum.
- **Kind restrictions per session** (future): a session can be limited to
  certain event kinds, e.g. a chat-only app.

## Sessions table

| column | notes |
|---|---|
| `identity_id` | whose key this session signs with |
| `client_pubkey` | the connected app |
| `app_name` | best-effort, from NIP-89 metadata or user-agent |
| `created_at`, `last_used_at` | shown in the UI |
| `revoked_at` | soft revocation; checked on every request |

## Library

`github.com/nbd-wtf/go-nostr` and its `nip46` package (the same machinery
behind `nak bunker`), so we implement policy, not protocol plumbing.
