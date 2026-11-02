package strategy

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestFileRepositoryLoadByID(t *testing.T) {
	dir := t.TempDir()
	definition := validDefinition()
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}

	repository := FileRepository{Dir: dir}
	if err := repository.Save(context.Background(), definition, raw); err != nil {
		t.Fatal(err)
	}

	loaded, err := repository.LoadByID(context.Background(), definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != definition.ID {
		t.Fatalf("loaded ID=%q, want %q", loaded.ID, definition.ID)
	}
}

func TestFileRepositoryRejectsUnsafeID(t *testing.T) {
	repository := FileRepository{Dir: filepath.Join(t.TempDir(), "strategies")}
	if _, err := repository.LoadByID(context.Background(), "../bad"); err == nil {
		t.Fatal("expected unsafe id to fail")
	}
}
