package strategy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func OpenSQLiteRepository(ctx context.Context, path string) (*SQLiteRepository, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	repository := &SQLiteRepository{db: db}
	if err := repository.Init(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return repository, nil
}

func (r *SQLiteRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *SQLiteRepository) Init(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS strategies (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  raw_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`)
	return err
}

func (r *SQLiteRepository) LoadByID(ctx context.Context, id string) (Definition, error) {
	if !safeStrategyID.MatchString(id) {
		return Definition{}, fmt.Errorf("invalid strategy id %q", id)
	}
	var raw string
	if err := r.db.QueryRowContext(ctx, `SELECT raw_json FROM strategies WHERE id = ?`, id).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return Definition{}, fmt.Errorf("strategy %q not found", id)
		}
		return Definition{}, err
	}
	definition, err := Decode([]byte(raw))
	if err != nil {
		return Definition{}, err
	}
	if definition.ID != id {
		return Definition{}, fmt.Errorf("strategy row id %q does not match payload id %q", id, definition.ID)
	}
	return definition, nil
}

func (r *SQLiteRepository) Save(ctx context.Context, definition Definition, raw []byte) error {
	if !safeStrategyID.MatchString(definition.ID) {
		return fmt.Errorf("invalid strategy id %q", definition.ID)
	}
	if len(raw) == 0 {
		encoded, err := json.Marshal(definition)
		if err != nil {
			return err
		}
		raw = encoded
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO strategies (id, name, raw_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  raw_json = excluded.raw_json,
  updated_at = excluded.updated_at;
`, definition.ID, definition.Name, string(raw), now, now)
	return err
}

func (r *SQLiteRepository) ListIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM strategies ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
