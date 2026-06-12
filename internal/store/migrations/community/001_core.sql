-- Core per-community schema (docs/architecture/storage.md). Grows by
-- numbered migrations; never edited in place once released.

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE identities (
    id              INTEGER PRIMARY KEY,
    username        TEXT NOT NULL COLLATE NOCASE UNIQUE,
    email           TEXT COLLATE NOCASE,
    pubkey          TEXT UNIQUE,
    nsec_enc        BLOB,
    is_organization INTEGER NOT NULL DEFAULT 0,
    -- unclaimed | pending | active  (docs/nostr/identities.md)
    status          TEXT NOT NULL DEFAULT 'pending',
    created_at      INTEGER NOT NULL
);
CREATE INDEX identities_email ON identities(email);

CREATE TABLE roles (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    color       TEXT NOT NULL DEFAULT '',
    permissions TEXT NOT NULL DEFAULT '',   -- comma-separated flags
    is_default  INTEGER NOT NULL DEFAULT 0, -- default roles are undeletable
    created_at  INTEGER NOT NULL
);

CREATE TABLE role_members (
    role_id     INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    identity_id INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, identity_id)
);

CREATE TABLE account_managers (
    identity_id INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    manager_id  INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    granted_by  INTEGER NOT NULL REFERENCES identities(id),
    since       INTEGER NOT NULL,
    paused      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (identity_id, manager_id)
);

CREATE TABLE email_codes (
    id         INTEGER PRIMARY KEY,
    email      TEXT NOT NULL COLLATE NOCASE,
    code_hash  BLOB NOT NULL,
    purpose    TEXT NOT NULL, -- login | verify | follow-confirm | claim
    attempts   INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX email_codes_email ON email_codes(email, purpose);

CREATE TABLE web_sessions (
    token_hash BLOB PRIMARY KEY,
    identity_id INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
