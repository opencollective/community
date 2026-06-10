# 0010 — per-channel approval policies, thread status, and NIP-52 events

Status: accepted (2026-06-10)

## Context

The Events channel needs an approval step before an event becomes public
and enters a subscribable calendar. Rather than an events-only mechanism,
the requirement generalizes: every thread channel (Proposals, Requests,
future Expenses/Resources/Products) benefits from a moderation/approval
stage — it is also the natural anti-spam gate for externally-posted
Requests. Approvers and thresholds should be configurable per channel.

## Decision

([nostr/channels.md](../nostr/channels.md) § thread approval, § events)

- Every thread carries a baseline status, **pending → approved**, derived
  from **kind 4550 approval events** referencing the thread root — the same
  kind, signatures and audit-trail semantics as community publishing
  (ADR 0005), distinguished by the channel `h` tag context.
- The **policy is per channel**, stored in the channel registry: a set of
  approving roles plus a required count. Default everywhere: **1 approval
  by a steward who is not the author**; the admin always approves alone.
  Author exclusion and edit-resets-approvals carry over from ADR 0005.
- Channel lists filter by status. Non-members of public channels see
  approved threads only.
- Template side effects key on approval (calendar feed, homepage, future
  expense payment).
- **Events template**: thread roots are NIP-52 calendar events (kinds
  31922/31923) so calendar-aware Nostr clients read them natively;
  recurrence is an `rrule` tag (RFC 5545 subset); approved events are
  served as a standard ICS feed at `/channels/events.ics` with native
  `RRULE` lines; an "Upcoming events" homepage section appears only when
  the channel is enabled and a future approved event exists.

## Consequences

- One approval mechanism across the platform: posting as the community,
  profile edits and channel threads are all kind 4550 trails with
  role-based quorums — different policies, same machinery and the same
  signed attributability.
- The channel registry grows policy columns (roles, count); the settings
  page gains a per-channel policy row next to each toggle.
- Behavior change to ADR 0008's Requests flow: external threads are no
  longer publicly visible on creation — they are pending until approved
  (test case CHAN-08 updated accordingly, per the testing contract rules).
- The `rrule` tag is a pragmatic extension to NIP-52 — documented, ignored
  gracefully by other clients; the ICS feed carries the authoritative
  recurrence semantics.
- Pending-thread visibility for members is transparency by default, same
  reasoning as pending posts (ADR 0005).
