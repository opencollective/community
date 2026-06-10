# NIPs and event kinds

Everything protocol-level the platform speaks, and where each piece is
implemented (`communityd`, `zooid`, or both).

## NIPs

| NIP | name | where | how we use it |
|---|---|---|---|
| [01](https://github.com/nostr-protocol/nips/blob/master/01.md) | Basic protocol | both | events, filters, subscriptions |
| [05](https://github.com/nostr-protocol/nips/blob/master/05.md) | DNS identifiers | communityd | `username@domain` for every identity, served at `/.well-known/nostr.json` |
| [04](https://github.com/nostr-protocol/nips/blob/master/04.md) | Encrypted DM (legacy) | communityd | NIP-46 transport compatibility with older clients only |
| [09](https://github.com/nostr-protocol/nips/blob/master/09.md) | Event deletion | both | retracting chat messages and approvals |
| [19](https://github.com/nostr-protocol/nips/blob/master/19.md) | bech32 entities | communityd | npub/nprofile/naddr in UI and links; nsec never serialized outward |
| [22](https://github.com/nostr-protocol/nips/blob/master/22.md) | Comments | communityd | thread replies (kind 1111) in thread channels ([channels.md](channels.md)) |
| [23](https://github.com/nostr-protocol/nips/blob/master/23.md) | Long-form content | communityd | blog posts (kind 30023), emailed to followers |
| [25](https://github.com/nostr-protocol/nips/blob/master/25.md) | Reactions | communityd | emoji reactions (kind 7) on threads and replies |
| [29](https://github.com/nostr-protocol/nips/blob/master/29.md) | Relay-based groups | zooid | the #general channel ([chat.md](chat.md)) |
| [32](https://github.com/nostr-protocol/nips/blob/master/32.md) | Labeling | communityd | declining proposals (kind 1985, `declined` label) |
| [42](https://github.com/nostr-protocol/nips/blob/master/42.md) | Relay auth | zooid | the relay is members-only for reads and writes |
| [44](https://github.com/nostr-protocol/nips/blob/master/44.md) | Versioned encryption | communityd | NIP-46 transport (preferred) |
| [46](https://github.com/nostr-protocol/nips/blob/master/46.md) | Remote signing | communityd | the bunker ([architecture/bunker.md](../architecture/bunker.md)) |
| [72](https://github.com/nostr-protocol/nips/blob/master/72.md) | Moderated communities | communityd | proposal/approval flow for posting as the community ([publishing.md](publishing.md)) |
| [86](https://github.com/nostr-protocol/nips/blob/master/86.md) | Relay management | zooid | communityd administers the relay (bans, membership) over localhost |
| [98](https://github.com/nostr-protocol/nips/blob/master/98.md) | HTTP auth | zooid | authenticates Blossom uploads and NIP-86 calls |
| [7D](https://github.com/nostr-protocol/nips/blob/master/7D.md) | Threads | communityd | thread roots (kind 11) in thread channels ([channels.md](channels.md)) |

## Blossom (BUDs)

| BUD | where | use |
|---|---|---|
| [BUD-00/01/02](https://github.com/hzrd149/blossom) | zooid | media storage: upload (members only, NIP-98), retrieval by sha256, listing |
| BUD-11 | zooid | as implemented upstream |

Community icons and any media in posts are Blossom blobs on the community's
own domain.

## Event kinds

| kind | meaning | signed by |
|---|---|---|
| 0 | profile metadata | every identity (members, followers, the community) |
| 1 | short note — announcements; also NIP-72 proposals of announcements | community npub (published) / proposer (proposal) |
| 3 | follow list — followers and members follow the community npub | member/follower identities |
| 5 | deletion | author of the deleted event |
| 7 | NIP-25 reaction — emoji on a thread or reply | the reacting identity |
| 11 | NIP-7D thread root — proposals, requests, future templates | thread author (member or external) |
| 1111 | NIP-22 comment — thread reply | reply author |
| 1985 | NIP-32 label — proposal declined; thread lifecycle status | the decliner / status setter |
| 4550 | NIP-72 post approval | approving steward / admin |
| 9 / 10 / 11 + NIP-29 kinds | group chat messages and management | members / relay |
| 24133 | NIP-46 request/response | bunker and client ephemeral keys |
| 30023 | long-form article — blog posts; also proposals of articles | community npub / proposer |
| 34550 | NIP-72 community definition | community npub |
| 8000/8001, 13534, 28934–28936 | zooid relay membership internals | relay / members |

## Deliberately not used (yet)

- **NIP-57 zaps** — payments are out of scope for v1.
- **NIP-59 gift wrap** — would make pending proposals steward-only at real
  complexity cost; member-visible proposals are a feature for now.
- **NIP-65 relay lists** — identities publish theirs as the community relay;
  full outbox-model support can come later.
