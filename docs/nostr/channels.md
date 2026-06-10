# Channels

A channel is a named space on the homepage tabs. #general is one instance of
this framework ([chat.md](chat.md)); Proposals and Requests are the first
*thread* channels; Expenses, Resources and Products/Services are planned as
templates on the same framework — not new architecture.

> Naming: the **Proposals channel** (open member discussion threads) is not
> the **pending posts** queue (NIP-72 approvals for publishing as the
> community, [publishing.md](publishing.md)). UI and docs keep these terms
> apart.

## Two interaction shapes

| type | behavior | protocol |
|---|---|---|
| `chat` | flat live messages (#general) | NIP-29 kind 9 |
| `threads` | every message starts a thread; anyone who can see it replies and reacts | kind 11 root (NIP-7D) + kind 1111 replies (NIP-22) + kind 7 reactions (NIP-25) |

Every channel is its own NIP-29 group on the local relay (`h` tag = channel
slug). Group membership mirrors the channel's audience, managed by
communityd.

## Built-in channels

| channel | type | template | who reads | who starts threads | default |
|---|---|---|---|---|---|
| #general | chat | — | members | members (chat) | on, cannot be disabled |
| Proposals | threads | proposal | members | members | **on** |
| Requests | threads | request | public | anyone with a verified email | **off** (admin enables — it opens a write surface to externals) |

Admins toggle each non-#general channel in `/settings/community`. Disabling
hides the tab and rejects writes; history stays on the relay and returns
intact when re-enabled.

## Templates

A template defines everything type-specific about a thread channel:

1. **Start-a-thread form** — fields beyond title/body, stored as tags on the
   kind 11 root and validated by communityd *before* the bunker signs;
2. **List rendering** — how the thread list is browsed (cards for proposals,
   request cards with status, expense rows with amounts, …);
3. **Lifecycle labels** — allowed status values and who may set them, as
   NIP-32 kind 1985 labels on the root (events are immutable; status is
   layered on, signed by whoever changed it);
4. **Audience defaults** — who reads, who posts.

Planned templates (the framework contract they must fit):

| template | structured fields | lifecycle | notes |
|---|---|---|---|
| proposal | title, body | open / closed | v1 |
| request | author name, body (external author, email-verified) | open / answered / closed | v1 |
| expense | amount, currency, receipt (Blossom blob) | submitted / approved / reimbursed | approval may reuse the quorum machinery of [publishing.md](publishing.md) |
| resource | category (room, vehicle, money, time, voucher, …), description, availability | available / lent / retired | offers *to* the community |
| product / service | listing details, price | active / archived | replies double as reviews; a rating is a label on the reply |

Adding a template is Go code (form, validator, renderer, labels) plus a
settings toggle — no schema migration, no new protocol, no new ADR unless
the template needs machinery the framework lacks.

## Reactions

Kind 7 (NIP-25) with emoji content, targeting a root or a reply. The UI
groups by emoji with counts; reacting again with the same emoji removes the
reaction (NIP-09 deletion). One active reaction per identity × emoji ×
target, enforced at the bunker.

## External participants

Requests (and future public channels) accept threads from non-members:

- the visitor provides a name and email, verified by 6-digit code (the same
  `email_codes` machinery as login);
- an **external identity** is created — same key handling as followers
  ([identities.md](identities.md)), held by the bunker, added to *that
  channel's group only*;
- they are notified by email when their thread receives replies (they have
  no other way to know);
- an external identity can be upgraded later — follow, or apply to join —
  keeping the same key.

Externals never gain access to members-only channels, `/members`, or
anything beyond the public channels they participate in.

## Access enforcement

Per-channel privacy is enforced at the NIP-29 group level: communityd keeps
each group's member list in sync with the channel audience (community
members for member channels; participants for public ones). Public channels
are *rendered* publicly by communityd regardless of relay auth. The
granularity of zooid's group read-scoping must be validated during
implementation — tracked as the open risk in
[decision 0008](../decisions/0008-typed-channels.md).

## Moderation

`moderate_chat` covers all channels: remove a thread (removing the root
hides the thread), remove replies, mute an identity (applies across
channels, including externals).
