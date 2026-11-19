DROP TABLE notices;
DELETE FROM cases WHERE kind IN ('review_request', 'notice');
DROP INDEX cases_open_kind;
ALTER TABLE cases DROP CONSTRAINT cases_decided_check;
ALTER TABLE cases DROP CONSTRAINT cases_subject_check;
ALTER TABLE cases DROP CONSTRAINT cases_kind_check;
ALTER TABLE cases DROP COLUMN request_text;
ALTER TABLE cases ADD CONSTRAINT cases_kind_check CHECK (kind IN ('account_status', 'pack_hidden'));
ALTER TABLE cases ADD CONSTRAINT cases_check CHECK (
       (kind = 'account_status' AND user_id IS NOT NULL AND to_status IS NOT NULL)
    OR (kind = 'pack_hidden' AND pack_kind IS NOT NULL AND pack_id IS NOT NULL));
ALTER TABLE cases ADD CONSTRAINT cases_check1 CHECK ((state = 'open') = (decided_at IS NULL));
