// Package middleware provides HTTP middleware for the API.
package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type contextKey string

const IdempotencyKeyCtx contextKey = "idempotency_key"

// Idempotency enforces idempotency on mutation endpoints.
// If an Idempotency-Key header is present and a matching cached response exists,
// the cached response is returned without re-executing the handler.
// This prevents double-charges on network retries.
func Idempotency(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only applies to state-changing methods.
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Read body to compute hash (for detecting same-key, different-body attacks).
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			hash := fmt.Sprintf("%x", sha256.Sum256(append([]byte(r.URL.Path), body...)))

			// Check if this key has been seen before.
			var cachedHash string
			var cachedResponse []byte
			var cachedStatus int
			err = db.QueryRowContext(r.Context(),
				`SELECT request_hash, response, status_code FROM idempotency_keys WHERE key = $1`, key,
			).Scan(&cachedHash, &cachedResponse, &cachedStatus)

			if err == nil {
				// Key found.
				if cachedHash != hash {
					// Same key, different request body — this is a client error.
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]string{
						"code":    "DUPLICATE_IDEMPOTENCY_KEY",
						"message": "idempotency key reused with different request parameters",
					})
					return
				}
				// Replay cached response.
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Idempotency-Replayed", "true")
				w.WriteHeader(cachedStatus)
				w.Write(cachedResponse)
				return
			}

			// Key not seen — run handler, capture response.
			rw := &responseCapture{ResponseWriter: w, status: http.StatusOK}
			ctx := context.WithValue(r.Context(), IdempotencyKeyCtx, key)
			next.ServeHTTP(rw, r.WithContext(ctx))

			// Cache the response for future replays.
			responseJSON, _ := json.Marshal(json.RawMessage(rw.body.Bytes()))
			db.ExecContext(r.Context(),
				`INSERT INTO idempotency_keys (key, request_hash, response, status_code)
				 VALUES ($1, $2, $3, $4) ON CONFLICT (key) DO NOTHING`,
				key, hash, responseJSON, rw.status,
			)
		})
	}
}

// responseCapture wraps ResponseWriter to capture the response for caching.
type responseCapture struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *responseCapture) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// Logger logs each request with method, path, status, and duration.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseCapture{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", r.RemoteAddr,
		)
	})
}

// Recoverer catches panics and returns a 500 rather than crashing the server.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "error", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "INTERNAL_ERROR",
					"message": "an unexpected error occurred",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
