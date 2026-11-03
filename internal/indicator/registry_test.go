package indicator

import (
	"encoding/json"
	"testing"
)

func TestBuiltinRegistryValidatesParams(t *testing.T) {
	registry := MustBuiltinRegistry()
	if err := registry.ValidateParams("ema", json.RawMessage(`{"period":20}`)); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateParams("boll", json.RawMessage(`{"period":20,"stddev":2}`)); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateParams("supertrend", json.RawMessage(`{"period":10,"multiplier":3}`)); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinRegistryRejectsInvalidParams(t *testing.T) {
	registry := MustBuiltinRegistry()
	tests := []struct {
		name          string
		indicatorType string
		params        string
	}{
		{name: "unknown field", indicatorType: "ema", params: `{"period":20,"bad":1}`},
		{name: "invalid period", indicatorType: "ema", params: `{"period":0}`},
		{name: "invalid boll stddev", indicatorType: "boll", params: `{"period":20,"stddev":0}`},
		{name: "invalid supertrend period", indicatorType: "supertrend", params: `{"period":0,"multiplier":3}`},
		{name: "invalid supertrend multiplier", indicatorType: "supertrend", params: `{"period":10,"multiplier":0}`},
		{name: "unknown supertrend field", indicatorType: "supertrend", params: `{"period":10,"multiplier":3,"bad":1}`},
		{name: "unknown indicator", indicatorType: "unknown", params: `{"period":20}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := registry.ValidateParams(test.indicatorType, json.RawMessage(test.params)); err == nil {
				t.Fatal("expected params validation to fail")
			}
		})
	}
}

func TestBuiltinRegistryExposesOutputSemantics(t *testing.T) {
	registry := MustBuiltinRegistry()
	ema, _ := registry.Definition("ema")
	rsi, _ := registry.Definition("rsi")
	supertrend, _ := registry.Definition("supertrend")

	if ema.Outputs[0].Semantic != OutputSemanticPrice ||
		ema.Outputs[0].CrossGroup != "moving_average" {
		t.Fatalf("unexpected EMA output semantics: %+v", ema.Outputs)
	}
	if rsi.Outputs[0].Semantic != OutputSemanticOscillator ||
		rsi.Outputs[0].CrossGroup != "rsi" {
		t.Fatalf("unexpected RSI output semantics: %+v", rsi.Outputs)
	}
	if supertrend.Outputs[0].Semantic != OutputSemanticTrendLevel ||
		supertrend.Outputs[1].Semantic != OutputSemanticTrendDirection {
		t.Fatalf("unexpected Supertrend output semantics: %+v", supertrend.Outputs)
	}
}
