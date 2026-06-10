# Follow flow

The lowest-friction relationship: an email address, nothing else. A follower
gets the community's long-form writing by email and — invisibly at first — a
real Nostr identity they can claim later.

## Steps

1. Visitor enters an email at `/follow` (or the homepage hero).
2. communityd creates (or finds) a follower identity:
   - username auto-derived from the email local part, lowercased, stripped to
     allowed chars, made unique by suffixing (`marie`, `marie1`, …);
   - keypair generated and encrypted; **nothing published yet**.
3. A confirmation email is sent (double opt-in): "confirm to start following
   — you'll be @marie on community.example.org".
4. On confirmation:
   - kind 0 profile and kind 3 (following the community npub) are published;
   - newsletter opt-in is recorded;
   - the page shows "you're following as @marie".

Already-followed emails get a "you're already following" email rather than an
on-page disclosure (no enumeration). Unconfirmed identities are garbage
collected after 30 days.

## Why double opt-in

Anyone can type any address into a form. Without confirmation we would
publish follow events for unowned addresses and send newsletters that train
providers to junk the domain. One extra click protects both.

## The newsletter

Every **newsletter** published by the community npub (long-form kind 30023
marked as a newsletter — [publishing.md](../nostr/publishing.md) § content
types) is emailed to all identities with a confirmed email and newsletter
opt-in — followers and members alike. Announcements and regular blog posts
are never emailed: blog posts are followed via **RSS** at `/feed.xml` or any
Nostr client. Pipeline and at-most-once guarantees:
[architecture/email.md](../architecture/email.md).

Unsubscribe: RFC 8058 one-click header plus a footer link; either flips the
opt-in flag without login. The follow relationship (kind 3) remains until
they delete it from a Nostr client — email and protocol are independent
layers.

## Upgrading to member

A follower who applies to join keeps their identity: the application upgrades
the same keypair/username, so anything they've done as a follower stays
theirs ([join.md](join.md)).
