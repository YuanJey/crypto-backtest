package runstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("backtest run not found")
var ErrTerminal = errors.New("backtest run is already terminal")
var ErrNotCompleted = errors.New("backtest run has no completed result")

type Record struct {
	ID               string          `json:"id"`
	StrategyID       string          `json:"strategy_id"`
	StrategyRevision int             `json:"strategy_revision"`
	Status           string          `json:"status"`
	Progress         int             `json:"progress"`
	Request          json.RawMessage `json:"request"`
	Provenance       json.RawMessage `json:"provenance"`
	Error            string          `json:"error,omitempty"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
	ResultSHA256     string          `json:"result_sha256,omitempty"`
}

type Event struct {
	Sequence  int    `json:"sequence"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

type Store interface {
	Create(context.Context, Record, []byte) error
	Get(context.Context, string) (Record, error)
	List(context.Context, string, int) ([]Record, error)
	Transition(context.Context, string, string, int, string, []byte) error
	Artifact(context.Context, string, string) ([]byte, error)
	Events(context.Context, string) ([]Event, error)
	Recover(context.Context) error
}

type SQLite struct{ db *sql.DB }

// Schema matches docs/backtest-runs-schema.sql. Migration is idempotent and
// never modifies strategy definitions or revisions.
const Schema = `
CREATE TABLE IF NOT EXISTS backtest_runs (
 id TEXT PRIMARY KEY,
 strategy_id TEXT NOT NULL,
 strategy_revision INTEGER NOT NULL CHECK (strategy_revision >= 1),
 status TEXT NOT NULL CHECK (status IN ('queued','running','completed','failed','cancelled','interrupted')),
 progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
 request_json TEXT NOT NULL,
 provenance_json TEXT NOT NULL,
 error TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 result_sha256 TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS backtest_runs_strategy_created ON backtest_runs(strategy_id, created_at DESC);
CREATE TABLE IF NOT EXISTS backtest_run_artifacts (
 run_id TEXT NOT NULL REFERENCES backtest_runs(id),
 kind TEXT NOT NULL CHECK (kind IN ('input','result')),
 sha256 TEXT NOT NULL,
 payload BLOB NOT NULL,
 PRIMARY KEY (run_id, kind)
);
CREATE TABLE IF NOT EXISTS backtest_run_events (
 run_id TEXT NOT NULL REFERENCES backtest_runs(id),
 sequence INTEGER NOT NULL,
 status TEXT NOT NULL,
 progress INTEGER NOT NULL,
 message TEXT NOT NULL,
 created_at TEXT NOT NULL,
 PRIMARY KEY (run_id, sequence)
);`

func NewSQLite(ctx context.Context, db *sql.DB) (*SQLite, error) {
	if _, err := db.ExecContext(ctx, Schema); err != nil {
		return nil, err
	}
	return &SQLite{db: db}, nil
}

func now() string            { return time.Now().UTC().Format(time.RFC3339Nano) }
func Hash(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }

func (s *SQLite) Create(ctx context.Context, record Record, input []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stamp := now()
	_, err = tx.ExecContext(ctx, `INSERT INTO backtest_runs
	(id,strategy_id,strategy_revision,status,request_json,provenance_json,created_at,updated_at)
	VALUES (?,?,?,'queued',?,?,?,?)`, record.ID, record.StrategyID, record.StrategyRevision,
		string(record.Request), string(record.Provenance), stamp, stamp)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO backtest_run_artifacts VALUES (?,'input',?,?)`,
		record.ID, Hash(input), input); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO backtest_run_events VALUES (?,1,'queued',0,'',?)`, record.ID, stamp); err != nil {
		return err
	}
	return tx.Commit()
}

const columns = "id,strategy_id,strategy_revision,status,progress,request_json,provenance_json,error,created_at,updated_at,result_sha256"

type scanner interface{ Scan(...any) error }

func scanRecord(row scanner) (Record, error) {
	var record Record
	var request, provenance string
	err := row.Scan(&record.ID, &record.StrategyID, &record.StrategyRevision, &record.Status, &record.Progress,
		&request, &provenance, &record.Error, &record.CreatedAt, &record.UpdatedAt, &record.ResultSHA256)
	if err == sql.ErrNoRows {
		err = ErrNotFound
	}
	record.Request, record.Provenance = json.RawMessage(request), json.RawMessage(provenance)
	return record, err
}

func (s *SQLite) Get(ctx context.Context, id string) (Record, error) {
	return scanRecord(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM backtest_runs WHERE id=?`, id))
}

func (s *SQLite) List(ctx context.Context, strategyID string, limit int) ([]Record, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM backtest_runs
	WHERE (?='' OR strategy_id=?) ORDER BY created_at DESC LIMIT ?`, strategyID, strategyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]Record, 0)
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func terminal(status string) bool { return status != "queued" && status != "running" }

// Transition publishes state, events and result in one transaction. Terminal
// states are immutable, including concurrent cancellation/completion.
func (s *SQLite) Transition(ctx context.Context, id, status string, progress int, message string, result []byte) error {
	if progress < 0 || progress > 100 {
		return fmt.Errorf("invalid run progress")
	}
	switch status {
	case "running", "completed", "cancelled", "failed", "interrupted":
	default:
		return fmt.Errorf("invalid run transition")
	}
	if (status == "completed") != (len(result) > 0) {
		return fmt.Errorf("completed transition requires a result")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	record, err := scanRecord(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM backtest_runs WHERE id=?`, id))
	if err != nil {
		return err
	}
	if terminal(record.Status) {
		return ErrTerminal
	}
	if progress < record.Progress {
		progress = record.Progress
	}
	hash := ""
	if status == "completed" {
		progress, hash = 100, Hash(result)
		if _, err := tx.ExecContext(ctx, `INSERT INTO backtest_run_artifacts VALUES (?,'result',?,?)`, id, hash, result); err != nil {
			return err
		}
	}
	stamp := now()
	if _, err := tx.ExecContext(ctx, `UPDATE backtest_runs SET status=?,progress=?,error=?,updated_at=?,result_sha256=? WHERE id=?`,
		status, progress, message, stamp, hash, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO backtest_run_events
	SELECT ?,COALESCE(MAX(sequence),0)+1,?,?,?,? FROM backtest_run_events WHERE run_id=?`,
		id, status, progress, message, stamp, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) Artifact(ctx context.Context, id, kind string) ([]byte, error) {
	record, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if kind == "result" && record.Status != "completed" {
		return nil, ErrNotCompleted
	}
	var raw []byte
	var hash string
	if err := s.db.QueryRowContext(ctx, `SELECT payload,sha256 FROM backtest_run_artifacts WHERE run_id=? AND kind=?`, id, kind).Scan(&raw, &hash); err != nil {
		return nil, err
	}
	if Hash(raw) != hash || (kind == "result" && hash != record.ResultSHA256) {
		return nil, fmt.Errorf("backtest artifact hash mismatch")
	}
	return raw, nil
}

func (s *SQLite) Events(ctx context.Context, id string) ([]Event, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,status,progress,message,created_at FROM backtest_run_events WHERE run_id=? ORDER BY sequence`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.Sequence, &event.Status, &event.Progress, &event.Message, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *SQLite) Recover(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM backtest_runs WHERE status IN ('queued','running')`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.Transition(ctx, id, "interrupted", 0, "server restarted before completion", nil); err != nil && !errors.Is(err, ErrTerminal) {
			return err
		}
	}
	return nil
}
