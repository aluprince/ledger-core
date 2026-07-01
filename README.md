# ledger-core

> Production-grade double-entry ledger & wallet API in Go.

Built to the standards of real fintech infrastructure — not a toy wallet. This project implements the accounting model that powers every serious payment platform: immutable ledger entries, atomic transfers, idempotency guarantees, and balances computed from the ledger rather than stored in a column.

---

## Architecture Decisions

These are deliberate, documented choices — not defaults.

### Money as integers (kobo)
All amounts are stored as `int64` in kobo (1 NGN = 100 kobo). Floating-point arithmetic is never used for money. This avoids precision loss that compounds into real discrepancies at volume — a mistake common in demo projects that use `float64`.

### Double-entry ledger
Every financial movement creates exactly two ledger entries: one debit and one credit. The books always balance. There is no `UPDATE accounts SET balance = balance - 500` anywhere in this codebase. This is the same accounting model used by Moniepoint, Paystack, and every bank in existence.

### Computed balances
Account balances are never stored. They are always computed on-demand:
```sql
SELECT
  SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END) -
  SUM(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END)
FROM ledger_entries
WHERE account_id = $1;
```
This makes the ledger the single source of truth. No synchronization bugs between a `balance` field and the actual transaction history.

### Idempotency keys
All mutation endpoints require an `Idempotency-Key` header. Replaying a request with the same key returns the original response without re-executing the operation. This is how production payment APIs prevent double-charges on network retries.

### sqlc over ORM
Database access is generated from raw SQL using sqlc. This produces type-safe Go code, forces explicit SQL knowledge, and eliminates the hidden query patterns that ORMs introduce. No magic. No N+1 surprises hidden behind method chains.

### Cursor-based pagination
Transaction history uses cursor pagination (`?after=txn_01J...`) instead of offset pagination (`?page=2`). Offset pagination is inconsistent under concurrent writes and degrades at scale. Cursors are stable and efficient regardless of dataset size.

### Atomic transfers
Wallet-to-wallet transfers use a single database transaction. Both ledger entries (debit + credit) commit together or neither does. There is no intermediate state where money has left one account but not arrived in another.

---

## Data Model

```
┌─────────────────┐         ┌──────────────────────┐
│    accounts     │         │   ledger_entries     │
│─────────────────│         │──────────────────────│
│ id              │◄────────│ account_id           │
│ name            │         │ transaction_id       │
│ type            │         │ direction (CR/DR)    │
│ currency        │         │ amount (int64, kobo) │
│ created_at      │         │ created_at           │
└─────────────────┘         └──────────────────────┘
                                      │
                             ┌────────┘
                             ▼
                    ┌─────────────────────┐
                    │    transactions     │
                    │─────────────────────│
                    │ id                  │
                    │ reference           │
                    │ description         │
                    │ status              │
                    │ idempotency_key     │
                    │ created_at          │
                    └─────────────────────┘
```

---

## API Reference

### Accounts

```
POST   /v1/accounts              Create an account
GET    /v1/accounts/:id          Get account details
GET    /v1/accounts/:id/balance  Compute balance from ledger
```

### Transfers

```
POST   /v1/transfers             Initiate wallet-to-wallet transfer
GET    /v1/transfers/:id         Get transfer details
```

Transfer request body:
```json
{
  "from_account_id": "acc_01J...",
  "to_account_id":   "acc_01J...",
  "amount":          50000,
  "currency":        "NGN",
  "description":     "Payment for services"
}
```
Headers: `Idempotency-Key: <uuid>`

### Transactions

```
GET    /v1/transactions?after=<cursor>&limit=20   Paginated history
GET    /v1/transactions/:id                        Single transaction
```

### Webhooks (Virtual Account Simulation)

```
POST   /v1/webhooks/inflow       Simulate a virtual account credit
```

Simulates the inflow webhook pattern used by Nigerian payment processors (Monnify, Providus) when money arrives at a virtual account.

### System

```
GET    /health                   Service health check
```

---

## Running Locally

Prerequisites: Docker, Docker Compose

```bash
git clone https://github.com/kalvin/ledger-core.git
cd ledger-core

# Start PostgreSQL and run migrations
docker-compose up -d

# Run the API
go run ./cmd/api
```

API available at `http://localhost:8080`

Environment variables:
```env
DATABASE_URL=postgres://postgres:postgres@localhost:5432/ledgercore?sslmode=disable
PORT=8080
JWT_SECRET=your-secret-here
```

---

## Project Structure

```
ledger-core/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── ledger/          # Double-entry core logic
│   ├── wallet/          # Account management
│   ├── transfer/        # Transfer orchestration
│   ├── webhook/         # Inflow simulation
│   └── middleware/      # Idempotency, auth, logging
├── db/
│   ├── migrations/      # golang-migrate SQL files
│   ├── queries/         # sqlc input SQL
│   └── sqlc.yaml
├── pkg/
│   ├── money/           # Integer money type, formatting
│   └── apierr/          # Typed error responses
├── docker-compose.yml
├── Makefile
└── README.md
```

---

## Tech Stack

| Layer | Choice | Reason |
|---|---|---|
| Language | Go 1.22 | Performance, concurrency, fintech adoption in NG |
| Database | PostgreSQL | ACID guarantees, row-level locking |
| DB Access | sqlc | Type-safe generated code, explicit SQL |
| Router | chi | Lightweight, idiomatic, composable middleware |
| Migrations | golang-migrate | Simple, battle-tested |
| IDs | ULID | Sortable, URL-safe, collision-resistant |
| Config | env + validation | 12-factor, no magic |

---

## What This Is Not

- Not a full banking application
- Not connected to real payment processors
- Not production-deployed (no live credentials)

This is a portfolio project demonstrating systems design, Go proficiency, and domain knowledge of financial infrastructure.

---

## Author

**Alu Onari** — Backend Engineer  
aluprince · GitHub
