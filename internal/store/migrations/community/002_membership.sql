-- People flows: display names, newsletter opt-in, join applications
-- (docs/flows/join.md, docs/flows/follow.md).

ALTER TABLE identities ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE identities ADD COLUMN newsletter INTEGER NOT NULL DEFAULT 0;

CREATE TABLE applications (
    id          INTEGER PRIMARY KEY,
    identity_id INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    motivation  TEXT NOT NULL DEFAULT '',
    newsletter  INTEGER NOT NULL DEFAULT 0,
    -- awaiting_email | pending | approved | declined
    status      TEXT NOT NULL DEFAULT 'awaiting_email',
    reason      TEXT NOT NULL DEFAULT '',  -- decline reason
    created_at  INTEGER NOT NULL,
    decided_at  INTEGER
);
CREATE INDEX applications_identity ON applications(identity_id);
CREATE INDEX applications_status ON applications(status);

CREATE TABLE application_approvals (
    application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    approver_id    INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    decision       TEXT NOT NULL, -- approve | decline
    created_at     INTEGER NOT NULL,
    PRIMARY KEY (application_id, approver_id)
);
