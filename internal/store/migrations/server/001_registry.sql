CREATE TABLE communities (
    slug       TEXT PRIMARY KEY,
    hostname   TEXT NOT NULL UNIQUE,
    parent     TEXT REFERENCES communities(slug),
    status     TEXT NOT NULL DEFAULT 'active',
    created_at INTEGER NOT NULL
);

CREATE TABLE server_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
