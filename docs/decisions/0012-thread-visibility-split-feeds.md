# 0012 — per-thread visibility with channel defaults, and split calendar feeds

Status: accepted (2026-06-10)

## Context

Events need to be public or members-only, per event. The same need
generalizes to any thread (expenses, requests, proposals) and to
announcements: sometimes one item in an otherwise public channel is
internal, or vice versa. ADR 0010 had fixed visibility at the channel
level ("visitors see approved threads only" in public channels).

## Decision

([nostr/channels.md](../nostr/channels.md) § thread visibility)

- Every thread is **public** or **members-only**, recorded as a
  `visibility` tag on the signed root. Announcements get the same choice
  at compose time.
- The **channel settings define the default** visibility for new threads
  **and whether the author may override** it. Built-in defaults:
  Proposals members/no-override, Requests public/overridable, Events
  public/overridable, #general chat always members.
- Ordering of gates: approval first — pending threads are never public;
  visibility takes effect at approval. Post-approval, the author or an
  approver role may flip visibility via a NIP-32 label (attributable,
  latest wins).
- The single events ICS feed splits in two:
  `/channels/events/public.ics` (approved public events, no auth) and
  `/channels/events/members.ics?token=…` (all approved events,
  authenticated by a per-member secret token — calendar clients cannot
  send cookies, so the capability-URL pattern applies; tokens are
  regenerable and die with membership).

## Consequences

- Channel-level "read audience" is subsumed: what a visitor sees is now
  decided per thread (approved + public), with channels supplying defaults
  rather than absolutes.
- The channel registry gains two columns (default visibility, overridable);
  thread forms gain a visibility selector when override is allowed.
- Members ICS tokens are a new credential class: low-sensitivity
  (read-only calendar), but they appear in calendar-app configs and must
  be revocable — listed alongside sessions on `/settings/apps`.
- A members-only event in a visitor's cached public feed simply disappears
  on the next poll; ICS clients handle removal natively.
- Updates ADR 0010's visibility rule; test cases EVT-04/05 updated, new
  CHAN-19/20, EVT-11, PUB-15.
