ALTER TABLE daily_stats
    ADD COLUMN messages_poll     BIGINT,
    ADD COLUMN messages_contact  BIGINT,
    ADD COLUMN messages_location BIGINT;
