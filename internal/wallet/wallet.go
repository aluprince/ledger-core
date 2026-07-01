// Package wallet handles account creation and balance queries.
// It sits on top of the ledger package — it does not touch ledger_entries directly.
package wallet

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/aluprince/ledger-core/pkg/money"
	"github.com/aluprince/ledger-core/pkg/uid"
)

type AccountType string

const (
	AssetAccount     AccountType = "asset"
	LiabilityAccount AccountType = "liability"
	RevenueAccount   AccountType = "revenue"
	ExpenseAccount   AccountType = "expense"
)

type Account struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Currency  string      `json:"currency"`
	CreatedAt time.Time   `json:"created_at"`
}

type BalanceResponse struct {
	AccountID string       `json:"account_id"`
	Currency  string       `json:"currency"`
	Balance   int64        `json:"balance_kobo"`   // raw kobo — clients do their own display formatting
	Display   string       `json:"balance_display"` // "NGN 1,500.00"
}

type CreateAccountInput struct {
	Name     string
	Type     AccountType
	Currency string
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateAccount(ctx context.Context, input CreateAccountInput) (*Account, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("wallet: name is required")
	}
	if input.Type == "" {
		return nil, fmt.Errorf("wallet: account type is required")
	}
	if input.Currency == "" {
		input.Currency = "NGN"
	}

	id := uid.NewAccount()
	var acc Account

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO accounts (id, name, type, currency)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, type, currency, created_at`,
		id, input.Name, string(input.Type), input.Currency,
	).Scan(&acc.ID, &acc.Name, &acc.Type, &acc.Currency, &acc.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("wallet: create account: %w", err)
	}
	return &acc, nil
}

func (s *Service) GetAccount(ctx context.Context, id string) (*Account, error) {
	var acc Account
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, currency, created_at FROM accounts WHERE id = $1`, id,
	).Scan(&acc.ID, &acc.Name, &acc.Type, &acc.Currency, &acc.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("wallet: account %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("wallet: get account: %w", err)
	}
	return &acc, nil
}

// GetBalance delegates to a raw SQL balance computation.
// The balance column does not exist — it is always computed from ledger entries.
func (s *Service) GetBalance(ctx context.Context, accountID string) (*BalanceResponse, error) {
	acc, err := s.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	var raw int64
	err = s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END), 0)
		FROM ledger_entries
		WHERE account_id = $1 AND currency = $2`,
		accountID, acc.Currency,
	).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("wallet: compute balance: %w", err)
	}

	amt := money.Amount(raw)
	return &BalanceResponse{
		AccountID: accountID,
		Currency:  acc.Currency,
		Balance:   amt.Kobo(),
		Display:   amt.String(),
	}, nil
}
