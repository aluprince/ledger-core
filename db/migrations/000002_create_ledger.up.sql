-- 000002_create_ledger.up.sql

CREATE TYPE transaction_status AS ENUM ('pending', 'posted', 'reversed', 'failed');
CREATE TYPE entry_direction AS ENUM ('DEBIT', 'CREDIT');

CREATE TABLE transactions (
    id               TEXT PRIMARY KEY,
    reference        TEXT NOT NULL UNIQUE,
    description      TEXT,
    status           transaction_status NOT NULL DEFAULT 'pending',
    idempotency_key  TEXT UNIQUE,
    metadata         JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_reference     ON transactions(reference);
CREATE INDEX idx_transactions_idempotency   ON transactions(idempotency_key);
CREATE INDEX idx_transactions_status        ON transactions(status);
CREATE INDEX idx_transactions_created_at    ON transactions(created_at DESC);

-- Ledger entries are IMMUTABLE. Never update or delete.
-- Every financial event creates exactly 2 entries (one DEBIT, one CREDIT).
CREATE TABLE ledger_entries (
    id              TEXT PRIMARY KEY,
    transaction_id  TEXT NOT NULL REFERENCES transactions(id),
    account_id      TEXT NOT NULL REFERENCES accounts(id),
    direction       entry_direction NOT NULL,
    amount          BIGINT NOT NULL CHECK (amount > 0),   -- stored in kobo, always positive
    currency        TEXT NOT NULL DEFAULT 'NGN',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ledger_entries_account_id     ON ledger_entries(account_id);
CREATE INDEX idx_ledger_entries_transaction_id ON ledger_entries(transaction_id);
CREATE INDEX idx_ledger_entries_created_at     ON ledger_entries(created_at DESC);

-- Idempotency cache: stores the full response for replayed requests.
CREATE TABLE idempotency_keys (
    key          TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,
    response     JSONB NOT NULL,
    status_code  INT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
