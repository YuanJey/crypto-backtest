package strategy

import (
	"math"
	"strings"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
)

func TestAdditionTriggerValidationAndNormalization(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuleSpec)
		valid  bool
	}{
		{"defaults", func(r *RuleSpec) {}, true},
		{"first anchor", func(r *RuleSpec) { r.AdditionTrigger.Anchor.Type = ExitAnchorFirstEntry }, true},
		{"last anchor", func(r *RuleSpec) { r.AdditionTrigger.Anchor.Type = ExitAnchorLastEntry }, true},
		{"fixed price", func(r *RuleSpec) { r.AdditionTrigger.Distance.Type = ExitDistanceFixedAmount }, true},
		{"ATR defaults", func(r *RuleSpec) {
			r.AdditionTrigger.Distance = ExitDistanceSpec{Type: ExitDistanceATRMultiple, Indicator: "atr", Value: 2}
		}, true},
		{"linear", func(r *RuleSpec) {
			r.AdditionTrigger.Spacing = &AdditionSpacingSpec{Mode: AdditionSpacingLinear, Value: 0.01}
		}, true},
		{"geometric", func(r *RuleSpec) {
			r.AdditionTrigger.Spacing = &AdditionSpacingSpec{Mode: AdditionSpacingGeometric, Value: 2}
		}, true},
		{"not add", func(r *RuleSpec) { r.Action = ActionSpec{Type: ActionEnter, Side: TradeSideLong, SizePct: 0.1} }, false},
		{"indicator anchor", func(r *RuleSpec) { r.AdditionTrigger.Anchor.Type = ExitAnchorIndicator }, false},
		{"anchor fields", func(r *RuleSpec) { r.AdditionTrigger.Anchor.Indicator = "atr" }, false},
		{"unknown direction", func(r *RuleSpec) { r.AdditionTrigger.Direction = "unknown" }, false},
		{"unknown trigger price", func(r *RuleSpec) { r.AdditionTrigger.TriggerPrice = "open" }, false},
		{"direct distance", func(r *RuleSpec) { r.AdditionTrigger.Distance.Type = ExitDistanceDirect }, false},
		{"zero distance", func(r *RuleSpec) { r.AdditionTrigger.Distance.Value = 0 }, false},
		{"NaN distance", func(r *RuleSpec) { r.AdditionTrigger.Distance.Value = math.NaN() }, false},
		{"percentage with ATR", func(r *RuleSpec) { r.AdditionTrigger.Distance.Indicator = "atr" }, false},
		{"price output not ATR", func(r *RuleSpec) {
			r.AdditionTrigger.Distance = ExitDistanceSpec{Type: ExitDistanceATRMultiple, Indicator: "ema_fast", Value: 1}
		}, false},
		{"missing ATR", func(r *RuleSpec) {
			r.AdditionTrigger.Distance = ExitDistanceSpec{Type: ExitDistanceATRMultiple, Indicator: "missing", Value: 1}
		}, false},
		{"missing ATR field", func(r *RuleSpec) {
			r.AdditionTrigger.Distance = ExitDistanceSpec{Type: ExitDistanceATRMultiple, Indicator: "atr", Field: "missing", Value: 1}
		}, false},
		{"invalid sampling", func(r *RuleSpec) {
			r.AdditionTrigger.Distance = ExitDistanceSpec{Type: ExitDistanceATRMultiple, Indicator: "atr", Value: 1, Sampling: "future"}
		}, false},
		{"fixed extra value", func(r *RuleSpec) {
			r.AdditionTrigger.Spacing = &AdditionSpacingSpec{Mode: AdditionSpacingFixed, Value: 1}
		}, false},
		{"negative increment", func(r *RuleSpec) {
			r.AdditionTrigger.Spacing = &AdditionSpacingSpec{Mode: AdditionSpacingLinear, Value: -1}
		}, false},
		{"shrinking spacing", func(r *RuleSpec) {
			r.AdditionTrigger.Spacing = &AdditionSpacingSpec{Mode: AdditionSpacingGeometric, Value: 0.5}
		}, false},
		{"unknown spacing", func(r *RuleSpec) { r.AdditionTrigger.Spacing = &AdditionSpacingSpec{Mode: "unknown"} }, false},
		{"spacing overflow", func(r *RuleSpec) {
			r.Action.AdditionIndex = 4
			r.AdditionTrigger.Spacing = &AdditionSpacingSpec{Mode: AdditionSpacingGeometric, Value: 1e308}
		}, false},
		{"percentage exceeds 100", func(r *RuleSpec) {
			r.Action.AdditionIndex = 3
			r.AdditionTrigger.Spacing = &AdditionSpacingSpec{Mode: AdditionSpacingLinear, Value: 0.5}
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def := validDefinition()
			def.Indicators = append(def.Indicators, IndicatorSpec{ID: "atr", Type: "atr", Params: []byte(`{"period":1}`)})
			rule := RuleSpec{ID: "add", When: Bool(true),
				Action:          ActionSpec{Type: ActionAdd, Side: TradeSideLong, AdditionIndex: 2, SizePct: 0.1},
				AdditionTrigger: &AdditionTriggerSpec{Distance: ExitDistanceSpec{Type: ExitDistancePercent, Value: 0.01}},
			}
			tc.mutate(&rule)
			def.Rules = []RuleSpec{rule}
			err := Validate(def, indicator.MustBuiltinRegistry())
			if (err == nil) != tc.valid {
				t.Fatalf("validation=%v, want valid=%v", err, tc.valid)
			}
		})
	}
}

func TestAdditionDistanceSequence(t *testing.T) {
	for _, tc := range []struct {
		spacing *AdditionSpacingSpec
		want    []float64
	}{
		{nil, []float64{2, 2, 2}},
		{&AdditionSpacingSpec{Mode: AdditionSpacingLinear, Value: 1}, []float64{2, 3, 4}},
		{&AdditionSpacingSpec{Mode: AdditionSpacingGeometric, Value: 2}, []float64{2, 4, 8}},
	} {
		spec := AdditionTriggerSpec{Distance: ExitDistanceSpec{Type: ExitDistanceATRMultiple, Value: 2}, Spacing: tc.spacing}
		for i, want := range tc.want {
			if got, ok := spec.DistanceAt(i + 1); !ok || got != want {
				t.Fatalf("distance[%d]=%g/%v, want %g", i+1, got, ok, want)
			}
		}
	}
}

func TestDecodeAdditionTriggerStrictFields(t *testing.T) {
	for _, raw := range []string{
		`{"rules":[{"addition_trigger":{"formula":"eval"}}]}`,
		`{"rules":[{"addition_trigger":{"spacing":{"mode":"linear","unknown":1}}}]}`,
		`{"rules":[{"addition_trigger":{"distance":{"value":"1"}}}]}`,
	} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Fatalf("expected strict decode rejection for %s", raw)
		}
	}
	def := validDefinition()
	def.Rules = []RuleSpec{{ID: "legacy", When: Bool(true), Action: ActionSpec{Type: ActionEnter, Side: TradeSideLong, SizePct: 0.1}}}
	normalized, err := Normalize(def)
	if err != nil || normalized.Rules[0].AdditionTrigger != nil {
		t.Fatal("legacy rule must not gain a price trigger")
	}
	raw := `{"rules":[{"id":"a","condition":"market.close.lt(90)","addition_trigger":{"distance":{"type":"percent","value":0.01}}}]}`
	decoded, err := Decode([]byte(raw))
	if err != nil || !strings.Contains(decoded.Rules[0].Condition, "close") {
		t.Fatalf("condition compatibility: %v", err)
	}
}
