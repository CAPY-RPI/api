CREATE TABLE IF NOT EXISTS telemetry_interactions (
    id BIGSERIAL PRIMARY KEY,
    correlation_id VARCHAR(12) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    interaction_type VARCHAR(20) NOT NULL,
    user_id BIGINT NOT NULL,
    command_name VARCHAR(100),
    guild_id BIGINT,
    guild_name VARCHAR(100),
    channel_id BIGINT NOT NULL,
    options JSONB NOT NULL DEFAULT '{}'::jsonb,
    bot_version VARCHAR(20) NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS telemetry_completions (
    id BIGSERIAL PRIMARY KEY,
    correlation_id VARCHAR(12) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    command_name VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL,
    duration_ms NUMERIC(10,2),
    error_type VARCHAR(100),
    CONSTRAINT telemetry_completions_status_check
        CHECK (status IN ('success', 'user_error', 'internal_error'))
);
