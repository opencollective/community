# Storage

Everything lives under `/opt/community`. A thin server registry, **one
self-contained SQLite database per community**
([multi-tenancy](multi-tenancy.md)), zooid's own database, and one media
directory. No external database server.

```
/opt/community/
├── bin/
│   ├── communityd
│   └── zooid                      # pinned build, see operations/updating.md
├── server.db                      # registry: hostname → community, parent links
├── communities/
│   └── <slug>/                    # one self-contained directory per community
│       ├── app.db                 # that community's database (WAL mode)
│       └── secrets/
│           └── machine.key        # per-community auto-unlock key (absent in strict mode)
├── data/
│   └── zooid/db                   # relay events, one schema per community (zooid, WAL)
├── media/<schema>/                # Blossom blobs per community, content-addressed
├── config/
│   └── zooid/<slug>.toml          # one virtual relay per community, hot-reloaded
└── acme/                          # certificate cache, all hostnames
```

## server.db (registry)

Holds no community content — only what the server needs to dispatch and
navigate:

| table | purpose |
|---|---|
| `communities` | slug, hostname, directory, parent (for subgroups), status |
| `server_settings` | server-level config (ACME contact, platform defaults) |

## app.db (one per community)

Source of truth for accounts and configuration; an *index* for anything that
is Nostr-native (the relay holds the truth for those). Identical schema in
every community — root collective or four-person circle.

| table | purpose |
|---|---|
| `settings` | community profile, domain, theme colors, posting policy, wrapped DEK, email provider config |
| `identities` | one row per person and one for the community: username, email, role flags via `role_members`, npub, encrypted nsec, kind (member / follower / external / community), status |
| `channels` | channel registry: slug, name, type (chat / threads), template, enabled, post audience, default thread visibility + override flag, approval policy (approver roles + required count), position |
| `roles` | name, color, permission flags, `is_default`, `deletable` |
| `role_members` | identity ↔ role |
| `account_managers` | managed identity ↔ manager: granted-by, since, paused-on-claim state |
| `applications` | join requests: motivation, status, decided_by |
| `application_approvals` | who approved/declined which application |
| `email_codes` | hashed 6-digit codes, purpose (login / verify / follow-confirm), expiry, attempts |
| `web_sessions` | login cookies |
| `bunker_sessions` | NIP-46 sessions ([bunker.md](bunker.md)) |
| `newsletter_log` | which kind 30023 event was emailed when, to how many recipients — guarantees at-most-once sending |
| `proposals_index` | cache of pending NIP-72 proposals/approvals for fast rendering; rebuildable from the relay |
| `threads_index` | cache of thread roots, status (pending / approved), visibility, reply and reaction counts per channel for list rendering; rebuildable from the relay |
| `feed_tokens` | per-member secret tokens for the members ICS feed; regenerable, revoked with membership |
| `ledger_index` | cache of fiscal-host ledger entries, derived balances per host × currency × earmark, attestation reconciliation; rebuildable from the relay |
| `contributions_index` | cache of confirmed payments and attributed credit sources per contributor; rebuildable from the relay |

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
