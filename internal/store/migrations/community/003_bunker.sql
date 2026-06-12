-- NIP-46 bunker state (docs/architecture/bunker.md): one bunker keypair
-- per identity, one-time connect secrets, persistent revocable sessions.

ALTER TABLE identities ADD COLUMN bunker_pubkey TEXT;
ALTER TABLE identities ADD COLUMN bunker_nsec_enc BLOB;
CREATE INDEX identities_bunker_pubkey ON identities(bunker_pubkey);

CREATE TABLE bunker_secrets (
    id          INTEGER PRIMARY KEY,
    identity_id INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    secret_hash BLOB NOT NULL,
    expires_at  INTEGER NOT NULL,
    created_at  INTEGER NOT NULL
);

CREATE TABLE bunker_sessions (
    id           INTEGER PRIMARY KEY,
    identity_id  INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    client_pubkey TEXT NOT NULL,
    app_name     TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER NOT NULL,
    revoked_at   INTEGER
);
CREATE INDEX bunker_sessions_lookup ON bunker_sessions(identity_id, client_pubkey);
