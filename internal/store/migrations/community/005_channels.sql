-- The typed channels registry (docs/nostr/channels.md, ADR 0008/0010/0012).
-- Thread content lives on the relay; this is configuration only.

CREATE TABLE channels (
    id                 INTEGER PRIMARY KEY,
    slug               TEXT NOT NULL UNIQUE,
    name               TEXT NOT NULL,
    type               TEXT NOT NULL,             -- chat | threads
    template           TEXT NOT NULL DEFAULT '',  -- proposal | request | …
    enabled            INTEGER NOT NULL DEFAULT 0,
    builtin            INTEGER NOT NULL DEFAULT 0,
    default_visibility TEXT NOT NULL DEFAULT 'members', -- public | members
    overridable        INTEGER NOT NULL DEFAULT 0,
    approve_roles      TEXT NOT NULL DEFAULT 'steward',
    approvals_required INTEGER NOT NULL DEFAULT 1,
    position           INTEGER NOT NULL DEFAULT 0,
    created_at         INTEGER NOT NULL
);
