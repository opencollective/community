# Key management

The server holds a Nostr private key (nsec) for every member, every follower,
and the community itself. Protecting those keys is the most security-critical
part of the system.

## Envelope encryption

We do not encrypt the SQLite file. We encrypt the secrets inside it
([decision 0003](../decisions/0003-envelope-encryption-not-sqlcipher.md)).

```
master password ──argon2id──► KEK (key encryption key, never stored)
                                │
                                ▼
                        wraps   DEK (random 32-byte data encryption key)
                                │
                                ▼
                        encrypts every nsec (XChaCha20-Poly1305)
```

- Each nsec is encrypted with the **DEK** using XChaCha20-Poly1305 with a
  random nonce, and stored in the `identities` table as
  `nonce ‖ ciphertext ‖ tag`. The AAD binds the ciphertext to the identity row
  id, so ciphertexts cannot be swapped between rows.
- The **DEK** is a random 32-byte key generated once at setup. It exists in
  plaintext only in communityd's memory.
- The **KEK** is derived from the master password with argon2id
  (64 MiB memory, 3 iterations, random 16-byte salt). The wrapped DEK
  (`argon2 params ‖ salt ‖ nonce ‖ ciphertext`) is stored in the `settings`
  table.

Everything that is not a private key — usernames, emails, applications,
settings — stays plaintext in SQLite, so operators can inspect their server
with plain `sqlite3`. Emails are personal data but not secrets; full-database
encryption is a non-goal (see threat model below).

## Master password rotation

Rotation re-wraps the DEK with a KEK derived from the new password. One row
update, instant, no re-encryption of data. The old password is invalid
immediately. Offered in `/settings/community`.

## Unlock modes

[Decision 0004](../decisions/0004-auto-unlock-default-strict-optional.md):

- **Auto-unlock (default)** — a second copy of the DEK is wrapped with a
  machine key stored at `/opt/community/secrets/machine.key` (mode 0600,
  owned by the service user). After a reboot, communityd unwraps the DEK
  itself and signing resumes unattended. This protects against database
  exfiltration but not against an attacker with root on the machine.
- **Strict mode (opt-in)** — no machine-wrapped copy exists. After a restart
  the bunker is locked: signing, login-code issuance and the newsletter are
  paused, and `/unlock` asks an admin for the master password. Toggling
  strict mode on deletes the machine-wrapped copy; toggling it off recreates
  it (requires the master password).

## What the master password gates

- Unlocking after restart (strict mode)
- Rotating itself
- Toggling strict mode
- Exporting key material (future, e.g. a member taking their key with them)

It is **not** used for day-to-day login — that's email codes
([flows/login.md](../flows/login.md)).

## Threat model summary

| Attacker capability | Outcome |
|---|---|
| Steals `app.db` (backup leak, snapshot) | Keys safe — nsecs are ciphertext, DEK is not in the db in usable form without the machine key or password |
| Steals `app.db` + `machine.key` (root, auto-unlock mode) | Keys compromised. Strict mode closes this at the cost of unattended restarts |
| Compromises the running process | Keys compromised (DEK is in memory). True for any signer that signs without per-event user approval |
| Guesses a weak master password offline against a leaked wrapped DEK | argon2id makes this expensive; the wizard enforces a minimum strength |

The bunker never returns an nsec to anyone — not to apps, not to admins, not
through any API. Sessions sign *through* the key ([bunker.md](bunker.md)).
