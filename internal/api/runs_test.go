package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/YuanJey/crypto-backtest/internal/backtest"
	"github.com/YuanJey/crypto-backtest/internal/engine"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/runstore"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func runAPI(t *testing.T) (*Server, *strategy.SQLiteRepository) {
	t.Helper()
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	definition := strategySummaryFixture()
	definition.Data.DatasetID = "__sample__"
	definition.Data.End = definition.Data.Start.Add(24 * time.Hour)
	if err := repository.Save(context.Background(), definition, nil); err != nil {
		t.Fatal(err)
	}
	server := NewServer(repository, indicator.MustBuiltinRegistry(), t.TempDir())
	t.Cleanup(server.Close)
	return server, repository
}

func callRunAPI(t *testing.T, server *Server, method, path string, body any, status int, target any) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(method, path, bytes.NewReader(raw)))
	if recorder.Code != status {
		t.Fatalf("%s %s: %d %s", method, path, recorder.Code, recorder.Body.String())
	}
	if target != nil {
		if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
			t.Fatal(err)
		}
	}
}

func awaitRunAPI(t *testing.T, server *Server, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var record runstore.Record
		callRunAPI(t, server, "GET", "/api/v1/backtest-runs/"+id, nil, 200, &record)
		if record.Status == "completed" {
			return
		}
		if record.Status != "queued" && record.Status != "running" {
			t.Fatalf("unexpected run: %+v", record)
		}
		runtime.Gosched()
	}
	t.Fatal("run did not finish")
}

func TestRunAPIFixesRevisionOwnsConfigAndReadsArchivedSnapshot(t *testing.T) {
	server, repository := runAPI(t)
	var snapshot struct {
		Definition strategy.Definition       `json:"definition"`
		Version    strategy.RevisionMetadata `json:"strategy_version"`
		Config     strategy.RunConfig        `json:"default_run_config"`
	}
	callRunAPI(t, server, "GET", "/api/v1/strategies/ema-boll-rsi/snapshot", nil, 200, &snapshot)
	updated := snapshot.Definition
	updated.Account.InitialCash = 50000
	updated.Name = "New revision"
	if err := repository.Save(context.Background(), updated, nil); err != nil {
		t.Fatal(err)
	}
	snapshot.Config.Account.InitialCash = 25000
	snapshot.Config.Risk.MaxSymbolEquityPct = 0.3
	snapshot.Config.Risk.MaxTotalExposurePct = 0.4
	var record runstore.Record
	callRunAPI(t, server, "POST", "/api/v1/backtest-runs", backtest.Request{
		StrategyID: snapshot.Definition.ID, StrategyRevision: snapshot.Version.Revision,
		RunConfig: &snapshot.Config, Report: engine.DefaultReportSpec(),
	}, 202, &record)
	awaitRunAPI(t, server, record.ID)
	var response backtest.Response
	callRunAPI(t, server, "GET", "/api/v1/backtest-runs/"+record.ID+"/result", nil, 200, &response)
	if response.StrategyVersion != snapshot.Version || response.Strategy.InitialCash != 25000 ||
		!reflect.DeepEqual(response.RunConfig, snapshot.Config) {
		t.Fatalf("pinned revision or experiment config changed: %+v", response)
	}
	var input backtest.RunInput
	callRunAPI(t, server, "GET", "/api/v1/backtest-runs/"+record.ID+"/snapshot", nil, 200, &input)
	if len(input.Data.Bars) != 96 || input.Version != snapshot.Version || input.Config.Account.InitialCash != 25000 {
		t.Fatalf("wrong snapshot: %+v", input)
	}
	var events struct {
		Events []runstore.Event `json:"events"`
	}
	callRunAPI(t, server, "GET", "/api/v1/backtest-runs/"+record.ID+"/events", nil, 200, &events)
	if events.Events[len(events.Events)-1].Status != "completed" {
		t.Fatal("missing terminal event")
	}
	var replay runstore.Record
	callRunAPI(t, server, "POST", "/api/v1/backtest-runs/"+record.ID+"/replay", nil, 202, &replay)
	awaitRunAPI(t, server, replay.ID)
	var replayResponse backtest.Response
	callRunAPI(t, server, "GET", "/api/v1/backtest-runs/"+replay.ID+"/result", nil, 200, &replayResponse)
	if !reflect.DeepEqual(response.Report, replayResponse.Report) || replayResponse.StrategyVersion != snapshot.Version {
		t.Fatal("archived replay changed")
	}
	var cancelled runstore.Record
	callRunAPI(t, server, "POST", "/api/v1/backtest-runs/"+record.ID+"/cancel", nil, 200, &cancelled)
	if cancelled.Status != "completed" {
		t.Fatal("late cancellation overwrote completed run")
	}
	var latest struct {
		Version strategy.RevisionMetadata `json:"strategy_version"`
	}
	callRunAPI(t, server, "GET", "/api/v1/strategies/ema-boll-rsi/snapshot", nil, 200, &latest)
	if latest.Version.Revision != 2 {
		t.Fatal("run modified strategy revision")
	}
}

func TestRunAPIRejectsUnknownFieldsAndInvalidRunLookups(t *testing.T) {
	server, _ := runAPI(t)
	for _, raw := range []string{
		`{"strategy_id":"ema-boll-rsi","unknown":1}`,
		`{"strategy_id":"ema-boll-rsi","run_config":{"account":{"initial_cash":1,"unknown":true}}}`,
		`{"strategy_id":"ema-boll-rsi","report":{"unknown":true}}`,
	} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest("POST", "/api/v1/backtest-runs", bytes.NewBufferString(raw)))
		if recorder.Code != 400 {
			t.Fatalf("unknown field accepted: %s", raw)
		}
	}
	callRunAPI(t, server, "GET", "/api/v1/strategies/ema-boll-rsi/snapshot?revision=0", nil, 400, nil)
	callRunAPI(t, server, "GET", "/api/v1/strategies/ema-boll-rsi/snapshot?revision=99", nil, 404, nil)
	for _, suffix := range []string{"", "/result", "/snapshot", "/events"} {
		callRunAPI(t, server, "GET", "/api/v1/backtest-runs/missing"+suffix, nil, 404, nil)
	}
}
