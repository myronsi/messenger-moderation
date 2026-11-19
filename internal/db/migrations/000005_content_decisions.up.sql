CREATE TABLE content_decisions (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id     BIGINT      NOT NULL,
    message_id  BIGINT      NOT NULL,
    action      TEXT        NOT NULL CHECK (action IN ('removed', 'no_violation')),
    staff_id    BIGINT      NOT NULL REFERENCES staff_accounts (id),
    reason_code TEXT,
    reason_text TEXT,
    decided_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (chat_id, message_id)
);

CREATE INDEX content_decisions_decided ON content_decisions (decided_at DESC);
