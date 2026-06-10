# 0011 — newsletter ≠ blog, per-section policies, RSS, and the admin activity log

Status: accepted (2026-06-10)

## Context

Three governance refinements. First: blog posts and newsletters were one
concept (every community kind 30023 was emailed), but they carry different
weight — an email reaches hundreds of inboxes and is irreversible, a blog
post is just published. Second: announcements, blog posts and newsletters
should each define their own approval policy, like channels do (ADR 0010).
Third: admins need observability — what happened, who signed it, with the
actual protocol data inspectable.

## Decision

- **Blog posts and newsletters split.** Both are kind 30023 from the
  community npub; a newsletter additionally carries a `newsletter`
  self-label (NIP-32 `l`/`L` tags). Only newsletters are emailed.
  Blog posts are followed via **RSS** (`/feed.xml`) or any Nostr client;
  newsletters are archived on the site.
- **Per-section approval policies.** Announcements, blog posts, newsletter
  and profile edits each get the unified policy shape from ADR 0010
  (approver roles + required count, author excluded, admin alone always
  sufficient). Defaults: announcements 1 steward, blog 1 steward,
  newsletter **2 stewards**, profile edits 2 stewards. This replaces the
  single posting-policy enum from ADR 0005.
- **Admin activity log** at `/log` (admin-only): every signed event on the
  community's relay rendered as a human-readable line ("@alice approved
  @dan's event …") with a raw-JSON inspector per entry, category and author
  filters, pagination. The log *is* the relay — there is no separate audit
  store; the human line is a rendering, the signed event is the truth.
  Unknown kinds render generically rather than being hidden.

## Consequences

- "One approval mechanism, many policies" is now complete: channels,
  content sections and profile edits all use roles + count over kind 4550
  trails. The settings page presents them uniformly.
- The newsletter's existing delivery guarantees (at-most-once, resume,
  unsubscribe) are unchanged — only the trigger narrows to labeled events.
  Test cases MAIL-01/02/04 and PUB-09/11 updated per the contract rules.
- RSS is a new public surface generated from the relay (like the ICS feed):
  blog posts only, no pending content.
- The /log view costs little (it reads the existing relay + indexes) and
  pays for the Nostr-native data principle: because every action is a
  signed event, observability is a renderer, not an infrastructure.
- App-level actions that are not events (logins, settings changes) are out
  of the log's v1 scope; if needed later they become a separate, clearly
  non-cryptographic audit table — a future ADR.
