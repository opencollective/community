# Backup and restore

## What to back up

Everything stateful is under `/opt/community`:

| path | contains | criticality |
|---|---|---|
| `server.db` | community registry and parent links | small but irreplaceable |
| `communities/` | every community's database (accounts, settings, **encrypted keys**, wrapped DEK) and its `machine.key` | irreplaceable |
| `data/zooid/` | every Nostr event: posts, approvals trail, threads, chat history | irreplaceable |
| `media/` | Blossom blobs (icons, uploads) | content-addressed, re-uploadable in principle |
| `config/`, `acme/` | regenerable | convenience only |

Plus one thing **not** on the server: the **master password**, in the
operator's keychain. A backup of everything above without the master
password still unlocks (via `machine.key`) — which is exactly why a
strict-mode operator must treat the password as the real key to the kingdom.

`community backup <dir>` produces a consistent snapshot: SQLite online
backups of every database (WAL-safe), a JSONL relay export per community
(zooid's export tool) for greater portability, and a copy of `media/`.
`community export <slug>` produces the same artifacts for a *single*
community — this is the [graduation path](../architecture/multi-tenancy.md):
the archive imports on any other Community server.

## Cadence

Nightly snapshot + offsite copy is sufficient for most communities. The
relay JSONL compresses extremely well.

## Restore

1. Fresh install on the new machine (`install.sh`), **stop both services**
   before the wizard.
2. Restore `/opt/community/{server.db,communities,data,media}`.
3. Point DNS at the new machine; start services. communityd sees the
   completed setup, re-obtains a certificate, and resumes.
4. Strict mode: visit `/unlock`.

## Security of backups

Backups contain each community's `machine.key` next to its wrapped DEK —
anyone holding a full backup (auto-unlock mode) can decrypt that
community's keys. Either encrypt backup
archives (age/restic) or exclude `secrets/` and accept that restores require
the master password. `community backup` defaults to prompting for an age
recipient; `--no-encrypt` is explicit.
