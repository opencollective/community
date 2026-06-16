-- Publishing as the community (docs/nostr/publishing.md, ADR 0011).
-- Content lives on the relay; these tables are an at-most-once guard and
-- a render cache, both rebuildable.

-- newsletter_log guarantees a newsletter is emailed at most once (MAIL-05).
CREATE TABLE newsletter_log (
    event_id        TEXT PRIMARY KEY,
    sent_at         INTEGER NOT NULL,
    recipient_count INTEGER NOT NULL
);

-- per-section approval policies (ADR 0011): roles + required count. Stored
-- here rather than settings so they query cleanly.
CREATE TABLE post_policies (
    content_type       TEXT PRIMARY KEY, -- announcement | blog | newsletter
    approve_roles      TEXT NOT NULL,
    approvals_required INTEGER NOT NULL
);
