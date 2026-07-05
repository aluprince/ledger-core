package transfer_test

import (
	"context"
	"testing"

	"github.com/aluprince/ledger-core/internal/ledger"
	"github.com/aluprince/ledger-core/internal/testhelper"
	"github.com/aluprince/ledger-core/internal/transfer"
	"github.com/aluprince/ledger-core/pkg/money"
)

func setup(t *testing.T) (*transfer.Service, func()) {
	t.Helper()
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)
	l := ledger.NewService(db)
	svc := transfer.NewService(db, l)
	return svc, func() { testhelper.TruncateAll(t, db) }
}

func TestInitiate_RejectsSameAccount(t *testing.T) {
	svc, cleanup := setup(t)
	defer cleanup()

	_, err := svc.Initiate(context.Background(), transfer.InitiateInput{
		FromAccountID: "acc_same",
		ToAccountID:   "acc_same",
		Amount:        money.FromNaira(100),
		Currency:      "NGN",
	})
	if err == nil {
		t.Fatal("expected error transferring to same account, got nil")
	}
}

func TestInitiate_RejectsZeroAmount(t *testing.T) {
	svc, cleanup := setup(t)
	defer cleanup()

	_, err := svc.Initiate(context.Background(), transfer.InitiateInput{
		FromAccountID: "acc_a",
		ToAccountID:   "acc_b",
		Amount:        money.Amount(0),
		Currency:      "NGN",
	})
	if err == nil {
		t.Fatal("expected error for zero amount, got nil")
	}
}

func TestInitiate_RejectsNegativeAmount(t *testing.T) {
	svc, cleanup := setup(t)
	defer cleanup()

	_, err := svc.Initiate(context.Background(), transfer.InitiateInput{
		FromAccountID: "acc_a",
		ToAccountID:   "acc_b",
		Amount:        money.Amount(-1000),
		Currency:      "NGN",
	})
	if err == nil {
		t.Fatal("expected error for negative amount, got nil")
	}
}

func TestInitiate_RejectsMissingFromAccount(t *testing.T) {
	svc, cleanup := setup(t)
	defer cleanup()

	_, err := svc.Initiate(context.Background(), transfer.InitiateInput{
		FromAccountID: "",
		ToAccountID:   "acc_b",
		Amount:        money.FromNaira(100),
	})
	if err == nil {
		t.Fatal("expected error for empty from_account_id, got nil")
	}
}

func TestInitiate_RejectsInsufficientFunds(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	// Create accounts with zero balance.
	srcID := testhelper.MustCreateAccount(t, db, "acc_broke_001", "Broke Account", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "acc_dst_010", "Destination", "asset")

	l := ledger.NewService(db)
	svc := transfer.NewService(db, l)

	// Try to transfer 500 NGN from an account with 0 balance.
	_, err := svc.Initiate(context.Background(), transfer.InitiateInput{
		FromAccountID: srcID,
		ToAccountID:   dstID,
		Amount:        money.FromNaira(500),
		Currency:      "NGN",
	})
	if err == nil {
		t.Fatal("expected insufficient funds error, got nil")
	}
}

func TestInitiate_SuccessfulTransfer(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	srcID := testhelper.MustCreateAccount(t, db, "acc_rich_001", "Rich Account", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "acc_dst_020", "Destination", "asset")
	inflowID := testhelper.MustCreateAccount(t, db, "acc_inflow_010", "Inflow", "liability")

	l := ledger.NewService(db)

	// Fund the source account with 10,000 NGN first.
	_, err := l.Post(context.Background(), ledger.PostInput{
		Reference:       "fund_src_001",
		DebitAccountID:  inflowID,
		CreditAccountID: srcID,
		Amount:          money.FromNaira(10000),
		Currency:        "NGN",
	})
	if err != nil {
		t.Fatalf("funding source account failed: %v", err)
	}

	svc := transfer.NewService(db, l)

	// Transfer 3,000 NGN from src to dst.
	trf, err := svc.Initiate(context.Background(), transfer.InitiateInput{
		FromAccountID: srcID,
		ToAccountID:   dstID,
		Amount:        money.FromNaira(3000),
		Currency:      "NGN",
		Description:   "Test transfer",
	})
	if err != nil {
		t.Fatalf("Initiate() failed: %v", err)
	}

	// Assert the transfer record is correct.
	if trf.FromAccountID != srcID {
		t.Errorf("from_account = %s, want %s", trf.FromAccountID, srcID)
	}
	if trf.ToAccountID != dstID {
		t.Errorf("to_account = %s, want %s", trf.ToAccountID, dstID)
	}
	if trf.Amount != money.FromNaira(3000).Kobo() {
		t.Errorf("amount = %d kobo, want %d kobo", trf.Amount, money.FromNaira(3000).Kobo())
	}

	// Assert source balance decreased: 10,000 - 3,000 = 7,000 NGN
	srcBalance, err := l.GetBalance(context.Background(), srcID, "NGN")
	if err != nil {
		t.Fatalf("GetBalance(src) failed: %v", err)
	}
	if srcBalance != money.FromNaira(7000) {
		t.Errorf("src balance = %s, want NGN 7,000.00", srcBalance.String())
	}

	// Assert destination balance increased: 0 + 3,000 = 3,000 NGN
	dstBalance, err := l.GetBalance(context.Background(), dstID, "NGN")
	if err != nil {
		t.Fatalf("GetBalance(dst) failed: %v", err)
	}
	if dstBalance != money.FromNaira(3000) {
		t.Errorf("dst balance = %s, want NGN 3,000.00", dstBalance.String())
	}
}

func TestInitiate_MoneyIsConserved(t *testing.T) {
	// The most important financial invariant:
	// total money in the system must not change after a transfer.
	// src_balance + dst_balance before == src_balance + dst_balance after.
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	srcID := testhelper.MustCreateAccount(t, db, "acc_conserve_src", "Source", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "acc_conserve_dst", "Destination", "asset")
	inflowID := testhelper.MustCreateAccount(t, db, "acc_conserve_inflow", "Inflow", "liability")

	l := ledger.NewService(db)

	// Fund source with 8,000 NGN.
	l.Post(context.Background(), ledger.PostInput{
		Reference: "fund_conserve", DebitAccountID: inflowID, CreditAccountID: srcID,
		Amount: money.FromNaira(8000), Currency: "NGN",
	})

	svc := transfer.NewService(db, l)
	svc.Initiate(context.Background(), transfer.InitiateInput{
		FromAccountID: srcID, ToAccountID: dstID,
		Amount: money.FromNaira(5000), Currency: "NGN",
	})

	srcBal, _ := l.GetBalance(context.Background(), srcID, "NGN")
	dstBal, _ := l.GetBalance(context.Background(), dstID, "NGN")

	total := srcBal.Add(dstBal)
	expected := money.FromNaira(8000)

	if total != expected {
		t.Fatalf("money not conserved: src(%s) + dst(%s) = %s, want %s",
			srcBal.String(), dstBal.String(), total.String(), expected.String())
	}
}
