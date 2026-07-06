// Package ledger implements the double-entry accounting engine.
// This is the most critical package in the codebase.
// Rules:
//   - Every transaction creates exactly 2 ledger entries (DEBIT + CREDIT).
//   - Ledger entries are immutable — they are never updated or deleted.
//   - All operations run inside a database transaction for atomicity.
//   - Amounts are always in kobo (int64). Never float64.
package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/aluprince/ledger-core/pkg/money"
	"github.com/aluprince/ledger-core/pkg/uid"
)

// Direction of a ledger entry.
type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

// Entry represents a single line in the ledger.
type Entry struct {
	ID            string
	TransactionID string
	AccountID     string
	Direction     Direction
	Amount        money.Amount
	Currency      string
	CreatedAt     time.Time
}

// Transaction represents a posted financial event.
type Transaction struct {
	ID             string
	Reference      string
	Description    string
	Status         string
	IdempotencyKey string
	Entries        []Entry
	CreatedAt      time.Time
}

// PostInput defines a double-entry posting.
// DebitAccountID is charged; CreditAccountID is credited.
type PostInput struct {
	Reference      string
	Description    string
	DebitAccountID string
	CreditAccountID string
	Amount         money.Amount
	Currency       string
	IdempotencyKey string
}

// Service is the ledger engine.
type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// Post records a double-entry transaction atomically.
// Either both entries commit or neither does.
// This is the core method — everything else calls this.
func (s *Service) Post(ctx context.Context, input PostInput) (*Transaction, error) {
	if !input.Amount.IsPositive() {
		return nil, fmt.Errorf("ledger: amount must be positive, got %d kobo", input.Amount.Kobo())
	}
	if input.DebitAccountID == input.CreditAccountID {
		return nil, fmt.Errorf("ledger: debit and credit accounts must differ")
	}
	if input.Currency == "" {
		input.Currency = "NGN"
	}

	txnID := uid.NewTransaction()
	debitID := uid.NewLedgerEntry()
	creditID := uid.NewLedgerEntry()

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, fmt.Errorf("ledger: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	//defer tx.Rollback()

	// 1. Insert transaction record.
	var idempKey sql.NullString
	if input.IdempotencyKey != "" {
		idempKey = sql.NullString{String: input.IdempotencyKey, Valid: true}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO transactions (id, reference, description, status, idempotency_key)
		VALUES ($1, $2, $3, 'posted', $4)`,
		txnID, input.Reference, input.Description, idempKey,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: insert transaction: %w", err)
	}

	// 2. Insert DEBIT entry.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, transaction_id, account_id, direction, amount, currency)
		VALUES ($1, $2, $3, 'DEBIT', $4, $5)`,
		debitID, txnID, input.DebitAccountID, input.Amount.Kobo(), input.Currency,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: insert debit entry: %w", err)
	}

	// 3. Insert CREDIT entry.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, transaction_id, account_id, direction, amount, currency)
		VALUES ($1, $2, $3, 'CREDIT', $4, $5)`,
		creditID, txnID, input.CreditAccountID, input.Amount.Kobo(), input.Currency,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: insert credit entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("ledger: commit: %w", err)
	}

	return &Transaction{
		ID:          txnID,
		Reference:   input.Reference,
		Description: input.Description,
		Status:      "posted",
		Entries: []Entry{
			{ID: debitID, TransactionID: txnID, AccountID: input.DebitAccountID, Direction: Debit, Amount: input.Amount, Currency: input.Currency},
			{ID: creditID, TransactionID: txnID, AccountID: input.CreditAccountID, Direction: Credit, Amount: input.Amount, Currency: input.Currency},
		},
	}, nil
}

// GetBalance computes an account's balance from ledger entries.
// Balance is NEVER stored — it's always derived from the ledger.
// This is intentional: the ledger is the source of truth.
func (s *Service) GetBalance(ctx context.Context, accountID, currency string) (money.Amount, error) {
	if currency == "" {
		currency = "NGN"
	}
	var balance int64
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END), 0)
		FROM ledger_entries
		WHERE account_id = $1 AND currency = $2`,
		accountID, currency,
	).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("ledger: get balance: %w", err)
	}
	return money.Amount(balance), nil
}

// GetTransaction fetches a transaction with its entries.
func (s *Service) GetTransaction(ctx context.Context, txnID string) (*Transaction, error) {
	var txn Transaction
	err := s.db.QueryRowContext(ctx, `
		SELECT id, reference, description, status, COALESCE(idempotency_key, ''), created_at
		FROM transactions WHERE id = $1`,
		txnID,
	).Scan(&txn.ID, &txn.Reference, &txn.Description, &txn.Status, &txn.IdempotencyKey, &txn.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ledger: transaction %q not found", txnID)
	}
	if err != nil {
		return nil, fmt.Errorf("ledger: get transaction: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, transaction_id, account_id, direction, amount, currency, created_at
		FROM ledger_entries WHERE transaction_id = $1 ORDER BY created_at ASC`,
		txnID,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: get entries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e Entry
		var dir string
		if err := rows.Scan(&e.ID, &e.TransactionID, &e.AccountID, &dir, &e.Amount, &e.Currency, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Direction = Direction(dir)
		txn.Entries = append(txn.Entries, e)
	}
	return &txn, rows.Err()
}
