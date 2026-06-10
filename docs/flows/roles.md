# Roles

Discord-style roles: named, colored, grantable sets of permissions that also
act as badges next to usernames.

## Default roles

Created at setup, not deletable (rename/recolor allowed):

| role | default permissions | auto-assigned |
|---|---|---|
| steward | approve members, propose posts, approve posts | admin gets it at setup |
| moderator | moderate #general | — |
| member | (participation baseline: chat, see member pages) | on approved join |
| follower | (newsletter only) | on confirmed follow |
| fiscal host | hold funds | — granted by the admin to approved member entities ([money.md](../nostr/money.md)) |

The **admin** is not a role but the account created at setup: it holds every
permission implicitly and is the quorum shortcut ("admin alone, or 2
stewards"). Transferring or adding admins is a future ADR.

## Permissions

| flag | grants |
|---|---|
| `approve_members` | act on `/members/pending`; counts toward the 2-approval quorum |
| `propose_posts` | see `/compose`, sign proposals |
| `approve_posts` | sign kind 4550 approvals; counts toward posting quorum |
| `moderate_chat` | remove messages, mute members in #general |
| `manage_roles` | create/edit roles, assign/remove members |
| `hold_funds` | sign fiscal-host ledger entries recognized by the treasury ([money.md](../nostr/money.md)) |

Permission checks happen at action time. Quorums always require **distinct**
identities, and a proposer/applicant never counts toward their own quorum.

One deliberate exception: proposing a **community profile edit** (name,
description, icon, links) requires no permission — any member may
([publishing.md](../nostr/publishing.md) § profile edits). Approving one
uses `approve_posts` as usual.

## Custom roles

`/roles` → create: name, color, any subset of permissions. Useful both as
pure badges (a "founding member" role with no permissions) and as functional
groups. Role colors follow the theming rules
([design-system.md](../design/design-system.md)): one hex value, tints
derived.

## Badges

Badges render wherever a username appears: chat, members directory,
approvals trail. Order: admin, then roles by creation date. A member with
many roles shows the first two plus a "+n".

## Storage and protocol

Roles are app-database concepts (`roles`, `role_members`) — deliberately not
protocol-level. Where a role has protocol consequences, communityd projects
it outward: stewards are listed as moderators in the kind 34550 community
definition, and `moderate_chat` holders are NIP-29 group admins on the
relay. The app database remains the source of truth for who holds what.
