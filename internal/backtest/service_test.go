package backtest

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/YuanJey/crypto-backtest/internal/dataset"
	"github.com/YuanJey/crypto-backtest/internal/engine"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestServiceRunsSQLiteStrategy(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := strategy.OpenSQLiteRepository(ctx, filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	definition := testDefinition()
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, definition, raw); err != nil {
		t.Fatal(err)
	}
	writeTestDataset(t, root, definition)

	service := NewService(repository, indicator.MustBuiltinRegistry(), filepath.Join(root, "market"))
	response, err := service.Run(ctx, Request{
		StrategyID: definition.ID,
		Report: engine.ReportSpec{
			Metrics: []engine.MetricType{engine.MetricFinalEquity, engine.MetricOrderCount, engine.MetricFillCount},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.BarCount != 3 {
		t.Fatalf("BarCount=%d, want 3", response.BarCount)
	}
	if response.OrderCount == 0 || response.FillCount == 0 {
		t.Fatalf("expected orders and fills, got orders=%d fills=%d", response.OrderCount, response.FillCount)
	}
	if response.Data.Synthetic || response.Data.ManifestSHA256 == "" {
		t.Fatalf("expected real dataset provenance with manifest hash: %+v", response.Data)
	}
	if response.StrategyVersion.Revision != 1 || response.StrategyVersion.DefinitionSHA256 == "" {
		t.Fatalf("unexpected strategy version: %+v", response.StrategyVersion)
	}
}

func TestServiceRunsRequestedStrategyRevision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := strategy.OpenSQLiteRepository(ctx, filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	first := testDefinition()
	first.Name = "Revision One"
	firstRaw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, first, firstRaw); err != nil {
		t.Fatal(err)
	}
	writeTestDataset(t, root, first)

	second := first
	second.Name = "Revision Two"
	secondRaw, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, second, secondRaw); err != nil {
		t.Fatal(err)
	}

	service := NewService(repository, indicator.MustBuiltinRegistry(), filepath.Join(root, "market"))
	response, err := service.Run(ctx, Request{
		StrategyID:       first.ID,
		StrategyRevision: 1,
		Report: engine.ReportSpec{
			Metrics: []engine.MetricType{engine.MetricFinalEquity},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Strategy.Name != "Revision One" || response.StrategyVersion.Revision != 1 {
		t.Fatalf("unexpected historical strategy response: %+v", response)
	}
}

func TestBuildResponseKeepsCountsWhenDetailsAreOmitted(t *testing.T) {
	response := BuildResponse(
		testDefinition(),
		strategy.RevisionMetadata{Revision: 1, DefinitionSHA256: "hash"},
		DataProvenance{DatasetID: "__sample__", Synthetic: true},
		3,
		engine.Result{TotalOrders: 2, TotalFills: 1, FinalEquity: 10_100},
	)
	if response.OrderCount != 2 || response.FillCount != 1 {
		t.Fatalf("unexpected response counts: %+v", response)
	}
}

func writeTestDataset(t *testing.T, root string, definition strategy.Definition) {
	t.Helper()
	outputRoot := filepath.Join(root, "market")
	writer := dataset.Writer{
		OutputRoot: outputRoot,
		DatasetID:  definition.Data.DatasetID,
		Exchange:   "binance",
		Timeframe:  definition.Data.Timeframe,
		Overwrite:  true,
	}
	bars := []market.Bar{
		testBar(0, 100),
		testBar(900_000, 101),
		testBar(1_800_000, 102),
	}
	entries, err := writer.WriteSymbolBars(definition.Data.Symbols[0], bars)
	if err != nil {
		t.Fatal(err)
	}
	report := dataset.QualityReport{Symbol: definition.Data.Symbols[0], Timeframe: definition.Data.Timeframe, RowCount: len(bars)}
	manifest := dataset.BuildManifest(definition.Data.DatasetID, "binance", "perpetual", definition.Data.Timeframe, definition.Data.Symbols, 0, 3*900_000, []dataset.QualityReport{report}, entries)
	datasetRoot, err := writer.Root()
	if err != nil {
		t.Fatal(err)
	}
	if err := dataset.WriteManifest(datasetRoot, manifest); err != nil {
		t.Fatal(err)
	}
}

func testDefinition() strategy.Definition {
	start := time.UnixMilli(0).UTC()
	end := time.UnixMilli(3 * 900_000).UTC()
	value := true
	return strategy.Definition{
		SchemaVersion: strategy.SchemaVersion,
		ID:            "test-strategy",
		Name:          "Test Strategy",
		Data: strategy.DataSelection{
			DatasetID: "test-dataset",
			Symbols:   []string{"BTC-USDT"},
			Timeframe: "15m",
			Start:     start,
			End:       end,
		},
		Account: strategy.AccountSpec{MarketType: strategy.MarketTypePerpetual, InitialCash: 10_000, QuoteCurrency: "USDT"},
		Execution: strategy.ExecutionSpec{
			SignalTime:       strategy.SignalTimeClose,
			MarketFillTime:   strategy.MarketFillNextOpen,
			IntrabarConflict: strategy.IntrabarConflictConservative,
			Fee:              strategy.FeeSpec{TakerRate: 0.001},
			Slippage:         strategy.SlippageSpec{Type: strategy.SlippageTypeNone},
		},
		Risk: strategy.RiskSpec{MaxSymbolEquityPct: 1, MaxTotalExposurePct: 1, MaxOpenPositions: 1, MaxDrawdownHaltPct: 0.25},
		Indicators: []strategy.IndicatorSpec{{
			ID:        "ema_15m",
			Type:      "ema",
			Timeframe: "15m",
			Inputs:    []strategy.ValueRef{{Scope: strategy.ValueScopeMarket, Name: "bar", Field: "close"}},
			Params:    json.RawMessage(`{"period":1}`),
		}},
		Rules: []strategy.RuleSpec{{
			ID:   "open_long",
			When: strategy.Expression{Kind: strategy.ExprBoolean, Bool: &value},
			Action: strategy.ActionSpec{
				Type:    strategy.ActionOpen,
				Side:    strategy.TradeSideLong,
				SizePct: 0.5,
			},
		}},
	}
}

func testBar(openTimeMS int64, open float64) market.Bar {
	return market.Bar{
		OpenTimeMS:  openTimeMS,
		Open:        open,
		High:        open + 2,
		Low:         open - 2,
		Close:       open + 1,
		Volume:      1,
		QuoteVolume: open,
		TradeCount:  1,
		State:       market.BarStateClosed,
	}
}

func TestServiceRejectsMissingDatasetWithoutExplicitSample(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := strategy.OpenSQLiteRepository(ctx, filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	definition := testDefinition()
	definition.Data.DatasetID = "non-existent-dataset"
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, definition, raw); err != nil {
		t.Fatal(err)
	}

	service := NewService(repository, indicator.MustBuiltinRegistry(), filepath.Join(root, "empty-market"))
	_, err = service.Run(ctx, Request{
		StrategyID: definition.ID,
		Report: engine.ReportSpec{
			Metrics: []engine.MetricType{engine.MetricFinalEquity, engine.MetricOrderCount},
		},
	})
	if err == nil {
		t.Fatal("expected missing dataset to fail instead of using synthetic bars")
	}
}

func TestServiceExplicitSampleDataset(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := strategy.OpenSQLiteRepository(ctx, filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	definition := testDefinition()
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, definition, raw); err != nil {
		t.Fatal(err)
	}

	service := NewService(repository, indicator.MustBuiltinRegistry(), filepath.Join(root, "empty-market"))
	response, err := service.Run(ctx, Request{
		StrategyID: definition.ID,
		DatasetID:  "__sample__",
		Symbol:     "ETH-USDT",
		Report: engine.ReportSpec{
			Metrics: []engine.MetricType{engine.MetricFinalEquity, engine.MetricOrderCount},
		},
	})
	if err != nil {
		t.Fatalf("expected explicit sample run to succeed, got: %v", err)
	}
	if response.BarCount == 0 {
		t.Fatalf("expected sample bars, got 0")
	}
	if !response.Data.Synthetic || response.Data.DatasetID != "__sample__" {
		t.Fatalf("unexpected sample provenance: %+v", response.Data)
	}
}
