// Package money provides a safe integer-based money type.
// All amounts are stored in the smallest currency unit (kobo for NGN).
// Never use float64 for money.
package money

import (
	"fmt"
	"strings"
)

// Amount represents a monetary value in kobo (smallest NGN unit).
// 1 NGN = 100 kobo.
type Amount int64

const (
	KoboPerNaira Amount = 100
)

// FromNaira converts a naira value (as int64) to kobo.
func FromNaira(naira int64) Amount {
	return Amount(naira) * KoboPerNaira
}

// Naira returns the whole naira portion.
func (a Amount) Naira() int64 {
	return int64(a) / int64(KoboPerNaira)
}

// Kobo returns the raw kobo value.
func (a Amount) Kobo() int64 {
	return int64(a)
}

// IsPositive returns true if amount > 0.
func (a Amount) IsPositive() bool {
	return a > 0
}

// IsZero returns true if amount == 0.
func (a Amount) IsZero() bool {
	return a == 0
}

// Add returns a + b.
func (a Amount) Add(b Amount) Amount {
	return a + b
}

// Sub returns a - b.
func (a Amount) Sub(b Amount) Amount {
	return a - b
}

// String formats the amount as "NGN X,XXX.XX".
func (a Amount) String() string {
	naira := a.Naira()
	kobo := int64(a) % int64(KoboPerNaira)
	if kobo < 0 {
		kobo = -kobo
	}
	return fmt.Sprintf("NGN %s.%02d", formatWithCommas(naira), kobo)
}

func formatWithCommas(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		s = s[1:]
	}
	var result strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(ch)
	}
	if n < 0 {
		return "-" + result.String()
	}
	return result.String()
}
