// Package server is the net/http + MCP adapter (P1) and session store — CLAUDE.md §6
// places "session store" inside this package specifically, not a standalone module,
// which is why PC-21 (this file, plus assess.go/http.go/mcp.go) owns wiring ADR-002's
// SQLite storage rather than treating it as a separate prerequisite: PC-9's own
// remaining criterion ("SQLite confirmed as the storage backend in the actual
// codebase") is satisfied by this package actually using it, not duplicated as a
// standalone effort. See PC-21/PC-9's Jira history for that ownership decision,
// recorded before this file was written, not assumed silently.
//
// This package is explicitly OUTSIDE core/ and is exactly where I/O belongs — I1 only
// binds core/, never server/.
package server

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go driver, registered under "sqlite" — no CGO, simpler cross-platform builds than mattn/go-sqlite3

	"preflight/core"
	"preflight/pricing"
)

// Store is a SQLite-backed (WAL mode, ADR-002) session/version store. A session is one
// architecture's ongoing author->evaluate->modify->re-evaluate loop (CLAUDE.md §3);
// each /assess call against a session produces the next VersionNumber (PRD §4: "every
// server call against a session produces Version N").
type Store struct {
	db *sql.DB

	// pricingStore is PC-117's own attachment point — nil unless AttachPricingStore
	// is called (cmd/assessd/main.go, after opening both stores). A nil pricingStore
	// means "no pricing configured," never an error: /assess must keep working for
	// every existing caller/test that never wires one up at all (this field's own
	// zero value, unchanged Store{db: db} construction everywhere else). pricing.Store
	// itself has zero network capability (pricing/boundary_test.go), so holding a
	// reference to it here adds no network capability to assessd (P1).
	pricingStore *pricing.Store
}

// AttachPricingStore wires PC-117's cost computation into this Store — /assess reads
// pricing.Store directly (a local SQLite read, never a network call) to resolve
// either a caller-pinned or the active snapshot. Optional: a Store with none attached
// behaves exactly as it did before PC-117 (Cost stays nil in every response).
func (s *Store) AttachPricingStore(ps *pricing.Store) { s.pricingStore = ps }

// OpenStore opens (creating if necessary) a SQLite database at path and ensures its
// schema exists. path may be ":memory:" for an ephemeral, test-only store.
func OpenStore(path string) (*Store, error) {
	// WAL mode per ADR-002 ("SQLite in WAL mode"); _pragma sets are modernc.org/sqlite's
	// DSN convention for applying PRAGMAs at connection time.
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("server: open sqlite store: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS versions (
	session_id     TEXT NOT NULL REFERENCES sessions(id),
	version_number INTEGER NOT NULL,
	ir_json        TEXT NOT NULL,
	findings_json  TEXT NOT NULL,
	scorecard_json TEXT NOT NULL,
	workload_json  TEXT NOT NULL DEFAULT '{}',
	cost_json      TEXT NOT NULL DEFAULT '',
	created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	PRIMARY KEY (session_id, version_number)
);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("server: migrate schema: %w", err)
	}
	// PC-82: workload_json was added after this table already existed in some
	// deployments — CREATE TABLE IF NOT EXISTS above does nothing for an
	// already-existing table, so an already-existing on-disk database needs the
	// column added explicitly. Error ignored: SQLite errors if the column already
	// exists, which is the expected, harmless case on every run after the first.
	s.db.Exec(`ALTER TABLE versions ADD COLUMN workload_json TEXT NOT NULL DEFAULT '{}'`)
	// PC-117: same reasoning, for cost_json — an empty string means "no cost was
	// computed for this version" (no pricing store was attached at assess time),
	// distinct from a real, marshaled core.CostReport.
	s.db.Exec(`ALTER TABLE versions ADD COLUMN cost_json TEXT NOT NULL DEFAULT ''`)
	return nil
}

// EnsureSession creates sessionID if it does not already exist. Idempotent — a caller
// need not track "is this the first call for this session" separately.
func (s *Store) EnsureSession(sessionID string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO sessions (id) VALUES (?)`, sessionID)
	if err != nil {
		return fmt.Errorf("server: ensure session %s: %w", sessionID, err)
	}
	return nil
}

// SessionExists reports whether sessionID has ever been created (EnsureSession or a
// prior producer call). PC-95 needs this distinction — /simulate must tell "no such
// session at all" (404, session_not_found) apart from "this session exists, but not
// that version number" (404, version_not_found) — which GetVersion's own single
// existence check can't do on its own, since a plain WHERE session_id=? AND
// version_number=? query returns "not found" identically in both cases.
func (s *Store) SessionExists(sessionID string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("server: check session %s exists: %w", sessionID, err)
	}
	return true, nil
}

// StoredVersion is one persisted version's content — the same three artifacts
// cmd/gen-golden-fixtures produces for the golden fixtures, now persisted per real
// session instead of only ever existing as checked-in files.
type StoredVersion struct {
	SessionID     string
	VersionNumber int
	IR            *core.IR
	Findings      []core.Finding
	Scorecard     core.Scorecard

	// Workload is persisted per-version (PC-82) so /simulate can operate on
	// {version, faults} alone, per its own stated request shape — without this, a
	// caller would need to re-supply workload_path just to re-derive
	// region/capacity facts /assess already had when it built this version.
	Workload core.Workload

	// Cost is PC-117's own acceptance criterion made real: "Assessment records the
	// snapshot ID used; the assess and read-back responses include it." Nil means no
	// pricing store was attached when this version was assessed — not an error, and
	// not the same thing as "priced at $0."
	Cost *core.CostReport
}

// StoreVersion persists v. VersionNumber must be exactly one greater than the current
// latest version for this session (enforced by the caller via NextVersionNumber, not
// re-derived here — this function trusts its input, consistent with core's own
// "counting is a different function from the thing that uses the count" separation,
// PC-14's SurvivingCapacity precedent).
func (s *Store) StoreVersion(v StoredVersion) error {
	irJSON, err := json.Marshal(v.IR)
	if err != nil {
		return fmt.Errorf("server: marshal IR: %w", err)
	}
	findingsJSON, err := json.Marshal(v.Findings)
	if err != nil {
		return fmt.Errorf("server: marshal findings: %w", err)
	}
	scorecardJSON, err := json.Marshal(v.Scorecard)
	if err != nil {
		return fmt.Errorf("server: marshal scorecard: %w", err)
	}
	workloadJSON, err := json.Marshal(v.Workload)
	if err != nil {
		return fmt.Errorf("server: marshal workload: %w", err)
	}
	var costJSON string
	if v.Cost != nil {
		b, err := json.Marshal(v.Cost)
		if err != nil {
			return fmt.Errorf("server: marshal cost: %w", err)
		}
		costJSON = string(b)
	}
	_, err = s.db.Exec(
		`INSERT INTO versions (session_id, version_number, ir_json, findings_json, scorecard_json, workload_json, cost_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.SessionID, v.VersionNumber, string(irJSON), string(findingsJSON), string(scorecardJSON), string(workloadJSON), costJSON,
	)
	if err != nil {
		return fmt.Errorf("server: store version %d for session %s: %w", v.VersionNumber, v.SessionID, err)
	}
	return nil
}

// LatestVersion returns the highest VersionNumber stored for sessionID, and whether
// any version exists at all (a brand-new session has none — the caller's first
// /assess call produces Version 1, with no prior version to diff against).
func (s *Store) LatestVersion(sessionID string) (StoredVersion, bool, error) {
	row := s.db.QueryRow(
		`SELECT version_number, ir_json, findings_json, scorecard_json, workload_json, cost_json FROM versions
		 WHERE session_id = ? ORDER BY version_number DESC LIMIT 1`,
		sessionID,
	)
	return scanVersion(row, sessionID)
}

// GetVersion returns a specific version by number.
func (s *Store) GetVersion(sessionID string, versionNumber int) (StoredVersion, bool, error) {
	row := s.db.QueryRow(
		`SELECT version_number, ir_json, findings_json, scorecard_json, workload_json, cost_json FROM versions
		 WHERE session_id = ? AND version_number = ?`,
		sessionID, versionNumber,
	)
	return scanVersion(row, sessionID)
}

func scanVersion(row *sql.Row, sessionID string) (StoredVersion, bool, error) {
	var (
		versionNumber                                               int
		irJSON, findingsJSON, scorecardJSON, workloadJSON, costJSON string
	)
	if err := row.Scan(&versionNumber, &irJSON, &findingsJSON, &scorecardJSON, &workloadJSON, &costJSON); err != nil {
		if err == sql.ErrNoRows {
			return StoredVersion{}, false, nil
		}
		return StoredVersion{}, false, fmt.Errorf("server: scan version for session %s: %w", sessionID, err)
	}

	var ir core.IR
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return StoredVersion{}, false, fmt.Errorf("server: unmarshal stored IR: %w", err)
	}
	var findings []core.Finding
	if err := json.Unmarshal([]byte(findingsJSON), &findings); err != nil {
		return StoredVersion{}, false, fmt.Errorf("server: unmarshal stored findings: %w", err)
	}
	var scorecard core.Scorecard
	if err := json.Unmarshal([]byte(scorecardJSON), &scorecard); err != nil {
		return StoredVersion{}, false, fmt.Errorf("server: unmarshal stored scorecard: %w", err)
	}
	var workload core.Workload
	if workloadJSON != "" && workloadJSON != "{}" {
		if err := json.Unmarshal([]byte(workloadJSON), &workload); err != nil {
			return StoredVersion{}, false, fmt.Errorf("server: unmarshal stored workload: %w", err)
		}
	}
	var cost *core.CostReport
	if costJSON != "" {
		var c core.CostReport
		if err := json.Unmarshal([]byte(costJSON), &c); err != nil {
			return StoredVersion{}, false, fmt.Errorf("server: unmarshal stored cost: %w", err)
		}
		cost = &c
	}

	return StoredVersion{
		SessionID: sessionID, VersionNumber: versionNumber,
		IR: &ir, Findings: findings, Scorecard: scorecard, Workload: workload, Cost: cost,
	}, true, nil
}
