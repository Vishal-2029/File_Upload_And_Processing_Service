-- File Upload & Processing Service — initial schema

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ── Users ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    email      VARCHAR(255) NOT NULL UNIQUE,
    password   VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Files ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS files (
    id            UUID         PRIMARY KEY,
    user_id       UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status        VARCHAR(20)  NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','processing','done','error')),
    file_type     VARCHAR(10)  NOT NULL CHECK (file_type IN ('pdf','image')),
    original_name VARCHAR(500) NOT NULL,
    storage_path  VARCHAR(1000),
    meta          JSONB        NOT NULL DEFAULT '{}',
    retry_count   INT          NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_files_user_id         ON files(user_id);
CREATE INDEX IF NOT EXISTS idx_files_status          ON files(status);
CREATE INDEX IF NOT EXISTS idx_files_user_id_created ON files(user_id, created_at DESC);

-- ── Dead Letter Jobs ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS dead_letter_jobs (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id     UUID        NOT NULL REFERENCES files(id),
    user_id     UUID        NOT NULL,
    error_msg   TEXT,
    retry_count INT         NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── updated_at trigger ───────────────────────────────────────────────────────
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE TRIGGER trg_files_updated_at
    BEFORE UPDATE ON files
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
