package engine

import (
	"context"
	"math"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestExitTargetSupportsPercentAndDirectionalPurpose(t *testing.T) {
	portfolio := newPortfolio(strategy.MarketTypePerpetual, 10_000)
	portfolio.quantity = 1
	portfolio.avgPrice = 100
	portfolio.firstEntryPrice = 98
	portfolio.lastEntryPrice = 102
	portfolio.highestSinceEntry = 110
	portfolio.lowestSinceEntry = 90

	tests := []struct {
		name    string
		side    strategy.TradeSide
		purpose strategy.ExitPurpose
		anchor  strategy.ExitAnchorType
		want    float64
	}{
		{name: "long take profit", side: strategy.TradeSideLong, purpose: strategy.ExitPurposeTakeProfit, anchor: strategy.ExitAnchorAverageEntry, want: 105},
		{name: "long stop loss", side: strategy.TradeSideLong, purpose: strategy.ExitPurposeStopLoss, anchor: strategy.ExitAnchorFirstEntry, want: 93.1},
		{name: "short take profit", side: strategy.TradeSideShort, purpose: strategy.ExitPurposeTakeProfit, anchor: strategy.ExitAnchorLastEntry, want: 96.9},
		{name: "short stop loss", side: strategy.TradeSideShort, purpose: strategy.ExitPurposeStopLoss, anchor: strategy.ExitAnchorAverageEntry, want: 105},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := exitPolicyRuntime{spec: strategy.ExitPolicySpec{
				Side:    test.side,
				Purpose: test.purpose,
				Anchor:  strategy.ExitAnchorSpec{Type: test.anchor},
				Distance: strategy.ExitDistanceSpec{
					Type:  strategy.ExitDistancePercent,
					Value: 0.05,
				},
			}}
			got, ok := exitTarget(portfolio, runtime, nil)
			if !ok {
				t.Fatal("expected exit target")
			}
			if math.Abs(got-test.want) > 1e-9 {
				t.Fatalf("target=%f, want %f", got, test.want)
			}
		})
	}
}

func TestExitPolicyATRSamplingModes(t *testing.T) {
	states := []indicatorState{{
		plan: compiler.IndicatorPlan{ID: "atr_15m"},
		last: map[string]float64{"value": 2},
	}}
	specs := []strategy.ExitPolicySpec{
		{ID: "entry", Distance: strategy.ExitDistanceSpec{Type: strategy.ExitDistanceATRMultiple, Value: 1, Indicator: "atr_15m", Field: "value", Sampling: strategy.ExitSamplingEntry}},
		{ID: "each", Distance: strategy.ExitDistanceSpec{Type: strategy.ExitDistanceATRMultiple, Value: 1, Indicator: "atr_15m", Field: "value", Sampling: strategy.ExitSamplingEachEntry}},
		{ID: "live", Distance: strategy.ExitDistanceSpec{Type: strategy.ExitDistanceATRMultiple, Value: 1, Indicator: "atr_15m", Field: "value", Sampling: strategy.ExitSamplingLive}},
	}
	runtimes := newExitPolicyRuntimes(specs)
	snapshotExitPolicies(runtimes, states, true)
	states[0].last["value"] = 3
	snapshotExitPolicies(runtimes, states, false)

	if runtimes[0].sampledDistance != 2 {
		t.Fatalf("entry sample=%f, want 2", runtimes[0].sampledDistance)
	}
	if runtimes[1].sampledDistance != 3 {
		t.Fatalf("each-entry sample=%f, want 3", runtimes[1].sampledDistance)
	}
	live, ok := exitDistanceValue(100, runtimes[2], states)
	if !ok || live != 3 {
		t.Fatalf("live distance=%f, ok=%v, want 3", live, ok)
	}
}

func TestRunExecutesPercentExitAtTriggerPrice(t *testing.T) {
	definition := baseDefinition()
	definition.Execution.Fee.TakerRate = 0
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong, SizePct: 0.2},
	}}
	definition.ExitPolicies = []strategy.ExitPolicySpec{{
		ID:      "long_tp_5pct",
		Side:    strategy.TradeSideLong,
		Purpose: strategy.ExitPurposeTakeProfit,
		Anchor:  strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry},
		Distance: strategy.ExitDistanceSpec{
			Type:  strategy.ExitDistancePercent,
			Value: 0.05,
		},
		TriggerPrice: strategy.ExitTriggerHigh,
		ClosePct:     1,
		Execution:    strategy.ExitExecutionTriggerPrice,
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	bars := constantPriceBars(4, 100)
	bars[2].High = 106
	bars[2].Close = 104
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 {
		t.Fatalf("fills=%d, want entry and exit", len(result.Fills))
	}
	if math.Abs(result.Fills[1].Price-105) > 1e-9 {
		t.Fatalf("exit price=%f, want target 105", result.Fills[1].Price)
	}
	if result.Fills[1].RuleID != "long_tp_5pct" {
		t.Fatalf("exit rule id=%q", result.Fills[1].RuleID)
	}
}

func TestRunUsesGapOpenForTriggerPriceStop(t *testing.T) {
	definition := baseDefinition()
	definition.Execution.Fee.TakerRate = 0
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong, SizePct: 0.2},
	}}
	definition.ExitPolicies = []strategy.ExitPolicySpec{{
		ID:      "long_sl_5pct",
		Side:    strategy.TradeSideLong,
		Purpose: strategy.ExitPurposeStopLoss,
		Anchor:  strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry},
		Distance: strategy.ExitDistanceSpec{
			Type:  strategy.ExitDistancePercent,
			Value: 0.05,
		},
		TriggerPrice: strategy.ExitTriggerLow,
		ClosePct:     1,
		Execution:    strategy.ExitExecutionTriggerPrice,
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	bars := constantPriceBars(4, 100)
	bars[2].Open = 90
	bars[2].High = 92
	bars[2].Low = 89
	bars[2].Close = 91
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 {
		t.Fatalf("fills=%d, want entry and stop", len(result.Fills))
	}
	if result.Fills[1].Price != 90 {
		t.Fatalf("stop price=%f, want gap open 90", result.Fills[1].Price)
	}
}

func TestRunUsesCloseForCloseTriggeredExit(t *testing.T) {
	definition := baseDefinition()
	definition.Execution.Fee.TakerRate = 0
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong, SizePct: 0.2},
	}}
	definition.ExitPolicies = []strategy.ExitPolicySpec{{
		ID:      "long_close_stop",
		Side:    strategy.TradeSideLong,
		Purpose: strategy.ExitPurposeStopLoss,
		Anchor:  strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry},
		Distance: strategy.ExitDistanceSpec{
			Type:  strategy.ExitDistancePercent,
			Value: 0.05,
		},
		TriggerPrice: strategy.ExitTriggerClose,
		ClosePct:     1,
		Execution:    strategy.ExitExecutionTriggerPrice,
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	bars := constantPriceBars(3, 100)
	bars[2].Low = 90
	bars[2].Close = 94
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 {
		t.Fatalf("fills=%d, want entry and close-triggered exit", len(result.Fills))
	}
	if result.Fills[1].Price != 94 {
		t.Fatalf("exit price=%f, want close 94", result.Fills[1].Price)
	}
}

func TestRunUsesPriorClosedATRForLiveTrigger(t *testing.T) {
	definition := baseDefinition()
	definition.Execution.Fee.TakerRate = 0
	definition.Indicators = append(definition.Indicators, strategy.IndicatorSpec{
		ID:        "atr_15m",
		Type:      "atr",
		Timeframe: "15m",
		Inputs: []strategy.ValueRef{
			{Scope: strategy.ValueScopeMarket, Name: "bar", Field: "high"},
			{Scope: strategy.ValueScopeMarket, Name: "bar", Field: "low"},
			{Scope: strategy.ValueScopeMarket, Name: "bar", Field: "close"},
		},
		Params: []byte(`{"period":1}`),
	})
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong, SizePct: 0.2},
	}}
	definition.ExitPolicies = []strategy.ExitPolicySpec{{
		ID:      "live_atr_tp",
		Side:    strategy.TradeSideLong,
		Purpose: strategy.ExitPurposeTakeProfit,
		Anchor:  strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry},
		Distance: strategy.ExitDistanceSpec{
			Type:      strategy.ExitDistanceATRMultiple,
			Value:     1,
			Indicator: "atr_15m",
			Field:     "value",
			Sampling:  strategy.ExitSamplingLive,
		},
		TriggerPrice: strategy.ExitTriggerHigh,
		ClosePct:     1,
		Execution:    strategy.ExitExecutionTriggerPrice,
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	bars := constantPriceBars(3, 100)
	bars[1].High = 101
	bars[1].Low = 99
	bars[2].High = 103
	bars[2].Low = 90
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 {
		t.Fatalf("fills=%d, want entry and live ATR exit", len(result.Fills))
	}
	if result.Fills[1].Price != 102 {
		t.Fatalf("exit price=%f, want prior-bar ATR target 102", result.Fills[1].Price)
	}
}

func TestRunExecutesExitAtPriorClosedExtreme(t *testing.T) {
	definition := baseDefinition()
	definition.Execution.Fee.TakerRate = 0
	definition.Indicators = append(definition.Indicators, strategy.IndicatorSpec{
		ID:        "extreme_2",
		Type:      "extreme",
		Timeframe: "15m",
		Inputs: []strategy.ValueRef{
			{Scope: strategy.ValueScopeMarket, Name: "bar", Field: "high"},
			{Scope: strategy.ValueScopeMarket, Name: "bar", Field: "low"},
		},
		Params: []byte(`{"period":2}`),
	})
	definition.Rules = []strategy.RuleSpec{{
		ID:     "open_long",
		When:   strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong, SizePct: 0.2},
	}}
	definition.ExitPolicies = []strategy.ExitPolicySpec{{
		ID:           "long_tp_extreme",
		Side:         strategy.TradeSideLong,
		Purpose:      strategy.ExitPurposeTakeProfit,
		Anchor:       strategy.ExitAnchorSpec{Type: strategy.ExitAnchorIndicator, Indicator: "extreme_2", Field: "highest"},
		Distance:     strategy.ExitDistanceSpec{Type: strategy.ExitDistanceDirect},
		TriggerPrice: strategy.ExitTriggerHigh,
		ClosePct:     1,
		Execution:    strategy.ExitExecutionTriggerPrice,
	}}
	plan, err := compiler.Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	bars := constantPriceBars(4, 100)
	bars[0].High = 105
	bars[1].High = 110
	bars[2].High = 111
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 {
		t.Fatalf("fills=%d, want entry and extreme exit", len(result.Fills))
	}
	if result.Fills[1].Price != 110 {
		t.Fatalf("exit price=%f, want prior closed rolling high 110", result.Fills[1].Price)
	}
}
