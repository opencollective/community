-- Chat moderation state (docs/nostr/chat.md): muting is app-level policy
-- enforced before the bunker signs.

ALTER TABLE identities ADD COLUMN muted INTEGER NOT NULL DEFAULT 0;
