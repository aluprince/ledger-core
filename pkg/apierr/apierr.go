// Package apierr provides typed error responses for the API.
// All errors returned to clients use this structure — never raw Go errors.
package apierr

import (
	"encoding/json"
	"net/http"
)

// Error codes — stable string identifiers for client error handling.
const (
	CodeNotFound            = "NOT_FOUND"
	CodeBadRequest          = "BAD_REQUEST"
	CodeUnprocessable       = "UNPROCESSABLE"
	CodeInsufficientFunds   = "INSUFFICIENT_FUNDS"
	CodeDuplicateIdempotency = "DUPLICATE_IDEMPOTENCY_KEY"
	CodeInternal            = "INTERNAL_ERROR"
	CodeUnauthorized        = "UNAUTHORIZED"
	CodeConflict            = "CONFLICT"
)

// APIError is the standard error envelope returned to clients.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *APIError) Error() string {
	return e.Message
}

// Write writes the error as JSON to the response writer.
func (e *APIError) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(e)
}

// Constructors

func NotFound(msg string) *APIError {
	return &APIError{Code: CodeNotFound, Message: msg, Status: http.StatusNotFound}
}

func BadRequest(msg string) *APIError {
	return &APIError{Code: CodeBadRequest, Message: msg, Status: http.StatusBadRequest}
}

func Unprocessable(msg string) *APIError {
	return &APIError{Code: CodeUnprocessable, Message: msg, Status: http.StatusUnprocessableEntity}
}

func InsufficientFunds() *APIError {
	return &APIError{Code: CodeInsufficientFunds, Message: "insufficient funds", Status: http.StatusUnprocessableEntity}
}

func DuplicateIdempotency() *APIError {
	return &APIError{Code: CodeDuplicateIdempotency, Message: "idempotency key already used with different parameters", Status: http.StatusConflict}
}

func Internal() *APIError {
	return &APIError{Code: CodeInternal, Message: "an internal error occurred", Status: http.StatusInternalServerError}
}

func Unauthorized() *APIError {
	return &APIError{Code: CodeUnauthorized, Message: "unauthorized", Status: http.StatusUnauthorized}
}
