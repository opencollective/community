-- Per-member secret tokens for the members ICS feed (ADR 0012, EVT-11).
-- Calendar apps can't send cookies, so the members feed is authenticated
-- by a capability URL; the token is regenerable and dies with membership.

CREATE TABLE feed_tokens (
    identity_id INTEGER PRIMARY KEY REFERENCES identities(id) ON DELETE CASCADE,
    token_hash  BLOB NOT NULL,
    created_at  INTEGER NOT NULL
);
CREATE UNIQUE INDEX feed_tokens_hash ON feed_tokens(token_hash);
