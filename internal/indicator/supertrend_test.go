package indicator

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSupertrendRuntimeUsesWilderATRAndFlipsDirection(t *testing.T) {
	runtime, err := MustBuiltinRegistry().BuildRuntime(
		"supertrend",
		json.RawMessage(`{"period":2,"multiplier":1}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	inputs := [][]float64{
		{10, 8, 9},
		{11, 9, 10},
		{12, 10, 11},
		{13, 11, 12},
		{11, 7, 8},
	}
	outputs := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		outputs = append(outputs, runtime.Update(input))
	}

	if !math.IsNaN(outputs[0][0]) || !math.IsNaN(outputs[1][0]) {
		t.Fatalf("expected warmup outputs to be NaN: %+v", outputs[:2])
	}
	assertFloat(t, outputs[2][0], 9)
	assertFloat(t, outputs[2][1], 1)
	assertFloat(t, outputs[3][0], 10)
	assertFloat(t, outputs[3][1], 1)
	assertFloat(t, outputs[4][0], 12.5)
	assertFloat(t, outputs[4][1], -1)

	runtime.Reset()
	resetOutput := runtime.Update(inputs[0])
	if !math.IsNaN(resetOutput[0]) || !math.IsNaN(resetOutput[1]) {
		t.Fatalf("expected reset runtime to warm up again: %+v", resetOutput)
	}
}

func TestSupertrendDefinitionExposesValueAndDirection(t *testing.T) {
	definition, ok := MustBuiltinRegistry().Definition("supertrend")
	if !ok {
		t.Fatal("supertrend definition not found")
	}
	if len(definition.Inputs) != 3 || len(definition.Params) != 2 || len(definition.Outputs) != 2 {
		t.Fatalf("unexpected supertrend definition: %+v", definition)
	}
	if definition.Outputs[0].Name != "value" || definition.Outputs[1].Name != "direction" {
		t.Fatalf("unexpected supertrend outputs: %+v", definition.Outputs)
	}
}

func assertFloat(t *testing.T, actual, expected float64) {
	t.Helper()
	if math.Abs(actual-expected) > 1e-9 {
		t.Fatalf("actual=%f, expected=%f", actual, expected)
	}
}
