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
