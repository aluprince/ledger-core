-- 000002_create_ledger.down.sql
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS transactions;
DROP TYPE IF EXISTS entry_direction;
DROP TYPE IF EXISTS transaction_status;
