-- queries/accounts.sql

-- name: CreateAccount :one
INSERT INTO accounts (id, name, type, currency, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts
WHERE id = $1;

-- name: GetAccountBalance :one
SELECT
    COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0) -
    COALESCE(SUM(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END), 0) AS balance
FROM ledger_entries
WHERE account_id = $1 AND currency = $2;

-- name: ListAccounts :many
SELECT * FROM accounts
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
