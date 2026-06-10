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

| channel | type | template | default thread visibility | author may override? | who starts threads | default |
|---|---|---|---|---|---|---|
| #general | chat | — | members (chat is never public) | — | members (chat) | on, cannot be disabled |
| Proposals | threads | proposal | members | no | members | **on** |
| Requests | threads | request | public | yes | anyone with a verified email | **off** (admin enables — it opens a write surface to externals) |
| Events | threads | event | public | yes | members | **off** |
| Expenses | threads | expense | members | yes | members | **off** (approval default: 2 stewards — [money.md](money.md)) |

Admins toggle each non-#general channel in `/settings/community`. Disabling
hides the tab and rejects writes; history stays on the relay and returns
intact when re-enabled. Every thread channel also has an **approval policy**
(below), configured next to its toggle.

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

| template | structured fields | template states (beyond pending / approved) | notes |
|---|---|---|---|
| proposal | title, body | open / closed | v1 |
| request | author name, body (external author, email-verified) | open / answered / closed | v1 |
| event | title, start, end, timezone, place or online URL, external link, cover image, recurrence | cancelled | v1 — feeds the ICS calendar and the homepage (below); members RSVP |
| expense | amount + currency, category, payout method(s), receipt(s) | paid | fully specced in [money.md](money.md); payout details and receipts are members-only regardless of thread visibility |
| resource | category (room, vehicle, money, time, voucher, …), description, availability | available / lent / retired | offers *to* the community |
| product / service | listing details, price | active / archived | replies double as reviews; a rating is a label on the reply |

Every thread, in every template, shares the **pending → approved** baseline
of the next section; the states above are layered on top of it.

Adding a template is Go code (form, validator, renderer, labels) plus a
settings toggle — no schema migration, no new protocol, no new ADR unless
the template needs machinery the framework lacks.

## Thread approval and status

Every thread starts **pending** and becomes **approved** when its channel's
policy is met ([decision 0010](../decisions/0010-channel-approvals-and-events.md)):

- an approval is a **kind 4550** event signed by the approver, referencing
  the thread root — the same event kind and audit-trail properties as
  [community publishing](publishing.md), in a channel context (`h` tag);
- the **policy is per channel**, configured in `/settings/community`: which
  role(s) may approve, and how many approvals are required. Default:
  **1 approval from a steward who is not the thread's author**; the admin
  always approves alone. Edits to a pending root reset approvals (new event
  id), declines are kind 1985 labels with a reason;
- channel lists have a **status filter** (all / pending / approved). Members
  see pending threads labeled as such; **visitors see only threads that are
  both approved and public** (next section) — which doubles as the
  anti-spam gate for external Requests;
- approval is what unlocks a template's side effects: events enter the
  calendar feed and the homepage, expenses become payable, and so on.

## Thread visibility

Every thread is either **public** or **members-only**
([decision 0012](../decisions/0012-thread-visibility-split-feeds.md)):

- the channel's settings define the **default visibility** for new threads
  and **whether the author may override** it (see the built-ins table; both
  knobs sit next to the channel's toggle and approval policy);
- visibility is a `visibility` tag on the signed thread root, chosen on the
  start-a-thread form when override is allowed;
- **pending threads are never public** regardless of visibility — the
  approval gate comes first; visibility takes effect at approval;
- after approval, the author or an approver role can flip visibility with a
  NIP-32 label (attributable, latest wins) — e.g. retract an event from
  public view without deleting it;
- visitors see approved + public threads; members see everything in
  channels they can access. The relay itself remains members-scoped —
  public visibility is communityd rendering, as everywhere.

This applies to every template: a members-only expense next to a public
one, a private proposal in an otherwise public channel, and so on.
Announcements get the same choice at compose time
([publishing.md](publishing.md) § content types).

## Events and the calendar feeds

The event template's thread roots are **NIP-52 calendar events** — kind
31923 (time-based) or 31922 (all-day) — signed by the author with the
channel `h` tag, so calendar-aware Nostr clients understand them natively.

- **When**: start, end, timezone (NIP-52 tags). **Recurrence is off by
  default**; the form offers presets — weekly, monthly, yearly, *every
  Monday/Tuesday/…*, and *every first/second/third/fourth Monday/… of the
  month* — encoded as an `rrule` tag carrying an RFC 5545 subset (`FREQ`,
  `INTERVAL`, `BYDAY` with ordinal prefixes like `1MO`…`4MO`,
  `UNTIL`/`COUNT`). A pragmatic extension, ignored gracefully elsewhere;
  the ICS feed carries it natively.
- **Where**: either a physical place or an online URL (`location` tag) —
  plus, in both cases, an optional **external event page URL** (`r` tag)
  rendered as a link.
- **Cover image** (optional): a Blossom upload in the NIP-52 `image` tag,
  shown on the thread and the channel list.
- **RSVP**: members signal *going / interested / not going* — a NIP-52
  **kind 31925 RSVP** signed with their key (statuses
  `accepted`/`tentative`/`declined`). 31925 is addressable, so changing
  your answer replaces the old one; the thread shows per-status counts and
  who's going. Members only.

- **Two ICS feeds** (`text/calendar`, RRULE carried natively so calendar
  apps handle recurrence expansion):
  - `/channels/events/public.ics` — approved **public** events, no
    authentication;
  - `/channels/events/members.ics?token=…` — approved events of **both**
    visibilities, authenticated by a per-member secret token (calendar
    apps can't send cookies — this is the private-URL pattern). Each member
    gets their token from the Events channel's subscribe button; it is
    regenerable, and revoked when membership ends.

  Cancelling an approved event (a label set by an approver role or the
  author) marks it `CANCELLED` in both feeds.
- **Homepage**: an "Upcoming events" section appears when the Events
  channel is enabled *and* at least one approved upcoming event is visible
  to the viewer (public ones for visitors; all for members; recurring
  events count by next occurrence). It links the matching ICS feed.
- Replies and reactions work on event threads like any other.

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
- their threads start pending like any other and become publicly visible
  once approved per the channel's policy;
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
