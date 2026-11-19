ALTER TABLE cases DROP CONSTRAINT cases_kind_check;
ALTER TABLE cases DROP CONSTRAINT cases_check;
ALTER TABLE cases DROP CONSTRAINT cases_check1;

ALTER TABLE cases ADD COLUMN request_text TEXT CHECK (char_length(request_text) <= 500);

ALTER TABLE cases ADD CONSTRAINT cases_kind_check
    CHECK (kind IN ('account_status', 'pack_hidden', 'review_request', 'notice'));
ALTER TABLE cases ADD CONSTRAINT cases_subject_check CHECK (
       (kind = 'account_status' AND user_id IS NOT NULL AND to_status IS NOT NULL)
    OR (kind = 'pack_hidden' AND pack_kind IS NOT NULL AND pack_id IS NOT NULL)
    OR (kind = 'review_request' AND user_id IS NOT NULL AND to_status IS NOT NULL AND request_text IS NOT NULL)
    OR (kind = 'notice' AND pack_id IS NULL));
ALTER TABLE cases ADD CONSTRAINT cases_decided_check CHECK ((state = 'open') = (decided_at IS NULL));

CREATE INDEX cases_open_kind ON cases (kind, deadline) WHERE state = 'open';

CREATE TABLE notices (
    case_id        BIGINT      PRIMARY KEY REFERENCES cases (id) ON DELETE CASCADE,
    notifier_name  TEXT        NOT NULL CHECK (char_length(notifier_name) BETWEEN 1 AND 200),
    notifier_email TEXT        CHECK (char_length(notifier_email) BETWEEN 3 AND 254),
    content        TEXT        NOT NULL CHECK (char_length(content) BETWEEN 1 AND 2000),
    location       TEXT        NOT NULL CHECK (char_length(location) BETWEEN 1 AND 1000),
    explanation    TEXT        NOT NULL CHECK (char_length(explanation) BETWEEN 1 AND 4000),
    received_at    TIMESTAMPTZ NOT NULL,
    entered_by     BIGINT      NOT NULL REFERENCES staff_accounts (id),
    outcome        TEXT        CHECK (outcome IN ('action_taken', 'no_violation')),
    mail_state     TEXT        NOT NULL DEFAULT 'none' CHECK (mail_state IN ('none', 'pending', 'sent', 'failed', 'skipped')),
    mail_attempts  INTEGER     NOT NULL DEFAULT 0,
    mail_next_at   TIMESTAMPTZ,
    mail_sent_at   TIMESTAMPTZ,
    CHECK (mail_state = 'none' OR notifier_email IS NOT NULL)
);

CREATE INDEX notices_mail_due ON notices (mail_next_at) WHERE mail_state = 'pending';
