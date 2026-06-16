-- Profile edits (docs/nostr/publishing.md § profile edits). The signed
-- trail (wrappers, approvals) lives on the relay; this records which
-- wrapper was applied to the community kind 0, an at-most-once guard.

CREATE TABLE applied_profile_edits (
    wrapper_id TEXT PRIMARY KEY,
    applied_at INTEGER NOT NULL
);
