package backtest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/runstore"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func testManager(t *testing.T) (*Manager, *strategy.SQLiteRepository, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	repository, err := strategy.OpenSQLiteRepository(ctx, filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	def := testDefinition()
	if err := repository.Save(ctx, def, nil); err != nil {
		t.Fatal(err)
	}
	writeTestDataset(t, root, def)
	store, err := runstore.NewSQLite(ctx, repository.Database())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(ctx, NewService(repository, indicator.MustBuiltinRegistry(), filepath.Join(root, "market")), store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return manager, repository, root
}

func TestManagerArchivesPinnedInputAndReplaysWithoutLiveData(t *testing.T) {
	m, repository, root := testManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	record, err := m.Start(ctx, Request{StrategyID: testDefinition().ID})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	m.wg.Wait()
	raw, err := m.Result(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	var first Response
	if err := json.Unmarshal(raw, &first); err != nil {
		t.Fatal(err)
	}
	if first.RunID != record.ID || first.StrategyVersion.Revision != 1 {
		t.Fatalf("unexpected archive: %+v", first)
	}
	def := testDefinition()
	def.Account.InitialCash *= 2
	if err := repository.Save(context.Background(), def, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "market"), filepath.Join(root, "market-replaced")); err != nil {
		t.Fatal(err)
	}
	replayed, err := m.Replay(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	m.wg.Wait()
	replayRaw, err := m.Result(context.Background(), replayed.ID)
	if err != nil {
		t.Fatal(err)
	}
	var second Response
	if err := json.Unmarshal(replayRaw, &second); err != nil {
		t.Fatal(err)
	}
	if second.StrategyVersion != first.StrategyVersion || !reflect.DeepEqual(second.Report, first.Report) || !reflect.DeepEqual(second.Data, first.Data) {
		t.Fatal("replay changed when live strategy/dataset changed")
	}
	events, err := m.store.Events(context.Background(), record.ID)
	if err != nil || events[len(events)-1].Status != "completed" {
		t.Fatalf("missing lifecycle events: %+v %v", events, err)
	}
	// Artifact hashes are checked on every read.
	if _, err := repository.Database().Exec(`UPDATE backtest_run_artifacts SET payload='corrupt' WHERE run_id=? AND kind='result'`, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Result(context.Background(), record.ID); err == nil {
		t.Fatal("corrupt result accepted")
	}
}

func TestManagerCancellationCannotPublishCompletedResult(t *testing.T) {
	m, _, _ := testManager(t)
	// Hold both worker slots so cancellation deterministically precedes work.
	m.slots <- struct{}{}
	m.slots <- struct{}{}
	record, err := m.Start(context.Background(), Request{StrategyID: testDefinition().ID})
	if err != nil {
		t.Fatal(err)
	}
	record, err = m.Cancel(context.Background(), record.ID)
	if err != nil || record.Status != "cancelled" {
		t.Fatalf("cancel failed: %+v %v", record, err)
	}
	m.wg.Wait()
	<-m.slots
	<-m.slots
	if _, err := m.Result(context.Background(), record.ID); !errors.Is(err, runstore.ErrNotCompleted) {
		t.Fatalf("cancelled run has result: %v", err)
	}
	if err := m.store.Transition(context.Background(), record.ID, "completed", 100, "", []byte(`{}`)); !errors.Is(err, runstore.ErrTerminal) {
		t.Fatalf("terminal state overwritten: %v", err)
	}
}

func TestManagerRecoversInterruptedRunsAndPersistedResults(t *testing.T) {
	m, repository, _ := testManager(t)
	response, err := m.Run(context.Background(), Request{StrategyID: testDefinition().ID})
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	stale := runstore.Record{ID: "stale", StrategyID: testDefinition().ID, StrategyRevision: 1, Request: json.RawMessage(`{}`), Provenance: json.RawMessage(`{}`)}
	if err := m.store.Create(context.Background(), stale, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := m.store.Transition(context.Background(), stale.ID, "running", 25, "", nil); err != nil {
		t.Fatal(err)
	}
	store, err := runstore.NewSQLite(context.Background(), repository.Database())
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewManager(context.Background(), m.service, store)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	record, err := restarted.store.Get(context.Background(), stale.ID)
	if err != nil || record.Status != "interrupted" || record.Progress != 25 {
		t.Fatalf("bad recovery: %+v %v", record, err)
	}
	if _, err := restarted.Result(context.Background(), response.RunID); err != nil {
		t.Fatal("completed result lost on restart:", err)
	}
}

func TestPrepareRejectsChangedFileHashAndConflictingOverrides(t *testing.T) {
	m, _, root := testManager(t)
	def := testDefinition()
	prepared, err := m.service.Prepare(context.Background(), Request{StrategyID: def.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Data.Provenance.Files) == 0 || prepared.Data.Provenance.BarsSHA256 == "" {
		t.Fatal("missing content provenance")
	}
	file := filepath.Join(root, "market", def.Data.DatasetID, filepath.FromSlash(prepared.Data.Provenance.Files[0].Path))
	if err := os.WriteFile(file, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.service.Prepare(context.Background(), Request{StrategyID: def.ID}); err == nil {
		t.Fatal("changed file hash accepted")
	}
	if _, err := m.service.Prepare(context.Background(), Request{StrategyID: def.ID, DatasetID: "__sample__", RunConfig: &prepared.Config}); err == nil {
		t.Fatal("conflicting run overrides accepted")
	}
}
