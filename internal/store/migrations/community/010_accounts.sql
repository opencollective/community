-- Account claiming (docs/nostr/identities.md, UNCL-03/04). The
-- account_managers table already exists from 001_core; this adds the
-- pending-claim address so only the latest one can claim the account.

ALTER TABLE identities ADD COLUMN claim_email TEXT NOT NULL DEFAULT '';
