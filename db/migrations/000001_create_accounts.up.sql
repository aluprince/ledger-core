-- 000001_create_accounts.up.sql
CREATE TYPE account_type AS ENUM ('asset', 'liability', 'revenue', 'expense');

CREATE TABLE accounts (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        account_type NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'NGN',
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_accounts_type ON accounts(type);
CREATE INDEX idx_accounts_created_at ON accounts(created_at);
