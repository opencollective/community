# Join flow

Membership is reviewed: applications are approved by the admin **or two
existing members holding the approve-members permission** (stewards, by
default).

## Applying (`/join`)

Fields: name, username (suggested from the name, live availability check),
email, "who are you and why do you want to join?", newsletter opt-in. The
form states plainly that an admin or two stewards will review it.

On submit:
1. Email is verified with a 6-digit code (same mechanism as login) — we
   review humans, not typos.
2. An identity is created immediately (or upgraded from an existing follower
   identity with that email): the application itself is signed by the
   applicant's new key — its first signature, and the start of the
   Nostr-native audit trail.
3. Status `pending`; the applicant sees confirmation and gets an email
   acknowledging the application.

## Reviewing (`/members/pending`)

Visible to all members; actionable by identities with `approve_members`.
Each card shows the name, username, motivation, newsletter choice and the
approval progress ("approved by @alice · 1 of 2").

Rules:
- the admin's approval decides alone;
- otherwise two **distinct** approvers with the permission;
- a decline by the admin, or any two permission-holders, closes the
  application with an optional reason (sent in the decision email);
- approvals are recorded with approver identity and timestamp.

## On approval

1. Identity status → active; `member` role assigned.
2. kind 0 profile + kind 3 follow published; NIP-05 starts resolving.
3. Added to the relay member list and the #general group (zooid management
   API).
4. Decision email with a login link.

On decline, the identity is kept (it may hold a follower relationship) but
stays non-member; the applicant may reapply after 30 days.

## Anti-abuse

- One pending application per email/IP; codes rate-limited.
- Username squatting limited by the reserved list and review itself.
- All of `/members/pending` is invisible to non-members.
