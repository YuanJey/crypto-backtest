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
