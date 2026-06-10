# Storage

Everything lives under `/opt/community`. Two SQLite databases (one per
process) and one media directory. No external database server.

```
/opt/community/
├── bin/
│   ├── communityd
│   └── zooid                  # pinned build, see operations/updating.md
├── data/
│   ├── app.db                 # communityd (WAL mode)
│   └── zooid/db               # relay events (zooid, WAL mode)
├── media/                     # Blossom blobs, content-addressed by sha256
├── config/
│   └── zooid/community.toml   # written by communityd, hot-reloaded by zooid
├── secrets/
│   └── machine.key            # auto-unlock wrap key (absent in strict mode)
└── acme/                      # Let's Encrypt certificate cache
```

## app.db (communityd)

Source of truth for accounts and configuration; an *index* for anything that
is Nostr-native (the relay holds the truth for those).

| table | purpose |
|---|---|
| `settings` | community profile, domain, theme colors, posting policy, wrapped DEK, email provider config |
| `identities` | one row per person and one for the community: username, email, role flags via `role_members`, npub, encrypted nsec, kind (member / follower / external / community), status |
| `channels` | channel registry: slug, name, type (chat / threads), template, enabled, read/post audiences, position |
| `roles` | name, color, permission flags, `is_default`, `deletable` |
| `role_members` | identity ↔ role |
| `applications` | join requests: motivation, status, decided_by |
| `application_approvals` | who approved/declined which application |
| `email_codes` | hashed 6-digit codes, purpose (login / verify / follow-confirm), expiry, attempts |
| `web_sessions` | login cookies |
| `bunker_sessions` | NIP-46 sessions ([bunker.md](bunker.md)) |
| `newsletter_log` | which kind 30023 event was emailed when, to how many recipients — guarantees at-most-once sending |
| `proposals_index` | cache of pending NIP-72 proposals/approvals for fast rendering; rebuildable from the relay |
| `threads_index` | cache of thread roots, reply and reaction counts per channel for list rendering; rebuildable from the relay |

Schema migrations are embedded in the binary and applied at startup
(numbered SQL files, a `schema_version` pragma).

## zooid data

zooid owns its own SQLite database (events, tags, full-text index) and the
`media/` blob store. We treat both as opaque — interaction happens over the
relay websocket and the NIP-86 management API, never by touching zooid's
files. zooid ships `import`/`export` tools for JSONL event dumps, which we
use in [backup](../operations/backup.md).

## What can be lost vs. rebuilt

- `proposals_index`, `threads_index` — rebuildable from the relay at any time.
- Relay events — exportable as JSONL; the member/approval audit trail lives
  here, so it is backed up, not treated as cache.
- `identities.nsec` ciphertexts — **irreplaceable**. A lost DEK (lost master
  password in strict mode, or lost machine key + password) means every
  identity is gone. Backups must include the wrapped DEK and the operator
  must keep the master password in a real keychain.
