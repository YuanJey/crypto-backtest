package indicator

import (
	"encoding/json"
	"math"
	"testing"
)

func TestExtremeRuntimeReturnsRollingHighAndLow(t *testing.T) {
	runtime, err := MustBuiltinRegistry().BuildRuntime("extreme", json.RawMessage(`{"period":3}`))
	if err != nil {
		t.Fatal(err)
	}
	inputs := [][]float64{{10, 7}, {12, 8}, {11, 6}, {9, 5}}
	wants := [][]float64{{math.NaN(), math.NaN()}, {math.NaN(), math.NaN()}, {12, 6}, {12, 5}}
	for index, input := range inputs {
		got := runtime.Update(input)
		for output := range got {
			if math.IsNaN(wants[index][output]) {
				if !math.IsNaN(got[output]) {
					t.Fatalf("update %d output %d=%f, want NaN", index, output, got[output])
				}
				continue
			}
			if got[output] != wants[index][output] {
				t.Fatalf("update %d output %d=%f, want %f", index, output, got[output], wants[index][output])
			}
		}
	}

	runtime.Reset()
	reset := runtime.Update(inputs[0])
	if !math.IsNaN(reset[0]) || !math.IsNaN(reset[1]) {
		t.Fatalf("reset runtime must warm up again: %+v", reset)
	}
}

func TestExtremeDefinitionExposesPriceOutputs(t *testing.T) {
	definition, ok := MustBuiltinRegistry().Definition("extreme")
	if !ok {
		t.Fatal("extreme definition not found")
	}
	if len(definition.Inputs) != 2 || len(definition.Outputs) != 2 {
		t.Fatalf("unexpected extreme definition: %+v", definition)
	}
	if definition.Outputs[0].Name != "highest" || definition.Outputs[1].Name != "lowest" {
		t.Fatalf("unexpected extreme outputs: %+v", definition.Outputs)
	}
	for _, output := range definition.Outputs {
		if output.Semantic != OutputSemanticPrice {
			t.Fatalf("output %q semantic=%q, want price", output.Name, output.Semantic)
		}
	}
}
