package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bytedance/crypto-backtest/internal/compiler"
	"github.com/bytedance/crypto-backtest/internal/indicator"
	"github.com/bytedance/crypto-backtest/internal/market"
	"github.com/bytedance/crypto-backtest/internal/strategy"
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
		})
	}
	return bars
}
