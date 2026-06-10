# Design principles

The rules we check every feature against, roughly in priority order.

## 1. Progressive configuration

Only ask to configure something the first time it is needed. The wizard asks
for an email provider at the moment it must send its first email — not on a
settings page the operator has to find first. Same pattern everywhere: the
posting policy has a sensible default and is only surfaced in settings; theme
colors default to the standard palette until a community cares.

A corollary: **defaults over questions**. Every question we remove from setup
is worth more than a feature we add.

## 2. The bunker never reveals a key

No API, no admin page, no export path returns an nsec. Apps get sessions;
people get usernames and email codes. Policy that governs a key (the
community posting quorum) is enforced in the same process that holds the key,
so there is no path around it.

## 3. Nostr-native data over private tables

If a fact can be a signed event on our own relay, it should be: proposals,
approvals, declines, chat, follows, profiles. The app database holds accounts,
configuration and rendering indexes — things that are genuinely ours. Benefit:
the audit trail is cryptographically attributable, survives the app database,
and is readable by other Nostr clients ([nostr/publishing.md](../nostr/publishing.md)).

## 4. Easy to inspect, test and maintain

One Go binary plus one pinned dependency. Server-rendered HTML, no node
toolchain in production. Plain SQLite an operator can open with `sqlite3`.
Plain files under `/opt/community`. Logs in journald. If a curious operator
cannot follow what their server is doing, that's a bug.

## 5. Members own their identity

Every member's identity is a real Nostr keypair with a NIP-05 name on the
community's domain. It works in any Nostr app today via the bunker, and a
future export path can hand the key to a member who leaves (gated by the
master password and the member's own verification). Communities are not
walled gardens; they are on-ramps.

## 6. Email is the bridge, not the identity

Email is how ordinary people prove account ownership and receive content —
it is deliberately the lowest-friction proof we support first. But the
identity is the keypair; proof methods (email today, passkeys or SMS
tomorrow) are pluggable and per-account.

## 7. Fractal by design — every group is whole

Any server can host many collectives. Any collective eventually has
subgroups — circles, working groups, chapters — and a subgroup is not a
feature: it is a **full community**, with the same channels, proposals,
requests, expenses, resources, newsletter, roles and its own database and
keys. Collectives can be members of collectives, so the structure replicates
like a fractal. And because every community is self-contained, a subgroup
can graduate to its own server — a fork, not a migration — or an independent
community can join a server as a subgroup. The atomic unit is the community;
everything composes from there
([architecture/multi-tenancy.md](../architecture/multi-tenancy.md)).

## 8. Decisions are written down

Anything we'd have to re-litigate later gets an ADR in
[decisions/](../decisions). The wireframe-first, hi-fi-second, code-third
workflow applies to significant UI too.
