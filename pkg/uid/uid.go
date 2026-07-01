// Package uid provides ULID-based ID generation.
// ULIDs are sortable, URL-safe, and collision-resistant —
// better than UUIDs for time-ordered records like transactions.
package uid

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// prefixes for each entity type — makes IDs self-describing in logs.
const (
	PrefixAccount     = "acc"
	PrefixTransaction = "txn"
	PrefixLedger      = "ldr"
	PrefixTransfer    = "trf"
)

func generate(prefix string) string {
	entropy := rand.Reader
	ms := ulid.Timestamp(time.Now())
	id, err := ulid.New(ms, entropy)
	if err != nil {
		panic(fmt.Sprintf("uid: failed to generate ULID: %v", err))
	}
	return prefix + "_" + strings.ToLower(id.String())
}

func NewAccount() string     { return generate(PrefixAccount) }
func NewTransaction() string { return generate(PrefixTransaction) }
func NewLedgerEntry() string { return generate(PrefixLedger) }
func NewTransfer() string    { return generate(PrefixTransfer) }
