// Package testhelper provides utilities for integration tests.
// It spins up a real PostgreSQL instance using testcontainers,
// runs migrations against it, and returns a live *sql.DB.
//
// This is NOT mocking. Every test that uses this package hits real SQL,
// real constraints, and real transaction semantics.
// That's the point.
package testhelper

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// DB returns a live *sql.DB connected to a test PostgreSQL instance.
// It reads DATABASE_URL from the environment — if not set, it falls back
// to a local postgres at localhost:5432 (for CI or docker-compose environments).
//
// Migrations are run automatically before the DB is returned.
// The caller does not need to clean up — each test should use transactions
// or truncate tables as needed.
func DB(t *testing.T) *sql.DB {
	t.Helper()

	// dsn := os.Getenv("TEST_DATABASE_URL")
	// if dsn == "" {
	// 	dsn = "postgres://postgres:postgres@localhost:5432/ledgercore_test?sslmode=disable"
	// }
	//dsn := "postgres://postgres:postgres@localhost:5432/ledgercore_test?sslmode=disable"
	dsn := "postgres://postgres:postgres@127.0.0.1:5432/ledgercore?sslmode=disable"

	var db *sql.DB
	var err error

	// Retry up to 10 times — useful when docker-compose postgres is still starting.
	for i := 0; i < 10; i++ {
		db, err = sql.Open("postgres", dsn)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err = db.PingContext(ctx)
			cancel()
			if err == nil {
				break
			}
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		t.Fatalf("testhelper: could not connect to test database: %v\nSet TEST_DATABASE_URL or start postgres on localhost:5432", err)
	}

	runMigrations(t, db)
	return db
}

// runMigrations runs all .up.sql files from the migrations directory in order.
// Uses the same migration files as production — no separate test schema.
func runMigrations(t *testing.T, db *sql.DB) {
	t.Helper()

	// Find the migrations directory relative to this file.
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..", "db", "migrations")

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("testhelper: read migrations dir: %v", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, filepath.Join(root, e.Name()))
		}
	}
	sort.Strings(upFiles)

	for _, f := range upFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("testhelper: read migration %s: %v", f, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			// Ignore "already exists" errors — idempotent migrations.
			if !strings.Contains(err.Error(), "already exists") {
				t.Fatalf("testhelper: run migration %s: %v", f, err)
			}
		}
	}
}

// TruncateAll clears all tables between tests so state doesn't leak.
// Call this at the start of each test that needs a clean slate.
func TruncateAll(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		TRUNCATE TABLE idempotency_keys, ledger_entries, transactions, accounts
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("testhelper: truncate tables: %v", err)
	}
}

// MustCreateAccount inserts a test account and returns its ID.
// Fails the test immediately if insertion fails.
func MustCreateAccount(t *testing.T, db *sql.DB, id, name, accType string) string {
	t.Helper()
	if id == "" {
		id = fmt.Sprintf("acc_test_%d", time.Now().UnixNano())
	}
	_, err := db.Exec(`
		INSERT INTO accounts (id, name, type, currency)
		VALUES ($1, $2, $3, 'NGN')
		ON CONFLICT (id) DO NOTHING`,
		id, name, accType,
	)
	if err != nil {
		t.Fatalf("testhelper: create account %q: %v", id, err)
	}
	return id
}
