package compiler

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestCompileBuildsIndicatorRuntimesAndRules(t *testing.T) {
	raw, err := os.ReadFile("../../strategies/ema-boll-rsi.json")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := strategy.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Indicators) != 4 {
		t.Fatalf("len(plan.Indicators)=%d, want 4", len(plan.Indicators))
	}
	if len(plan.Rules) != 2 {
		t.Fatalf("len(plan.Rules)=%d, want 2", len(plan.Rules))
	}
	if plan.Rules[0].ID != "open_long" {
		t.Fatalf("first rule=%q, want open_long", plan.Rules[0].ID)
	}
	if plan.Indicators[1].Timeframe != "1h" {
		t.Fatalf("second indicator timeframe=%q, want 1h", plan.Indicators[1].Timeframe)
	}
}

func TestCompileOrdersIndicatorsByDependency(t *testing.T) {
	raw, err := os.ReadFile("../../strategies/ema-boll-rsi.json")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := strategy.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	dependent := strategy.IndicatorSpec{
		ID:        "sma_of_ema",
		Type:      "sma",
		Timeframe: "15m",
		Inputs: []strategy.ValueRef{{
			Scope: strategy.ValueScopeIndicator,
			Name:  "ema_fast_15m",
			Field: "value",
		}},
		Params: json.RawMessage(`{"period":1}`),
	}
	definition.Indicators = append([]strategy.IndicatorSpec{dependent}, definition.Indicators...)

	plan, err := Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	indexByID := map[string]int{}
	for index, item := range plan.Indicators {
		indexByID[item.ID] = index
	}
	if indexByID["ema_fast_15m"] >= indexByID["sma_of_ema"] {
		t.Fatalf("dependency order is invalid: %+v", indexByID)
	}
}

func TestCompileOrdersExitPoliciesByPriorityAndConservativeTieBreak(t *testing.T) {
	raw, err := os.ReadFile("../../strategies/ema-boll-rsi.json")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := strategy.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	policy := func(id string, purpose strategy.ExitPurpose, priority int) strategy.ExitPolicySpec {
		return strategy.ExitPolicySpec{
			ID:           id,
			Side:         strategy.TradeSideLong,
			Purpose:      purpose,
			Anchor:       strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry},
			Distance:     strategy.ExitDistanceSpec{Type: strategy.ExitDistancePercent, Value: 0.03},
			TriggerPrice: strategy.ExitTriggerClose,
			ClosePct:     1,
			Priority:     priority,
			Execution:    strategy.ExitExecutionNextOpen,
		}
	}
	definition.ExitPolicies = []strategy.ExitPolicySpec{
		policy("low_priority", strategy.ExitPurposeStopLoss, 1),
		policy("take_profit", strategy.ExitPurposeTakeProfit, 10),
		policy("stop_loss", strategy.ExitPurposeStopLoss, 10),
	}
	plan, err := Compile(definition, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	wants := []string{"stop_loss", "take_profit", "low_priority"}
	for index, want := range wants {
		if plan.ExitPolicies[index].ID != want {
			t.Fatalf("exit policy %d=%q, want %q", index, plan.ExitPolicies[index].ID, want)
		}
	}
}
