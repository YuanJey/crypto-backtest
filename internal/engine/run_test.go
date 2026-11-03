package engine

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestRunFillsSignalAtNextBarOpen(t *testing.T) {
	definition := baseDefinition()
	value := true
	definition.Rules = []strategy.RuleSpec{{
		ID: "open_long",
		When: strategy.Expression{
			Kind: strategy.ExprBoolean,
			Bool: &value,
		},
		Action: strategy.ActionSpec{
			Type:    strategy.ActionOpen,
			Side:    strategy.TradeSideLong,
			SizePct: 0.5,
		},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: testBars(3)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Orders) == 0 {
		t.Fatal("expected at least one order")
	}
	if len(result.Fills) != 1 {
		t.Fatalf("len(result.Fills)=%d, want 1", len(result.Fills))
	}
	if result.Fills[0].TimeMS != 900_000 {
		t.Fatalf("fill TimeMS=%d, want next bar open 900000", result.Fills[0].TimeMS)
	}
	if result.Orders[0].SignalTimeMS != 900_000 {
		t.Fatalf("signal TimeMS=%d, want first bar close 900000", result.Orders[0].SignalTimeMS)
	}
}

func TestRunCanGateEntryWithPositionFlatCondition(t *testing.T) {
	definition := baseDefinition()
	definition.Rules = []strategy.RuleSpec{{
		ID: "open_long",
		When: strategy.And(
			strategy.PositionFlat(),
			strategy.Bool(true),
		),
		Action: strategy.ActionSpec{
			Type:    strategy.ActionOpen,
			Side:    strategy.TradeSideLong,
			SizePct: 0.5,
		},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: testBars(5)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 1 {
		t.Fatalf("len(result.Fills)=%d, want only one fill because position.flat() blocks later entries", len(result.Fills))
	}
}

func TestRunSupportsStateDrivenPyramiding(t *testing.T) {
	definition := baseDefinition()
	definition.Position = strategy.PositionSpec{
		InitialSizePct:    0.2,
		AllowPyramiding:   true,
		MaxAdditions:      2,
		AddSizeMultiplier: 2,
		MaxPositionPct:    1,
	}
	definition.Rules = []strategy.RuleSpec{{
		ID:        "add_long",
		Condition: "or(position.flat(), and(position.long(), position.entry_count.lt(3), position.avg_entry_price.gte(market.close)))",
		Action: strategy.ActionSpec{
			Type: strategy.ActionOpen,
			Side: strategy.TradeSideLong,
		},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: fallingBars()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 3 {
		t.Fatalf("len(result.Fills)=%d, want 3 entries capped by entry_count", len(result.Fills))
	}
	if len(result.Orders) != 3 {
		t.Fatalf("len(result.Orders)=%d, want 3", len(result.Orders))
	}
}

func TestRunCapsPyramidingSizeByMaxPositionPct(t *testing.T) {
	definition := baseDefinition()
	definition.Position = strategy.PositionSpec{
		InitialSizePct:    0.6,
		AllowPyramiding:   true,
		MaxAdditions:      5,
		AddSizeMultiplier: 2,
		MaxPositionPct:    0.7,
	}
	definition.Rules = []strategy.RuleSpec{{
		ID:        "add_long",
		Condition: "or(position.flat(), and(position.long(), position.entry_count.lt(6), position.avg_entry_price.gte(market.close)))",
		Action: strategy.ActionSpec{
			Type: strategy.ActionOpen,
			Side: strategy.TradeSideLong,
		},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: fallingBars()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) < 2 {
		t.Fatalf("len(result.Fills)=%d, want at least initial entry plus capped addition", len(result.Fills))
	}
	firstNotional := result.Fills[0].Price * result.Fills[0].Quantity
	secondNotional := result.Fills[1].Price * result.Fills[1].Quantity
	if secondNotional >= firstNotional {
		t.Fatalf("secondNotional=%f, want capped addition smaller than firstNotional=%f", secondNotional, firstNotional)
	}
}

func TestRunDoesNotPyramidWhenMaxAdditionsIsZero(t *testing.T) {
	definition := baseDefinition()
	definition.Behavior.AllowPyramiding = true
	definition.Position = strategy.PositionSpec{
		InitialSizePct: 0.2,
		MaxAdditions:   0,
		MaxPositionPct: 1,
	}
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: testBars(5)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 1 {
		t.Fatalf("len(result.Fills)=%d, want 1", len(result.Fills))
	}
}

func TestRunAppliesCooldownAfterFill(t *testing.T) {
	definition := baseDefinition()
	definition.Behavior.AllowPyramiding = true
	definition.Behavior.CooldownBars = 2
	definition.Position = strategy.PositionSpec{
		InitialSizePct: 0.1,
		MaxAdditions:   4,
		AddSizePct:     0.1,
		MaxPositionPct: 1,
	}
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: testBars(6)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 {
		t.Fatalf("len(result.Fills)=%d, want fills separated by cooldown", len(result.Fills))
	}
	if result.Fills[1].TimeMS-result.Fills[0].TimeMS != 3*900_000 {
		t.Fatalf("unexpected fill spacing: %+v", result.Fills)
	}
}

func TestRunCapsOpenByRiskExposure(t *testing.T) {
	definition := baseDefinition()
	definition.Risk.MaxSymbolEquityPct = 0.25
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong, SizePct: 1},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: testBars(3)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 1 {
		t.Fatalf("len(result.Fills)=%d, want 1", len(result.Fills))
	}
	notional := math.Abs(result.Fills[0].Quantity) * result.Fills[0].Price
	if notional > definition.Account.InitialCash*0.25+1e-9 {
		t.Fatalf("fill notional=%f exceeds risk cap", notional)
	}
}

func TestRunHaltsNewEntriesAtMaxDrawdown(t *testing.T) {
	definition := baseDefinition()
	definition.Behavior.AllowPyramiding = true
	definition.Position = strategy.PositionSpec{
		InitialSizePct: 0.5,
		MaxAdditions:   4,
		AddSizePct:     0.1,
		MaxPositionPct: 1,
	}
	definition.Risk.MaxDrawdownHaltPct = 0.001
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: fallingBars()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 1 {
		t.Fatalf("len(result.Fills)=%d, want risk halt after first fill", len(result.Fills))
	}
}

func TestRunSpotOpenKeepsCashNonNegative(t *testing.T) {
	definition := baseDefinition()
	definition.Account.MarketType = strategy.MarketTypeSpot
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong, SizePct: 1},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: testBars(3)})
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range result.EquityCurve {
		if point.Cash < -1e-9 {
			t.Fatalf("spot cash became negative: %+v", point)
		}
	}
}

func TestRunUsesCompletedHigherTimeframeIndicator(t *testing.T) {
	definition := baseDefinition()
	definition.Indicators[0].ID = "ema_1h"
	definition.Indicators[0].Timeframe = "1h"
	threshold := 0.0
	definition.Rules = []strategy.RuleSpec{{
		ID: "open_long",
		When: strategy.Expression{
			Kind: strategy.ExprOperator,
			Op:   strategy.OperatorGT,
			Args: []strategy.Expression{
				{
					Kind: strategy.ExprReference,
					Ref: &strategy.ValueRef{
						Scope: strategy.ValueScopeIndicator,
						Name:  "ema_1h",
						Field: "value",
					},
				},
				{
					Kind:   strategy.ExprNumber,
					Number: &threshold,
				},
			},
		},
		Action: strategy.ActionSpec{
			Type:    strategy.ActionOpen,
			Side:    strategy.TradeSideLong,
			SizePct: 0.5,
		},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{Bars: testBars(5)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 1 {
		t.Fatalf("len(result.Fills)=%d, want 1", len(result.Fills))
	}
	if result.Orders[0].SignalTimeMS != 3_600_000 {
		t.Fatalf("signal TimeMS=%d, want 1h close", result.Orders[0].SignalTimeMS)
	}
	if result.Fills[0].TimeMS != 3_600_000 {
		t.Fatalf("fill TimeMS=%d, want next bar open after 1h close", result.Fills[0].TimeMS)
	}
}

func TestRunAppliesReportSpec(t *testing.T) {
	definition := baseDefinition()
	value := true
	definition.Rules = []strategy.RuleSpec{{
		ID: "open_long",
		When: strategy.Expression{
			Kind: strategy.ExprBoolean,
			Bool: &value,
		},
		Action: strategy.ActionSpec{
			Type:    strategy.ActionOpen,
			Side:    strategy.TradeSideLong,
			SizePct: 0.5,
		},
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, BacktestRequest{
		Bars: testBars(3),
		Report: ReportSpec{
			Metrics: []MetricType{MetricFinalEquity, MetricOrderCount, MetricFillCount},
			Indicators: []IndicatorOverlay{{
				ID:     "ema_15m",
				Fields: []string{"value"},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Orders) != 0 || len(result.Fills) != 0 || len(result.EquityCurve) != 0 {
		t.Fatalf("expected detail sections to be omitted: %+v", result)
	}
	if result.TotalOrders == 0 || result.TotalFills == 0 {
		t.Fatalf("expected internal totals to survive report filtering: %+v", result)
	}
	if result.Metrics[MetricOrderCount] == 0 || result.Metrics[MetricFillCount] == 0 {
		t.Fatalf("expected order/fill metrics: %+v", result.Metrics)
	}
	if len(result.Indicators["ema_15m"].Fields["value"]) != 3 {
		t.Fatalf("expected indicator values for every bar: %+v", result.Indicators)
	}
}

func TestRunRejectsUnknownReportIndicator(t *testing.T) {
	definition := baseDefinition()
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	_, err = Run(context.Background(), plan, BacktestRequest{
		Bars: testBars(3),
		Report: ReportSpec{
			Indicators: []IndicatorOverlay{{ID: "missing"}},
		},
	})
	if err == nil {
		t.Fatal("expected unknown report indicator to fail")
	}
}

func TestRunIndicatorReportUsesNullForWarmupValues(t *testing.T) {
	definition := baseDefinition()
	definition.Indicators[0].Params = json.RawMessage(`{"period":3}`)
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), plan, RunRequest{
		Bars: testBars(3),
		Report: ReportSpec{
			Indicators: []IndicatorOverlay{{ID: "ema_15m"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	values := result.Indicators["ema_15m"].Fields["value"]
	if len(values) != 3 || values[0] != nil || values[1] != nil || values[2] == nil {
		t.Fatalf("unexpected nullable warmup series: %+v", values)
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatalf("indicator report must be JSON serializable: %v", err)
	}
}

func TestRunRejectsUnknownReportMetric(t *testing.T) {
	definition := baseDefinition()
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	_, err = Run(context.Background(), plan, RunRequest{
		Bars:   testBars(3),
		Report: ReportSpec{Metrics: []MetricType{"unknown"}},
	})
	if err == nil {
		t.Fatal("expected unknown report metric to fail")
	}
}

func TestRunRejectsFormingBaseBar(t *testing.T) {
	definition := baseDefinition()
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	bars := testBars(3)
	bars[1].State = market.BarStateForming
	_, err = Run(context.Background(), plan, RunRequest{Bars: bars})
	if err == nil {
		t.Fatal("expected forming base bar to fail")
	}
}

func baseDefinition() strategy.Definition {
	start := time.UnixMilli(0).UTC()
	end := time.UnixMilli(10 * 900_000).UTC()
	return strategy.Definition{
		SchemaVersion: strategy.SchemaVersion,
		ID:            "test",
		Name:          "test",
		Data: strategy.DataSelection{
			DatasetID: "test-dataset",
			Symbols:   []string{"BTC-USDT"},
			Timeframe: "15m",
			Start:     start,
			End:       end,
		},
		Account: strategy.AccountSpec{
			MarketType:    strategy.MarketTypePerpetual,
			InitialCash:   10_000,
			QuoteCurrency: "USDT",
		},
		Behavior: strategy.BehaviorSpec{},
		Execution: strategy.ExecutionSpec{
			SignalTime:       strategy.SignalTimeClose,
			MarketFillTime:   strategy.MarketFillNextOpen,
			IntrabarConflict: strategy.IntrabarConflictConservative,
			Fee:              strategy.FeeSpec{TakerRate: 0.001},
			Slippage:         strategy.SlippageSpec{Type: strategy.SlippageTypeNone},
		},
		Risk: strategy.RiskSpec{
			MaxSymbolEquityPct:  1,
			MaxTotalExposurePct: 1,
			MaxOpenPositions:    1,
			MaxDrawdownHaltPct:  0.25,
		},
		Indicators: []strategy.IndicatorSpec{{
			ID:        "ema_15m",
			Type:      "ema",
			Timeframe: "15m",
			Inputs: []strategy.ValueRef{{
				Scope: strategy.ValueScopeMarket,
				Name:  "bar",
				Field: "close",
			}},
			Params: json.RawMessage(`{"period":1}`),
		}},
	}
}

func testBars(count int) []market.Bar {
	bars := make([]market.Bar, 0, count)
	for i := 0; i < count; i++ {
		openTime := int64(i) * 900_000
		open := 100.0 + float64(i)
		bars = append(bars, market.Bar{
			OpenTimeMS:  openTime,
			Open:        open,
			High:        open + 2,
			Low:         open - 2,
			Close:       open + 1,
			Volume:      1,
			QuoteVolume: open,
			TradeCount:  1,
			State:       market.BarStateClosed,
		})
	}
	return bars
}

func fallingBars() []market.Bar {
	opens := []float64{100, 99, 97, 95, 93, 91}
	bars := make([]market.Bar, 0, len(opens))
	for i, open := range opens {
		openTime := int64(i) * 900_000
		bars = append(bars, market.Bar{
			OpenTimeMS:  openTime,
			Open:        open,
			High:        open + 1,
			Low:         open - 2,
			Close:       open - 1,
			Volume:      1,
			QuoteVolume: open,
			TradeCount:  1,
			State:       market.BarStateClosed,
		})
	}
	return bars
}
