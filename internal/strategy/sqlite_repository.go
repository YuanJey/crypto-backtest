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

// Database supplies the shared connection to independently owned persistence
// modules. Strategy operations do not depend on run storage.
func (r *SQLiteRepository) Database() *sql.DB { return r.db }

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
	// Keep :memory: databases and concurrent strategy/run transactions on one
	// connection. This also avoids independent writers racing SQLite locks.
	db.SetMaxOpenConns(1)
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
	if _, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS strategies (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  raw_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS strategy_revisions (
  strategy_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  name TEXT NOT NULL,
  raw_json TEXT NOT NULL,
  definition_sha256 TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (strategy_id, revision)
);
`); err != nil {
		return err
	}
	return r.backfillRevisions(ctx)
}

func (r *SQLiteRepository) backfillRevisions(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, raw_json, created_at
FROM strategies
WHERE NOT EXISTS (
  SELECT 1 FROM strategy_revisions WHERE strategy_id = strategies.id
)
`)
	if err != nil {
		return err
	}
	type legacyStrategy struct {
		id        string
		name      string
		raw       string
		createdAt string
	}
	var items []legacyStrategy
	for rows.Next() {
		var item legacyStrategy
		if err := rows.Scan(&item.id, &item.name, &item.raw, &item.createdAt); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range items {
		definition, err := Decode([]byte(item.raw))
		if err != nil {
			return fmt.Errorf("decode legacy strategy %q: %w", item.id, err)
		}
		hash, err := DefinitionSHA256(definition)
		if err != nil {
			return fmt.Errorf("hash legacy strategy %q: %w", item.id, err)
		}
		if _, err := r.db.ExecContext(ctx, `
INSERT OR IGNORE INTO strategy_revisions (
  strategy_id, revision, name, raw_json, definition_sha256, created_at
) VALUES (?, 1, ?, ?, ?, ?)
`, item.id, item.name, item.raw, hash, item.createdAt); err != nil {
			return err
		}
	}
	return nil
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

func (r *SQLiteRepository) LoadRevision(ctx context.Context, id string, revision int) (Definition, RevisionMetadata, error) {
	if !safeStrategyID.MatchString(id) {
		return Definition{}, RevisionMetadata{}, fmt.Errorf("invalid strategy id %q", id)
	}
	if revision < 1 {
		return Definition{}, RevisionMetadata{}, fmt.Errorf("strategy revision must be >= 1")
	}
	var raw string
	var hash string
	err := r.db.QueryRowContext(ctx, `
SELECT raw_json, definition_sha256
FROM strategy_revisions
WHERE strategy_id = ? AND revision = ?
`, id, revision).Scan(&raw, &hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return Definition{}, RevisionMetadata{}, fmt.Errorf("strategy %q revision %d not found", id, revision)
		}
		return Definition{}, RevisionMetadata{}, err
	}
	definition, err := Decode([]byte(raw))
	if err != nil {
		return Definition{}, RevisionMetadata{}, err
	}
	if definition.ID != id {
		return Definition{}, RevisionMetadata{}, fmt.Errorf("strategy row id %q does not match payload id %q", id, definition.ID)
	}
	return definition, RevisionMetadata{
		Revision:         revision,
		DefinitionSHA256: hash,
	}, nil
}

func (r *SQLiteRepository) LatestRevision(ctx context.Context, id string) (RevisionMetadata, error) {
	if !safeStrategyID.MatchString(id) {
		return RevisionMetadata{}, fmt.Errorf("invalid strategy id %q", id)
	}
	var metadata RevisionMetadata
	err := r.db.QueryRowContext(ctx, `
SELECT revision, definition_sha256
FROM strategy_revisions
WHERE strategy_id = ?
ORDER BY revision DESC
LIMIT 1
`, id).Scan(&metadata.Revision, &metadata.DefinitionSHA256)
	if err != nil {
		if err == sql.ErrNoRows {
			return RevisionMetadata{}, fmt.Errorf("strategy %q not found", id)
		}
		return RevisionMetadata{}, err
	}
	return metadata, nil
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
	if err := validateRawDefinition(definition, raw); err != nil {
		return err
	}
	hash, err := DefinitionSHA256(definition)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var latestRevision int
	var latestHash string
	err = tx.QueryRowContext(ctx, `
SELECT revision, definition_sha256
FROM strategy_revisions
WHERE strategy_id = ?
ORDER BY revision DESC
LIMIT 1
`, definition.ID).Scan(&latestRevision, &latestHash)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows || latestHash != hash {
		latestRevision++
		if _, err := tx.ExecContext(ctx, `
INSERT INTO strategy_revisions (
  strategy_id, revision, name, raw_json, definition_sha256, created_at
) VALUES (?, ?, ?, ?, ?, ?)
`, definition.ID, latestRevision, definition.Name, string(raw), hash, now); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO strategies (id, name, raw_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  raw_json = excluded.raw_json,
  updated_at = excluded.updated_at;
`, definition.ID, definition.Name, string(raw), now, now); err != nil {
		return err
	}
	return tx.Commit()
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
