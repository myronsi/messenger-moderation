CREATE TABLE cases (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        TEXT        NOT NULL CHECK (kind IN ('account_status', 'pack_hidden')),
    source_key  TEXT        NOT NULL UNIQUE,
    user_id     BIGINT,
    pack_kind   TEXT        CHECK (pack_kind IN ('sticker_pack', 'emoji_pack')),
    pack_id     BIGINT,
    from_status TEXT,
    to_status   TEXT,
    opened_at   TIMESTAMPTZ NOT NULL,
    deadline    TIMESTAMPTZ NOT NULL,
    state       TEXT        NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'confirmed', 'reversed', 'superseded')),
    decided_at  TIMESTAMPTZ,
    decided_by  BIGINT      REFERENCES staff_accounts (id),
    reason_code TEXT,
    reason_text TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((kind = 'account_status' AND user_id IS NOT NULL AND to_status IS NOT NULL)
        OR (kind = 'pack_hidden' AND pack_kind IS NOT NULL AND pack_id IS NOT NULL)),
    CHECK ((state = 'open') = (decided_at IS NULL))
);

CREATE INDEX cases_open ON cases (deadline) WHERE state = 'open';
CREATE INDEX cases_decided ON cases (decided_at DESC) WHERE state <> 'open';
CREATE INDEX cases_user ON cases (user_id) WHERE user_id IS NOT NULL;
CREATE INDEX cases_pack ON cases (pack_kind, pack_id) WHERE pack_id IS NOT NULL;

CREATE TABLE case_watch (
    name    TEXT   PRIMARY KEY,
    last_id BIGINT NOT NULL
);
