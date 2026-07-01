-- queries/ledger.sql

-- name: CreateTransaction :one
INSERT INTO transactions (id, reference, description, status, idempotency_key, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetTransaction :one
SELECT * FROM transactions WHERE id = $1;

-- name: GetTransactionByReference :one
SELECT * FROM transactions WHERE reference = $1;

-- name: UpdateTransactionStatus :one
UPDATE transactions SET status = $2 WHERE id = $1 RETURNING *;

-- name: CreateLedgerEntry :one
INSERT INTO ledger_entries (id, transaction_id, account_id, direction, amount, currency)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetLedgerEntriesByTransaction :many
SELECT * FROM ledger_entries WHERE transaction_id = $1 ORDER BY created_at ASC;

-- name: ListTransactionsCursor :many
-- Cursor pagination: stable, efficient, correct under concurrent writes.
-- Offset pagination is NOT used here by design.
SELECT t.* FROM transactions t
WHERE t.id < $1
ORDER BY t.id DESC
LIMIT $2;

-- name: ListTransactionsByAccount :many
SELECT t.* FROM transactions t
INNER JOIN ledger_entries le ON le.transaction_id = t.id
WHERE le.account_id = $1
  AND ($2::text = '' OR t.id < $2)
ORDER BY t.id DESC
LIMIT $3;

-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys WHERE key = $1;

-- name: CreateIdempotencyKey :one
INSERT INTO idempotency_keys (key, request_hash, response, status_code)
VALUES ($1, $2, $3, $4)
RETURNING *;
