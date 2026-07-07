// Package server wires up HTTP handlers and routes.
package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/aluprince/ledger-core/internal/ledger"
	"github.com/aluprince/ledger-core/internal/middleware"
	"github.com/aluprince/ledger-core/internal/transfer"
	"github.com/aluprince/ledger-core/internal/wallet"
	"github.com/aluprince/ledger-core/internal/webhook"
	"github.com/aluprince/ledger-core/pkg/apierr"
	"github.com/aluprince/ledger-core/pkg/money"
)

type Server struct {
	db       *sql.DB
	wallets  *wallet.Service
	ledger   *ledger.Service
	transfer *transfer.Service
	webhook  *webhook.Service
}

func New(db *sql.DB) *Server {
	l := ledger.NewService(db)
	return &Server{
		db:       db,
		wallets:  wallet.NewService(db),
		ledger:   l,
		transfer: transfer.NewService(db, l),
		webhook:  webhook.NewService(db, l),
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Global middleware stack — order matters.
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(middleware.Idempotency(s.db))

	r.Get("/health", s.handleHealth)

	r.Route("/v1", func(r chi.Router) {
		// Accounts
		r.Post("/accounts", s.handleCreateAccount)
		r.Get("/accounts/{id}", s.handleGetAccount)
		r.Get("/accounts/{id}/balance", s.handleGetBalance)

		// Transfers
		r.Post("/transfers", s.handleInitiateTransfer)

		// Transactions (ledger history)
		r.Get("/transactions/{id}", s.handleGetTransaction)
		r.Get("/accounts/{id}/transactions", s.handleListAccountTransactions)

		// Webhook simulation (virtual account inflow)
		r.Post("/webhooks/inflow", s.handleInflowWebhook)
	})

	return r
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

// POST /v1/accounts
func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Currency string `json:"currency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierr.BadRequest("invalid JSON body").Write(w)
		return
	}
	if body.Name == "" || body.Type == "" {
		apierr.BadRequest("name and type are required").Write(w)
		return
	}

	acc, err := s.wallets.CreateAccount(r.Context(), wallet.CreateAccountInput{
		Name:     body.Name,
		Type:     wallet.AccountType(body.Type),
		Currency: body.Currency,
	})
	if err != nil {
		apierr.Unprocessable(err.Error()).Write(w)
		return
	}
	respond(w, http.StatusCreated, acc)
}

// GET /v1/accounts/{id}
func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, err := s.wallets.GetAccount(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			apierr.NotFound(fmt.Sprintf("account %q not found", id)).Write(w)
			return
		}
		apierr.Internal().Write(w)
		return
	}
	respond(w, http.StatusOK, acc)
}

// GET /v1/accounts/{id}/balance
func (s *Server) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bal, err := s.wallets.GetBalance(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			apierr.NotFound(fmt.Sprintf("account %q not found", id)).Write(w)
			return
		}
		apierr.Internal().Write(w)
		return
	}
	respond(w, http.StatusOK, bal)
}

// POST /v1/transfers
func (s *Server) handleInitiateTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FromAccountID string `json:"from_account_id"`
		ToAccountID   string `json:"to_account_id"`
		AmountKobo    int64  `json:"amount"`      // clients send kobo
		Currency      string `json:"currency"`
		Description   string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierr.BadRequest("invalid JSON body").Write(w)
		return
	}

	idempKey := r.Header.Get("Idempotency-Key")

	t, err := s.transfer.Initiate(r.Context(), transfer.InitiateInput{
		FromAccountID:  body.FromAccountID,
		ToAccountID:    body.ToAccountID,
		Amount:         money.Amount(body.AmountKobo),
		Currency:       body.Currency,
		Description:    body.Description,
		IdempotencyKey: idempKey,
	})
	if err != nil {
		if strings.Contains(err.Error(), "insufficient funds") {
			apierr.InsufficientFunds().Write(w)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			apierr.NotFound(err.Error()).Write(w)
			return
		}
		apierr.Unprocessable(err.Error()).Write(w)
		return
	}
	respond(w, http.StatusCreated, t)
}

// GET /v1/transactions/{id}
func (s *Server) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	txn, err := s.ledger.GetTransaction(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			apierr.NotFound(fmt.Sprintf("transaction %q not found", id)).Write(w)
			return
		}
		apierr.Internal().Write(w)
		return
	}
	respond(w, http.StatusOK, txn)
}

// GET /v1/accounts/{id}/transactions?after=<cursor>&limit=20
func (s *Server) handleListAccountTransactions(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	after := r.URL.Query().Get("after")   // cursor
	limitStr := r.URL.Query().Get("limit")

	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT DISTINCT t.id, t.reference, t.description, t.status, t.created_at
		FROM transactions t
		INNER JOIN ledger_entries le ON le.transaction_id = t.id
		WHERE le.account_id = $1
		  AND ($2 = '' OR t.id < $2)
		ORDER BY t.id DESC
		LIMIT $3`,
		accountID, after, limit,
	)
	if err != nil {
		apierr.Internal().Write(w)
		return
	}
	defer rows.Close()

	type row struct {
		ID          string    `json:"id"`
		Reference   string    `json:"reference"`
		Description string    `json:"description"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"created_at"`
	}

	var results []row
	var nextCursor string
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.ID, &item.Reference, &item.Description, &item.Status, &item.CreatedAt); err != nil {
			apierr.Internal().Write(w)
			return
		}
		results = append(results, item)
		nextCursor = item.ID // last item becomes the next cursor
	}

	respond(w, http.StatusOK, map[string]interface{}{
		"data":        results,
		"next_cursor": nextCursor,
		"has_more":    len(results) == limit,
	})
}

// POST /v1/webhooks/inflow
func (s *Server) handleInflowWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountID   string `json:"account_id"`
		AmountKobo  int64  `json:"amount"`
		Currency    string `json:"currency"`
		Reference   string `json:"reference"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierr.BadRequest("invalid JSON body").Write(w)
		return
	}

	txn, err := s.webhook.ProcessInflow(r.Context(), webhook.InflowInput{
		AccountID:   body.AccountID,
		Amount:      money.Amount(body.AmountKobo),
		Currency:    body.Currency,
		Reference:   body.Reference,
		Description: body.Description,
	})
	if err != nil {
		apierr.Unprocessable(err.Error()).Write(w)
		return
	}
	respond(w, http.StatusCreated, txn)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func respond(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
