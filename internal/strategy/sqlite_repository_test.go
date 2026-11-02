package strategy

import (
	"context"
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
