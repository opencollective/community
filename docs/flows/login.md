# Login flow

Passwordless: email + 6-digit code. The account *is* a Nostr keypair, but
people never handle keys — proving control of the bound email opens a web
session, and the web session authorizes the bunker to sign for them.

## Steps

1. `/login`: enter email → "Send me a code".
2. communityd generates a 6-digit code: hashed (argon2id) in `email_codes`
   with purpose `login`, 10-minute expiry, 5 attempts, then sends it.
   Unknown emails receive nothing but the page behaves identically
   (no account enumeration).
3. Code entry → on match, a `web_sessions` row + HttpOnly Secure cookie
   (30 days, sliding). Codes are single-use.
4. Rate limits: 3 codes per address per hour, attempt counter per code,
   constant-time comparison.

## What a session can do

A web session belongs to one identity and lets its owner act as that
identity everywhere in the app — chat, proposals, approvals — with each
action signed by the bunker using their key. Permissions come from roles at
action time, not session creation time, so role changes apply immediately.

## Logout and revocation

- Logout deletes the session row.
- "Log out everywhere" (on `/settings/apps`) deletes all web sessions for the
  identity — separate from NIP-46 bunker sessions, which are listed and
  revoked individually on the same page.

## Relationship to ownership proof

Email is the first *proof method* bound to an account
([principles §6](../design/principles.md)). The `email_codes` mechanism is
deliberately generic (purposes: login, verify, follow-confirm) so future
methods — passkeys especially — can sit beside it without redesign.
