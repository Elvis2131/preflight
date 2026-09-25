package pricing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // same pure-Go driver as server.Store (ADR-002)
)

// Store is a SQLite-backed (WAL mode, ADR-002) pricing snapshot store — its own
// schema/file, separate from server.Store's session/version tables (ADR-006 §4), but
// the same storage technology and WAL discipline.
type Store struct {
	db *sql.DB
}

// OpenStore opens (creating if necessary) a SQLite database at path and ensures its
// schema exists. path may be ":memory:" for an ephemeral, test-only store.
func OpenStore(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("pricing: open sqlite store: %w", err)
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
CREATE TABLE IF NOT EXISTS pricing_snapshots (
	id           TEXT PRIMARY KEY,
	fetched_at   TEXT NOT NULL,
	source       TEXT NOT NULL,
	disclaimer   TEXT NOT NULL,
	entries_json TEXT NOT NULL,
	active       INTEGER NOT NULL DEFAULT 0
);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("pricing: migrate schema: %w", err)
	}
	return nil
}

// PutSnapshot persists a snapshot (insert or replace by ID) — never mutated partially;
// a snapshot is a single immutable document once fetched, the same "author once,
// re-read many times" discipline server.Store already uses for IR versions.
func (s *Store) PutSnapshot(snap Snapshot) error {
	entriesJSON, err := json.Marshal(snap.Entries)
	if err != nil {
		return fmt.Errorf("pricing: marshal entries for snapshot %s: %w", snap.ID, err)
	}
	active := 0
	if snap.Active {
		active = 1
	}
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO pricing_snapshots (id, fetched_at, source, disclaimer, entries_json, active) VALUES (?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.FetchedAt.Format("2006-01-02T15:04:05.000Z07:00"), snap.Source, snap.Disclaimer, string(entriesJSON), active,
	)
	if err != nil {
		return fmt.Errorf("pricing: put snapshot %s: %w", snap.ID, err)
	}
	return nil
}

// ListSnapshots returns every stored snapshot's metadata (no entries — a caller
// wanting the full price table calls GetSnapshot by ID), oldest first.
func (s *Store) ListSnapshots() ([]Snapshot, error) {
	rows, err := s.db.Query(`SELECT id, fetched_at, source, disclaimer, active FROM pricing_snapshots ORDER BY fetched_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("pricing: list snapshots: %w", err)
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		var snap Snapshot
		var fetchedAt string
		var active int
		if err := rows.Scan(&snap.ID, &fetchedAt, &snap.Source, &snap.Disclaimer, &active); err != nil {
			return nil, fmt.Errorf("pricing: scan snapshot row: %w", err)
		}
		snap.FetchedAt, _ = parseTimestamp(fetchedAt)
		snap.Active = active != 0
		out = append(out, snap)
	}
	return out, rows.Err()
}

// GetSnapshot returns one full snapshot (metadata + entries) by ID.
func (s *Store) GetSnapshot(id string) (Snapshot, bool, error) {
	var snap Snapshot
	var fetchedAt, entriesJSON string
	var active int
	err := s.db.QueryRow(`SELECT id, fetched_at, source, disclaimer, entries_json, active FROM pricing_snapshots WHERE id = ?`, id).
		Scan(&snap.ID, &fetchedAt, &snap.Source, &snap.Disclaimer, &entriesJSON, &active)
	if err == sql.ErrNoRows {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("pricing: get snapshot %s: %w", id, err)
	}
	snap.FetchedAt, _ = parseTimestamp(fetchedAt)
	snap.Active = active != 0
	if err := json.Unmarshal([]byte(entriesJSON), &snap.Entries); err != nil {
		return Snapshot{}, false, fmt.Errorf("pricing: unmarshal entries for snapshot %s: %w", id, err)
	}
	return snap, true, nil
}

// SetActive marks id as the one active default snapshot, atomically clearing any
// previously-active one — exactly one snapshot is ever active at a time (or zero,
// before any activation has happened).
func (s *Store) SetActive(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("pricing: set active %s: begin tx: %w", id, err)
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM pricing_snapshots WHERE id = ?`, id).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("pricing: set active: no such snapshot %s", id)
		}
		return fmt.Errorf("pricing: set active %s: %w", id, err)
	}
	if _, err := tx.Exec(`UPDATE pricing_snapshots SET active = 0`); err != nil {
		return fmt.Errorf("pricing: set active %s: clear previous: %w", id, err)
	}
	if _, err := tx.Exec(`UPDATE pricing_snapshots SET active = 1 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("pricing: set active %s: %w", id, err)
	}
	return tx.Commit()
}

// ActiveSnapshot returns the one currently-active snapshot, if any.
func (s *Store) ActiveSnapshot() (Snapshot, bool, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM pricing_snapshots WHERE active = 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("pricing: active snapshot: %w", err)
	}
	return s.GetSnapshot(id)
}

func parseTimestamp(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05.000Z07:00", s)
}
