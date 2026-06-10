# Use case 1 — starting Commons Hub

Xavier wants a home for **Commons Hub**, a Brussels space for people
building commons. He has a VPS (1 GB RAM, Ubuntu), the domain
`commonshub.brussels`, and a Resend account.

## Day one: from bare VPS to live community (≈ 5 minutes)

1. **DNS + install.** He points an A record for `commonshub.brussels` at
   the VPS and runs the one-liner. install.sh downloads the release
   (communityd + pinned zooid), creates the systemd units, and prints
   `http://<ip>/setup`. *(spec: [operations/install.md](../operations/install.md); cases: SETUP-01)*
2. **Domain → HTTPS.** Step 1 of the wizard checks the A record, obtains a
   Let's Encrypt certificate and reloads over
   `https://commonshub.brussels`. *(flows/setup.md; SETUP-02/03)*
3. **Master password.** He generates one in his password manager and saves
   it there; leaves strict mode off — the Hub values uptime over
   paranoia, and he can flip it later. *(architecture/key-management.md; SETUP-05/06)*
4. **His account.** Username `xavier` → the server generates his keypair;
   his NIP-05 will be `xavier@commonshub.brussels`. *(nostr/identities.md; SETUP-07)*
5. **Email.** Resend API key, From `hub@commonshub.brussels`; the wizard
   shows the two DNS records Resend needs, he adds them, verification
   passes, a test email lands. *(architecture/email.md; SETUP-08/09)*
6. **Verify + profile.** He verifies his own address with a code, then
   names the community, writes the description, uploads the logo (first
   Blossom blob). Default roles (steward, moderator, member, follower,
   fiscal host) and #general are created; the community npub publishes its
   profile and NIP-72 definition. He lands on the homepage, logged in as
   admin. *(SETUP-10/11/12)*

## Week one: people

7. **Followers.** He shares the URL; visitors follow with an email and
   confirm — each silently becomes a Nostr identity that follows the
   community. *(flows/follow.md; FOLLOW-01/03)*
8. **Members.** Leen and Bob apply with motivations; Xavier approves (admin
   decides alone). He grants both the **steward** role — from now on
   member approvals can happen without him (any 2 stewards).
   *(flows/join.md, flows/roles.md; JOIN-05/06, ROLE-04)*
9. **Channels.** He enables **Proposals** (already on) and **Events**;
   leaves Requests off for now. He sets the linktree via a profile edit —
   which Leen approves, because even the admin's profile edits go through
   the quorum. *(nostr/channels.md, publishing.md § profile edits; CHAN-01, PROF-03)*
10. **First words.** Leen drafts the opening announcement in `/compose`;
    Bob approves it (announcements default to 1 steward); the community
    npub signs and it's on the homepage. The activity log shows every
    step, signed. *(nostr/publishing.md; PUB-02/04, LOG-02)*

Commons Hub now exists: a homepage with a linktree, a members directory, a
chat, threads, and a community identity any Nostr client can follow —
all on one €5/month box they own.
