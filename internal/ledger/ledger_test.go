package ledger_test

import (
	"context"
	"testing"

	"github.com/aluprince/ledger-core/internal/ledger"
	"github.com/aluprince/ledger-core/internal/testhelper"
	"github.com/aluprince/ledger-core/pkg/money"
)

// ── Unit Tests ────────────────────────────────────────────────────────────────
// These test pure input validation — no database required.

func TestPostInput_Validation(t *testing.T) {
	t.Run("zero amount is rejected", func(t *testing.T) {
		db := testhelper.DB(t)
		testhelper.TruncateAll(t, db)
		svc := ledger.NewService(db)

		_, err := svc.Post(context.Background(), ledger.PostInput{
			Reference:       "ref_001",
			DebitAccountID:  "acc_a",
			CreditAccountID: "acc_b",
			Amount:          money.Amount(0), // zero — must be rejected
			Currency:        "NGN",
		})
		if err == nil {
			t.Fatal("expected error for zero amount, got nil")
		}
	})

	t.Run("negative amount is rejected", func(t *testing.T) {
		db := testhelper.DB(t)
		testhelper.TruncateAll(t, db)
		svc := ledger.NewService(db)

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
		db := testhelper.DB(t)
		testhelper.TruncateAll(t, db)
		svc := ledger.NewService(db)

		_, err := svc.Post(context.Background(), ledger.PostInput{
			Reference:       "ref_003",
			DebitAccountID:  "acc_same",
			CreditAccountID: "acc_same", // same — must be rejected
			Amount:          money.Amount(10000),
			Currency:        "NGN",
		})
		if err == nil {
			t.Fatal("expected error for same debit/credit account, got nil")
		}
	})
}

// ── Integration Tests ─────────────────────────────────────────────────────────
// These hit a real PostgreSQL database.
// They test that the double-entry invariants actually hold in the DB.

func TestPost_CreatesExactlyTwoLedgerEntries(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	// Create two real accounts.
	srcID := testhelper.MustCreateAccount(t, db, "acc_src_001", "Source Account", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "acc_dst_001", "Destination Account", "asset")

	svc := ledger.NewService(db)

	txn, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_ref_001",
		Description:     "Test posting",
		DebitAccountID:  srcID,
		CreditAccountID: dstID,
		Amount:          money.FromNaira(500), // 50,000 kobo
		Currency:        "NGN",
	})
	if err != nil {
		t.Fatalf("Post() failed: %v", err)
	}

	// Assert exactly 2 entries were created in the DB — not 1, not 3. Always 2.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM ledger_entries WHERE transaction_id = $1`, txn.ID).Scan(&count)
	if count != 2 {
		t.Fatalf("expected 2 ledger entries, got %d — double-entry invariant violated", count)
	}
}

func TestPost_DebitAndCreditAreSymmetric(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	srcID := testhelper.MustCreateAccount(t, db, "acc_src_002", "Source", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "acc_dst_002", "Destination", "asset")

	svc := ledger.NewService(db)
	amount := money.FromNaira(1000) // 100,000 kobo

	txn, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_ref_002",
		DebitAccountID:  srcID,
		CreditAccountID: dstID,
		Amount:          amount,
		Currency:        "NGN",
	})
	if err != nil {
		t.Fatalf("Post() failed: %v", err)
	}

	// Assert debit entry amount == credit entry amount.
	// The books must balance — debits always equal credits.
	rows, _ := db.Query(`
		SELECT direction, amount FROM ledger_entries
		WHERE transaction_id = $1 ORDER BY direction ASC`, txn.ID)
	defer rows.Close()

	entries := map[string]int64{}
	for rows.Next() {
		var dir string
		var amt int64
		rows.Scan(&dir, &amt)
		entries[dir] = amt
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

	srcID := testhelper.MustCreateAccount(t, db, "acc_src_003", "Source", "asset")
	dstID := testhelper.MustCreateAccount(t, db, "acc_dst_003", "Destination", "asset")

	svc := ledger.NewService(db)

	txn, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_ref_003",
		DebitAccountID:  srcID,
		CreditAccountID: dstID,
		Amount:          money.FromNaira(200),
		Currency:        "NGN",
	})
	if err != nil {
		t.Fatalf("Post() failed: %v", err)
	}

	// Assert the DEBIT entry is on srcID, CREDIT on dstID.
	var debitAccount, creditAccount string
	db.QueryRow(`SELECT account_id FROM ledger_entries WHERE transaction_id = $1 AND direction = 'DEBIT'`, txn.ID).Scan(&debitAccount)
	db.QueryRow(`SELECT account_id FROM ledger_entries WHERE transaction_id = $1 AND direction = 'CREDIT'`, txn.ID).Scan(&creditAccount)

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

	// This test proves balance is computed from ledger entries — not a stored field.
	// We post directly to the ledger and assert GetBalance reflects it.
	walletID := testhelper.MustCreateAccount(t, db, "acc_wallet_001", "User Wallet", "asset")
	inflowID := testhelper.MustCreateAccount(t, db, "acc_inflow_001", "System Inflow", "liability")

	svc := ledger.NewService(db)

	// Credit the wallet with 5,000 NGN (500,000 kobo).
	_, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_inflow_001",
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
		t.Fatalf("balance = %d kobo (%s), want %d kobo (%s)",
			balance.Kobo(), balance.String(),
			expected.Kobo(), expected.String())
	}
}

func TestGetBalance_MultipleTransactions(t *testing.T) {
	db := testhelper.DB(t)
	testhelper.TruncateAll(t, db)

	walletID := testhelper.MustCreateAccount(t, db, "acc_wallet_002", "User Wallet", "asset")
	inflowID := testhelper.MustCreateAccount(t, db, "acc_inflow_002", "System Inflow", "liability")
	outflowID := testhelper.MustCreateAccount(t, db, "acc_outflow_002", "System Outflow", "asset")

	svc := ledger.NewService(db)

	// Credit 10,000 NGN
	svc.Post(context.Background(), ledger.PostInput{
		Reference: "txn_credit_001", DebitAccountID: inflowID, CreditAccountID: walletID,
		Amount: money.FromNaira(10000), Currency: "NGN",
	})

	// Debit 3,000 NGN
	svc.Post(context.Background(), ledger.PostInput{
		Reference: "txn_debit_001", DebitAccountID: walletID, CreditAccountID: outflowID,
		Amount: money.FromNaira(3000), Currency: "NGN",
	})

	// Debit another 2,000 NGN
	svc.Post(context.Background(), ledger.PostInput{
		Reference: "txn_debit_002", DebitAccountID: walletID, CreditAccountID: outflowID,
		Amount: money.FromNaira(2000), Currency: "NGN",
	})

	// Expected: 10,000 - 3,000 - 2,000 = 5,000 NGN
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

	// Post to non-existent accounts — the DB foreign key constraint will reject it.
	// Both entries must fail — we must not end up with 1 entry committed.
	_, err := svc.Post(context.Background(), ledger.PostInput{
		Reference:       "txn_atomic_001",
		DebitAccountID:  "acc_does_not_exist",
		CreditAccountID: "acc_also_does_not_exist",
		Amount:          money.FromNaira(100),
		Currency:        "NGN",
	})
	if err == nil {
		t.Fatal("expected error posting to non-existent accounts, got nil")
	}

	// Assert zero entries were written — atomicity holds.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM ledger_entries`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 ledger entries after failed post, got %d — atomicity violated", count)
	}
}
