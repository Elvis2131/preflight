// This file is PC-95's core: a stable, structured failure signal a caller can branch
// on programmatically, alongside the plain `error` interface every function in this
// package already returns. Surfaced during the proactive API audit run after PC-89
// closed: server/http.go collapsed session-not-found, malformed input, an
// insufficient-data bundle, and an invalid workload into one generic 400 Bad Request,
// distinguished only by grepping the error string — never questioned before a browser
// client (the canvas) existed to actually need to branch on it, the same root pattern
// as node_loss/CORS/inline-workload/nil-slice-to-JSON-null before it.
//
// Deliberately minimal, per this ticket's own Conversation ("not a full
// error-taxonomy redesign"): one type, one constructor, no hierarchy. A handler that
// gets back a plain (non-*APIError) error still falls back to today's generic 400 —
// this is additive, not a breaking change to any existing error string a CLI/test
// caller might already grep.
package server

import "fmt"

// APIError carries a stable, machine-readable Code and the HTTP Status a handler
// should respond with, alongside the same human-readable Message this codebase's
// plain errors already produced. Error() returns exactly Message — a caller doing
// err.Error() (logging, an existing test's string-matching assertion) sees identical
// text to before; Code/Status are ADDITIVE information a structured caller can read
// via errors.As, not a replacement for the message text.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Message }

func newAPIError(status int, code, format string, args ...any) *APIError {
	return &APIError{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}
