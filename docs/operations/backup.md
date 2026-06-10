# Backup and restore

## What to back up

Everything stateful is under `/opt/community`:

| path | contains | criticality |
|---|---|---|
| `data/app.db` | accounts, settings, **encrypted keys**, wrapped DEK | irreplaceable |
| `data/zooid/` | every Nostr event: posts, approvals trail, chat history | irreplaceable |
| `media/` | Blossom blobs (icons, uploads) | content-addressed, re-uploadable in principle |
| `secrets/machine.key` | auto-unlock wrap key | see below |
| `config/`, `acme/` | regenerable | convenience only |

Plus one thing **not** on the server: the **master password**, in the
operator's keychain. A backup of everything above without the master
password still unlocks (via `machine.key`) — which is exactly why a
strict-mode operator must treat the password as the real key to the kingdom.

`community backup <dir>` produces a consistent snapshot: SQLite online
backups of both databases (WAL-safe), a JSONL relay export (zooid's export
tool) for greater portability, and a copy of `media/` + `secrets/`.

## Cadence

Nightly snapshot + offsite copy is sufficient for most communities. The
relay JSONL compresses extremely well.

## Restore

1. Fresh install on the new machine (`install.sh`), **stop both services**
   before the wizard.
2. Restore `/opt/community/{data,media,secrets}`.
3. Point DNS at the new machine; start services. communityd sees the
   completed setup, re-obtains a certificate, and resumes.
4. Strict mode: visit `/unlock`.

## Security of backups

Backups contain `machine.key` next to the wrapped DEK — anyone holding a
full backup (auto-unlock mode) can decrypt the keys. Either encrypt backup
archives (age/restic) or exclude `secrets/` and accept that restores require
the master password. `community backup` defaults to prompting for an age
recipient; `--no-encrypt` is explicit.
