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
	version, err := repository.LatestRevision(context.Background(), definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if version.Revision != 1 || version.DefinitionSHA256 == "" {
		t.Fatalf("unexpected file strategy version: %+v", version)
	}
}

func TestFileRepositoryRejectsUnsafeID(t *testing.T) {
	repository := FileRepository{Dir: filepath.Join(t.TempDir(), "strategies")}
	if _, err := repository.LoadByID(context.Background(), "../bad"); err == nil {
		t.Fatal("expected unsafe id to fail")
	}
}

func TestFileRepositoryRejectsMismatchedRawDefinition(t *testing.T) {
	repository := FileRepository{Dir: t.TempDir()}
	definition := validDefinition()
	other := definition
	other.Name = "Different"
	raw, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(context.Background(), definition, raw); err == nil {
		t.Fatal("expected mismatched raw strategy to fail")
	}
}

func TestDefinitionSHA256CanonicalizesJSONObjects(t *testing.T) {
	first := validDefinition()
	first.Indicators[0].Type = "boll"
	first.Indicators[0].Params = json.RawMessage(`{"period":20,"stddev":2}`)
	second := first
	second.Indicators = append([]IndicatorSpec(nil), first.Indicators...)
	second.Indicators[0].Params = json.RawMessage(`{"stddev":2,"period":20}`)

	firstHash, err := DefinitionSHA256(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := DefinitionSHA256(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("semantically identical definitions have different hashes: %s != %s", firstHash, secondHash)
	}
}
