CREATE TABLE staff_accounts (
    id                        BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username                  TEXT        NOT NULL CHECK (username ~ '^[a-z0-9_]{3,32}$'),
    password_hash             TEXT        NOT NULL,
    totp_secret               BYTEA       NOT NULL,
    role                      TEXT        NOT NULL CHECK (role IN ('admin', 'moderator')),
    confidentiality_signed_at TIMESTAMPTZ,
    disabled_at               TIMESTAMPTZ,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by                BIGINT      REFERENCES staff_accounts (id),
    CHECK (role = 'admin' OR confidentiality_signed_at IS NOT NULL)
);

CREATE UNIQUE INDEX staff_accounts_username ON staff_accounts (username);

CREATE TABLE staff_sessions (
    token_hash BYTEA       PRIMARY KEY,
    staff_id   BIGINT      NOT NULL REFERENCES staff_accounts (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    user_agent TEXT
);

CREATE INDEX staff_sessions_staff ON staff_sessions (staff_id);
CREATE INDEX staff_sessions_expires ON staff_sessions (expires_at);

CREATE TABLE audit_log (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    staff_id    BIGINT      REFERENCES staff_accounts (id),
    action      TEXT        NOT NULL CHECK (char_length(action) BETWEEN 1 AND 64),
    target_type TEXT,
    target_id   TEXT,
    details     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_created ON audit_log (created_at DESC);
CREATE INDEX audit_log_target ON audit_log (target_type, target_id, created_at DESC);
CREATE INDEX audit_log_staff ON audit_log (staff_id, created_at DESC);

CREATE FUNCTION audit_log_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'audit_log is append-only';
    END IF;
    IF OLD.created_at > now() - interval '2 years' THEN
        RAISE EXCEPTION 'audit_log rows are kept for 2 years';
    END IF;
    RETURN OLD;
END
$$;

CREATE TRIGGER audit_log_guard BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_guard();
