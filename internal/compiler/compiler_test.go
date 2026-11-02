package compiler

import (
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
