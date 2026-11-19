CREATE TABLE service_checks (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    component  TEXT        NOT NULL CHECK (char_length(component) BETWEEN 1 AND 64),
    status     TEXT        NOT NULL CHECK (status IN ('ok', 'slow', 'failed')),
    latency_ms INTEGER     NOT NULL CHECK (latency_ms >= 0),
    detail     TEXT,
    checked_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX service_checks_component_time ON service_checks (component, checked_at DESC);
CREATE INDEX service_checks_time ON service_checks (checked_at);

CREATE TABLE incidents (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    component  TEXT        NOT NULL,
    kind       TEXT        NOT NULL DEFAULT 'failure' CHECK (kind IN ('failure', 'monitoring_gap')),
    started_at TIMESTAMPTZ NOT NULL,
    ended_at   TIMESTAMPTZ,
    detail     TEXT,
    CHECK (ended_at IS NULL OR ended_at >= started_at)
);

CREATE UNIQUE INDEX incidents_one_open ON incidents (component) WHERE ended_at IS NULL;
CREATE INDEX incidents_started ON incidents (started_at DESC);

CREATE TABLE service_uptime_daily (
    component TEXT    NOT NULL,
    day       DATE    NOT NULL,
    ok        INTEGER NOT NULL,
    slow      INTEGER NOT NULL,
    failed    INTEGER NOT NULL,
    PRIMARY KEY (component, day)
);
