package strategy

import (
	"math"
	"strings"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
)

func TestValidateRuleLocalSizing(t *testing.T) {
	cases := []struct {
		name   string
		action ActionSpec
		valid  bool
	}{
		{"entry", ActionSpec{Type: ActionEnter, SizePct: 0.02}, true},
		{"tiny entry", ActionSpec{Type: ActionEnter, SizePct: 0.000000001}, true},
		{"entry cannot inherit", ActionSpec{Type: ActionEnter}, false},
		{"entry cannot specify addition index", ActionSpec{Type: ActionEnter, SizePct: 0.02, AdditionIndex: 1}, false},
		{"entry cannot specify multiplier", ActionSpec{Type: ActionEnter, SizePct: 0.02, SizeMultiplier: 2}, false},
		{"fixed addition", ActionSpec{Type: ActionAdd, SizePct: 0.02, AdditionIndex: 1}, true},
		{"multiplier addition", ActionSpec{Type: ActionAdd, SizeMultiplier: 4, AdditionIndex: 2}, true},
		{"current position multiple", ActionSpec{Type: ActionAdd, SizingMode: AdditionSizeCurrentMultiple, SizeMultiplier: 2, AdditionIndex: 2}, true},
		{"first entry geometric", ActionSpec{Type: ActionAdd, SizingMode: AdditionSizeFirstGeometric, SizeMultiplier: 2, AdditionIndex: 3}, true},
		{"explicit equity size", ActionSpec{Type: ActionAdd, SizingMode: AdditionSizeEquityPct, SizePct: 0.1, AdditionIndex: 1}, true},
		{"equity mode with multiplier", ActionSpec{Type: ActionAdd, SizingMode: AdditionSizeEquityPct, SizeMultiplier: 2, AdditionIndex: 1}, false},
		{"current mode with percentage", ActionSpec{Type: ActionAdd, SizingMode: AdditionSizeCurrentMultiple, SizePct: 0.1, AdditionIndex: 1}, false},
		{"unknown sizing mode", ActionSpec{Type: ActionAdd, SizingMode: "unknown", SizeMultiplier: 2, AdditionIndex: 1}, false},
		{"geometric overflow", ActionSpec{Type: ActionAdd, SizingMode: AdditionSizeFirstGeometric, SizeMultiplier: 1e308, AdditionIndex: 2}, false},
		{"geometric underflow", ActionSpec{Type: ActionAdd, SizingMode: AdditionSizeFirstGeometric, SizeMultiplier: 1e-308, AdditionIndex: 2}, false},
		{"entry cannot specify sizing mode", ActionSpec{Type: ActionEnter, SizingMode: AdditionSizeEquityPct, SizePct: 0.1}, false},
		{"missing addition index", ActionSpec{Type: ActionAdd, SizePct: 0.02}, false},
		{"negative addition index", ActionSpec{Type: ActionAdd, SizePct: 0.02, AdditionIndex: -1}, false},
		{"missing addition size", ActionSpec{Type: ActionAdd, AdditionIndex: 1}, false},
		{"ambiguous addition size", ActionSpec{Type: ActionAdd, SizePct: 0.02, SizeMultiplier: 2, AdditionIndex: 1}, false},
		{"infinite multiplier", ActionSpec{Type: ActionAdd, SizeMultiplier: math.Inf(1), AdditionIndex: 1}, false},
		{"NaN proportion", ActionSpec{Type: ActionEnter, SizePct: math.NaN()}, false},
		{"full close", ActionSpec{Type: ActionClose}, true},
		{"close cannot specify addition index", ActionSpec{Type: ActionClose, SizePct: 1, AdditionIndex: 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := validDefinition()
			def.Position.InitialSizePct = 0.5 // New actions must not inherit this.
			tc.action.Side = TradeSideLong
			def.Rules = []RuleSpec{{ID: "rule", When: Bool(true), Action: tc.action}}
			err := Validate(def, indicator.MustBuiltinRegistry())
			if (err == nil) != tc.valid {
				t.Fatalf("validation=%v, want valid=%t", err, tc.valid)
			}
		})
	}
}

func TestDecodeRejectsUnknownActionSizingFields(t *testing.T) {
	raw := `{"rules":[{"action":{"type":"add","side":"long","size_pct":0.1,"addition_index":1,"size_formula":"eval"}}]}`
	if _, err := Decode([]byte(raw)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected strict rejection of unknown sizing field, got %v", err)
	}
}
