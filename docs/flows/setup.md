# Setup flow

Six steps, one-time, resumable. Every route redirects to the first incomplete
step until setup finishes. Designed so each step configures exactly what the
next step needs ([progressive configuration](../design/principles.md)).

## Step 1 — domain (`/setup`, plain HTTP)

The only page ever served over HTTP. Operator enters a domain or subdomain.

Behind the submit:
1. Resolve the domain's A/AAAA record and compare against the server's public
   IP (detected via the request itself); show a precise error with the record
   to create if it doesn't match.
2. Request a Let's Encrypt certificate (HTTP-01 via autocert, cached in
   `/opt/community/acme/`).
3. Persist the domain, write zooid's TOML (`host = <domain>`), start the
   HTTPS listener, redirect to `https://<domain>/setup/password`. Port 80
   serves redirects + ACME challenges from then on.

Failure states: DNS mismatch, ports 80/443 unreachable from the internet,
ACME rate limits (told explicitly, with retry timing).

## Step 2 — master password (`/setup/password`)

Two fields + strength meter, strict-mode checkbox (default off,
[decision 0004](../decisions/0004-auto-unlock-default-strict-optional.md)).
Generates the DEK, wraps it with the password-derived KEK (and with the
machine key unless strict), stores wrapped copies
([key-management.md](../architecture/key-management.md)).

## Step 3 — your account (`/setup/admin`)

Username (validated live against the rules in
[identities.md](../nostr/identities.md)) and the ownership-proof method —
email is the only option in v1 and leads to step 4. Generates the admin
identity keypair, encrypted immediately.

## Step 4 — enable email sending (`/setup/email`)

Provider (Resend), API key, From address (prefilled
`community@<domain>`). "Save and send a test" calls the provider's domain
verification; if the From domain isn't verified, the missing DNS records are
displayed verbatim and the step blocks until verification passes. Outgoing
config only — the operator's address is deliberately not asked here.

## Step 5 — verify it's you (`/setup/verify`)

The admin's own email → 6-digit code (10 min, 5 attempts, hashed at rest) →
on success the email is bound to the admin identity and a web session is
created. The operator is now logged in, and the email pipeline is proven
end-to-end.

## Step 6 — community profile (`/setup/community`)

Name, description, icon (URL or upload — the upload is the first Blossom
blob). Then communityd:

1. generates the community identity, publishes its kind 0 profile;
2. publishes the NIP-72 community definition (kind 34550);
3. creates the #general NIP-29 group;
4. creates default roles (steward, moderator, member, follower, fiscal
   host) and assigns the admin the steward role;
5. publishes the admin's kind 0 + kind 3 (following the community);
6. redirects to the homepage. Done.

## Re-running

The wizard never reappears after completion. Domain changes, email changes,
theme and policy live in `/settings/community`. A reinstall over an existing
`/opt/community/data` detects the database and skips setup.
