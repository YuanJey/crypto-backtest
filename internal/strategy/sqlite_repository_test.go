package strategy

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSQLiteRepositorySavesAndLoadsStrategy(t *testing.T) {
	repository, err := OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	definition := validDefinition()
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(context.Background(), definition, raw); err != nil {
		t.Fatal(err)
	}
	ids, err := repository.ListIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != definition.ID {
		t.Fatalf("ids=%v, want [%s]", ids, definition.ID)
	}
	loaded, err := repository.LoadByID(context.Background(), definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != definition.ID || loaded.Name != definition.Name {
		t.Fatalf("loaded=%+v, want id/name from definition", loaded)
	}
}

func TestSQLiteRepositoryKeepsImmutableRevisions(t *testing.T) {
	ctx := context.Background()
	repository, err := OpenSQLiteRepository(ctx, filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	first := validDefinition()
	firstRaw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, first, firstRaw); err != nil {
		t.Fatal(err)
	}
	firstVersion, err := repository.LatestRevision(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}

	second := first
	second.Name = "Updated strategy"
	secondRaw, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, second, secondRaw); err != nil {
		t.Fatal(err)
	}
	secondVersion, err := repository.LatestRevision(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstVersion.Revision != 1 || secondVersion.Revision != 2 {
		t.Fatalf("unexpected revisions: first=%+v second=%+v", firstVersion, secondVersion)
	}
	if firstVersion.DefinitionSHA256 == secondVersion.DefinitionSHA256 {
		t.Fatal("expected changed definition to have a new hash")
	}

	loadedFirst, loadedVersion, err := repository.LoadRevision(ctx, first.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if loadedFirst.Name != first.Name || loadedVersion != firstVersion {
		t.Fatalf("revision 1 changed: definition=%+v version=%+v", loadedFirst, loadedVersion)
	}

	if err := repository.Save(ctx, second, secondRaw); err != nil {
		t.Fatal(err)
	}
	unchangedVersion, err := repository.LatestRevision(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchangedVersion.Revision != 2 {
		t.Fatalf("saving identical definition created revision %d, want 2", unchangedVersion.Revision)
	}
}

func TestSQLiteRepositoryBackfillsLegacyStrategies(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
CREATE TABLE strategies (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  raw_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	definition := validDefinition()
	raw, err := json.Marshal(definition)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO strategies (id, name, raw_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
`, definition.ID, definition.Name, string(raw), "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	repository, err := OpenSQLiteRepository(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	version, err := repository.LatestRevision(ctx, definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if version.Revision != 1 || version.DefinitionSHA256 == "" {
		t.Fatalf("legacy strategy was not backfilled: %+v", version)
	}
}
