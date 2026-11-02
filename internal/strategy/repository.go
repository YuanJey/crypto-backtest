package strategy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var safeStrategyID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type Repository interface {
	LoadByID(ctx context.Context, id string) (Definition, error)
	Save(ctx context.Context, definition Definition, raw []byte) error
	ListIDs(ctx context.Context) ([]string, error)
}

type FileRepository struct {
	Dir string
}

func (r FileRepository) LoadByID(ctx context.Context, id string) (Definition, error) {
	if !safeStrategyID.MatchString(id) {
		return Definition{}, fmt.Errorf("invalid strategy id %q", id)
	}
	if err := ctx.Err(); err != nil {
		return Definition{}, err
	}
	path := filepath.Join(r.Dir, id+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}
	definition, err := Decode(raw)
	if err != nil {
		return Definition{}, err
	}
	if definition.ID != id {
		return Definition{}, fmt.Errorf("strategy file id %q does not match requested id %q", definition.ID, id)
	}
	return definition, nil
}

func (r FileRepository) Save(ctx context.Context, definition Definition, raw []byte) error {
	if !safeStrategyID.MatchString(definition.ID) {
		return fmt.Errorf("invalid strategy id %q", definition.ID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.Dir, definition.ID+".json"), raw, 0o644)
}

func (r FileRepository) ListIDs(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		if safeStrategyID.MatchString(id) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
