# 0003 — envelope encryption of secrets, not full-database encryption

Status: accepted (2026-06-10)

## Context

The requirement: a master password, kept in the operator's keychain,
encrypting the local SQLite data, rotatable. The obvious reading — encrypt
the whole database file (SQLCipher) — has real costs: a nonstandard CGO
build of SQLite, `PRAGMA rekey` rewriting the entire database on rotation,
and an opaque file that defeats the "inspect your server with sqlite3"
principle. Meanwhile the only true secrets in the database are the Nostr
private keys; openbunker stored these in **plaintext**, which we refuse to
repeat.

## Decision

Envelope encryption at the application layer
([architecture/key-management.md](../architecture/key-management.md)):

- a random 32-byte DEK encrypts every nsec (XChaCha20-Poly1305, per-row
  nonce, row-bound AAD);
- the DEK is wrapped by a KEK derived from the master password (argon2id);
- rotation re-wraps the DEK — one row update, instant;
- all non-secret data remains plaintext SQLite.

## Consequences

- Password rotation is O(1) instead of O(database).
- Standard SQLite everywhere: ordinary tooling, ordinary backups, online
  backup API.
- Operators can read everything except secrets — by design. Emails and
  applications are not encrypted at rest; full-disk encryption of the host
  remains the operator's call for that layer.
- The crypto we own is small and standard (argon2id + AEAD via
  golang.org/x/crypto), but it is ours to get right — concentrated in one
  reviewed package with test vectors.
