package money_test

import (
	"testing"

	"github.com/aluprince/ledger-core/pkg/money"
)

// These are pure unit tests — no database, no I/O, instant.
// They test the money type that every other package depends on.

func TestFromNaira(t *testing.T) {
	tests := []struct {
		naira    int64
		wantKobo int64
	}{
		{1, 100},
		{100, 10000},
		{1000, 100000},
		{0, 0},
	}
	for _, tt := range tests {
		got := money.FromNaira(tt.naira)
		if got.Kobo() != tt.wantKobo {
			t.Errorf("FromNaira(%d) = %d kobo, want %d kobo", tt.naira, got.Kobo(), tt.wantKobo)
		}
	}
}

func TestAmount_IsPositive(t *testing.T) {
	if money.Amount(0).IsPositive() {
		t.Error("0 should not be positive")
	}
	if money.Amount(-1).IsPositive() {
		t.Error("-1 should not be positive")
	}
	if !money.Amount(1).IsPositive() {
		t.Error("1 should be positive")
	}
}

func TestAmount_Add(t *testing.T) {
	a := money.FromNaira(1000)
	b := money.FromNaira(500)
	got := a.Add(b)
	want := money.FromNaira(1500)
	if got != want {
		t.Errorf("Add: got %s, want %s", got.String(), want.String())
	}
}

func TestAmount_Sub(t *testing.T) {
	a := money.FromNaira(1000)
	b := money.FromNaira(300)
	got := a.Sub(b)
	want := money.FromNaira(700)
	if got != want {
		t.Errorf("Sub: got %s, want %s", got.String(), want.String())
	}
}

func TestAmount_String(t *testing.T) {
	tests := []struct {
		kobo int64
		want string
	}{
		{100, "NGN 1.00"},
		{10000, "NGN 100.00"},
		{150050, "NGN 1,500.50"},
		{100000000, "NGN 1,000,000.00"},
	}
	for _, tt := range tests {
		got := money.Amount(tt.kobo).String()
		if got != tt.want {
			t.Errorf("Amount(%d).String() = %q, want %q", tt.kobo, got, tt.want)
		}
	}
}

func TestFloatPrecisionIssue(t *testing.T) {
	// This test documents WHY we use integers.
	// Run this and understand the problem float64 causes.
	//
	// 0.1 + 0.2 in float64 is NOT 0.3.
	// At scale this compounds into real money discrepancies.
	// With int64 kobo, this problem does not exist.

	var floatResult = 0.1 + 0.2
	// floatResult is 0.30000000000000004 — not 0.3
	if floatResult == 0.3 {
		t.Log("float64 0.1+0.2 == 0.3 (this would be wrong in most languages)")
	}

	// With integers: 10 kobo + 20 kobo = 30 kobo. Always exact.
	a := money.Amount(10)
	b := money.Amount(20)
	if a.Add(b) != money.Amount(30) {
		t.Error("integer addition failed — this should never happen")
	}
}
