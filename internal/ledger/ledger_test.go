package ledger_test

import (
	"context"
	"testing"

	"github.com/aluprince/ledger-core/internal/ledger"
	"github.com/aluprince/ledger-core/internal/testhelper"
	"github.com/aluprince/ledger-core/pkg/money"
)

func TestPostInput_Validation(t *testing.T) {
	t.Run("zero amount is rejected", func(t *testing.T) {
		svc := ledger.NewService(testhelper.DB(t))
		_, err := svc.Post(context.Background(), ledger.PostInput{
			Reference:       "ref_001",
			DebitAccountID:  "acc_a",
			CreditAccountID: "acc_b",
			Amount:          money.Amount(0),
			Currency:        "NGN",
		})
		if err == nil {
			t.Fatal("expected error for zero amount, got nil")
		}
	})

	t.Run("negative amount is rejected", func(t *testing.T) {
		svc := ledger.NewService(testhelper.DB(t))
		_, err := svc.Post(context.Background(), ledger.PostInput{
			Reference:       "ref_002",
			DebitAccountID:  "acc_a",
			CreditAccountID: "acc_b",
			Amount:          money.Amount(-5000),
			Currency:        "NGN",
		})
		if err == nil {
			t.Fatal("expected error for negative amount, got nil")
		}
	})

	t.Run("same debit and credit account is rejected", func(t *testing.T) {
		svc := ledger.NewService(testhelper.DB(t))
		_, err := svc.Post(context.Background(), ledger.PostInput{
			Reference:       "ref_003",
			DebitAccountID:  "acc_same",
			CreditAccountID: "acc_same",
			Amount:          money.Amount(10000),
			Currency:        "NGN",
		})
		if err == nil {
			t.Fatal("expected error for same debit/credit account, got nil")
		}
	})
}

func TestPost_CreatesExactlyTwoLedgerEntries(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	srcID := testhelper.MustCreateAccount(t, db, "", "Source Account", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "", "Destination Account", "asset")

	svc := ledger.NewService(db)
	txn, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_two_entries",
		DebitAccountID:  srcID,
		CreditAccountID: dstID,
		Amount:          money.FromNaira(500),
		Currency:        "NGN",
	})
	if err != nil {
		t.Fatalf("Post() failed: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger_entries WHERE transaction_id = $1`, txn.ID).Scan(&count); err != nil {
		t.Fatalf("scan count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 ledger entries, got %d — double-entry invariant violated", count)
	}
}

func TestPost_DebitAndCreditAreSymmetric(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	srcID := testhelper.MustCreateAccount(t, db, "", "Source", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "", "Destination", "asset")

	svc := ledger.NewService(db)
	amount := money.FromNaira(1000)

	txn, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_symmetric",
		DebitAccountID:  srcID,
		CreditAccountID: dstID,
		Amount:          amount,
		Currency:        "NGN",
	})
	if err != nil {
		t.Fatalf("Post() failed: %v", err)
	}

	rows, err := db.Query(`
		SELECT direction, amount FROM ledger_entries
		WHERE transaction_id = $1 ORDER BY direction ASC`, txn.ID)
	if err != nil {
		t.Fatalf("query entries: %v", err)
	}
	defer rows.Close()

	entries := map[string]int64{}
	for rows.Next() {
		var dir string
		var amt int64
		if err := rows.Scan(&dir, &amt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		entries[dir] = amt
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if entries["DEBIT"] != amount.Kobo() {
		t.Fatalf("debit amount = %d, want %d", entries["DEBIT"], amount.Kobo())
	}
	if entries["CREDIT"] != amount.Kobo() {
		t.Fatalf("credit amount = %d, want %d", entries["CREDIT"], amount.Kobo())
	}
	if entries["DEBIT"] != entries["CREDIT"] {
		t.Fatal("debit != credit — books do not balance")
	}
}

func TestPost_CorrectAccountsAreDebited(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	srcID := testhelper.MustCreateAccount(t, db, "", "Source", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "", "Destination", "asset")

	svc := ledger.NewService(db)
	txn, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_correct_accounts",
		DebitAccountID:  srcID,
		CreditAccountID: dstID,
		Amount:          money.FromNaira(200),
		Currency:        "NGN",
	})
	if err != nil {
		t.Fatalf("Post() failed: %v", err)
	}

	var debitAccount, creditAccount string
	if err := db.QueryRow(`SELECT account_id FROM ledger_entries WHERE transaction_id = $1 AND direction = 'DEBIT'`, txn.ID).Scan(&debitAccount); err != nil {
		t.Fatalf("scan debit account: %v", err)
	}
	if err := db.QueryRow(`SELECT account_id FROM ledger_entries WHERE transaction_id = $1 AND direction = 'CREDIT'`, txn.ID).Scan(&creditAccount); err != nil {
		t.Fatalf("scan credit account: %v", err)
	}

	if debitAccount != srcID {
		t.Fatalf("DEBIT on wrong account: got %s, want %s", debitAccount, srcID)
	}
	if creditAccount != dstID {
		t.Fatalf("CREDIT on wrong account: got %s, want %s", creditAccount, dstID)
	}
}

func TestGetBalance_ComputedFromLedger(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	walletID := testhelper.MustCreateAccount(t, db, "", "User Wallet", "asset")
	inflowID := testhelper.MustCreateAccount(t, db, "", "System Inflow", "liability")

	svc := ledger.NewService(db)
	_, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_balance_check",
		DebitAccountID:  inflowID,
		CreditAccountID: walletID,
		Amount:          money.FromNaira(5000),
		Currency:        "NGN",
	})
	if err != nil {
		t.Fatalf("Post() failed: %v", err)
	}

	balance, err := svc.GetBalance(context.Background(), walletID, "NGN")
	if err != nil {
		t.Fatalf("GetBalance() failed: %v", err)
	}

	expected := money.FromNaira(5000)
	if balance != expected {
		t.Fatalf("balance = %s, want %s", balance.String(), expected.String())
	}
}

func TestGetBalance_MultipleTransactions(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	walletID := testhelper.MustCreateAccount(t, db, "", "User Wallet", "asset")
	inflowID := testhelper.MustCreateAccount(t, db, "", "System Inflow", "liability")
	outflowID := testhelper.MustCreateAccount(t, db, "", "System Outflow", "asset")

	svc := ledger.NewService(db)

	if _, err := svc.Post(context.Background(), ledger.PostInput{
		Reference: "txn_multi_credit", DebitAccountID: inflowID, CreditAccountID: walletID,
		Amount: money.FromNaira(10000), Currency: "NGN",
	}); err != nil {
		t.Fatalf("Post() credit failed: %v", err)
	}
	if _, err := svc.Post(context.Background(), ledger.PostInput{
		Reference: "txn_multi_debit1", DebitAccountID: walletID, CreditAccountID: outflowID,
		Amount: money.FromNaira(3000), Currency: "NGN",
	}); err != nil {
		t.Fatalf("Post() debit 1 failed: %v", err)
	}
	if _, err := svc.Post(context.Background(), ledger.PostInput{
		Reference: "txn_multi_debit2", DebitAccountID: walletID, CreditAccountID: outflowID,
		Amount: money.FromNaira(2000), Currency: "NGN",
	}); err != nil {
		t.Fatalf("Post() debit 2 failed: %v", err)
	}

	balance, err := svc.GetBalance(context.Background(), walletID, "NGN")
	if err != nil {
		t.Fatalf("GetBalance() failed: %v", err)
	}

	expected := money.FromNaira(5000)
	if balance != expected {
		t.Fatalf("balance = %s, want %s", balance.String(), expected.String())
	}
}

func TestPost_TransactionIsAtomicOnFailure(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	svc := ledger.NewService(db)
	_, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_atomic_fail",
		DebitAccountID:  "acc_does_not_exist",
		CreditAccountID: "acc_also_does_not_exist",
		Amount:          money.FromNaira(100),
		Currency:        "NGN",
	})
	if err == nil {
		t.Fatal("expected error posting to non-existent accounts, got nil")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger_entries`).Scan(&count); err != nil {
		t.Fatalf("scan count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 ledger entries after failed post, got %d — atomicity violated", count)
	}
}
