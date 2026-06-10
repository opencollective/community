# Chat: the #general channel

The members-only, Discord-style channel on the homepage is a
[NIP-29](https://github.com/nostr-protocol/nips/blob/master/29.md) group on
the community's own zooid relay. We implement almost none of it — zooid
speaks NIP-29 natively; communityd is just a client.

#general is the `chat`-type instance of the [channels framework](channels.md);
thread channels (Proposals, Requests, …) are documented there.

## How it maps

| product concept | protocol reality |
|---|---|
| #general | a NIP-29 group (`h` tag) on the local relay, created at setup |
| membership | relay membership (NIP-42 auth + zooid member list) mirrors community membership; communityd adds/removes members via zooid's management API as they are approved or removed |
| a message | kind 9 (group chat message) signed with the member's own key by the bunker |
| moderation (remove message, mute) | NIP-29 group-admin events issued by identities holding the `moderate_chat` permission, plus kind 5 deletion |
| role badges next to names | rendered by communityd from `role_members`; not a protocol concept |

## The web client

- The browser talks to communityd (htmx + a small websocket handler);
  communityd relays to zooid over `wss://localhost`. Messages are signed
  server-side by the bunker with the *author's* key — the web session cookie
  is the authorization, consistent with every other web action.
- History loads from the relay (it is the storage); no separate chat table.

## External clients

This is the big win of doing it natively: a member who connects
[Flotilla](https://flotilla.social) (or any NIP-29 client) through their
bunker URL sees the same channel — same history, same members — from outside
the web UI. Moderation applies equally because it lives on the relay, not in
our UI.

## More channels

NIP-29 supports many groups per relay; #general is just the default one.
Additional channels — including the thread-based Proposals and Requests and
the planned Expenses/Resources/Products templates — are instances of the
[channels framework](channels.md), not protocol changes.
