// Package webhook simulates the inflow webhook pattern used by
// Nigerian payment processors (Monnify, Providus, Wema).
// When a customer pays to a virtual account, the processor fires
// a webhook to the merchant's server. This package simulates that flow.
package webhook

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/aluprince/ledger-core/internal/ledger"
	"github.com/aluprince/ledger-core/pkg/money"
)

// systemInflowAccountID is a special ledger account representing external money
// entering the system. In double-entry: external funds CREDIT this account,
// then DEBIT the user's wallet. Or more precisely: we DEBIT the inflow source
// and CREDIT the user account.
const systemInflowAccountID = "acc_system_inflow"

type InflowInput struct {
	AccountID   string
	Amount      money.Amount
	Currency    string
	Reference   string
	Description string
}

type Service struct {
	db     *sql.DB
	ledger *ledger.Service
}

func NewService(db *sql.DB, l *ledger.Service) *Service {
	return &Service{db: db, ledger: l}
}

// ProcessInflow simulates receiving money from a virtual account credit.
// The system inflow account is debited; the user's account is credited.
// This ensures the books balance even for external money entering the system.
func (s *Service) ProcessInflow(ctx context.Context, input InflowInput) (*ledger.Transaction, error) {
	if input.AccountID == "" {
		return nil, fmt.Errorf("webhook: account_id is required")
	}
	if !input.Amount.IsPositive() {
		return nil, fmt.Errorf("webhook: amount must be positive")
	}
	if input.Currency == "" {
		input.Currency = "NGN"
	}
	if input.Reference == "" {
		return nil, fmt.Errorf("webhook: reference is required")
	}
	if input.Description == "" {
		input.Description = "Virtual account inflow"
	}

	// Ensure the system inflow account exists.
	if err := s.ensureSystemAccount(ctx); err != nil {
		return nil, err
	}

	txn, err := s.ledger.Post(ctx, ledger.PostInput{
		Reference:       input.Reference,
		Description:     input.Description,
		DebitAccountID:  systemInflowAccountID, // external source
		CreditAccountID: input.AccountID,       // user wallet receives funds
		Amount:          input.Amount,
		Currency:        input.Currency,
	})
	if err != nil {
		return nil, fmt.Errorf("webhook: post inflow: %w", err)
	}

	return txn, nil
}

func (s *Service) ensureSystemAccount(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO accounts (id, name, type, currency)
		VALUES ($1, 'System Inflow', 'liability', 'NGN')
		ON CONFLICT (id) DO NOTHING`,
		systemInflowAccountID,
	)
	return err
}
