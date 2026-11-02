package market

import "testing"

func TestParseTimeframeMS(t *testing.T) {
	tests := map[string]int64{
		"15m": 900_000,
		"1h":  3_600_000,
		"1d":  86_400_000,
	}
	for input, want := range tests {
		got, err := ParseTimeframeMS(input)
		if err != nil {
			t.Fatalf("ParseTimeframeMS(%q) error: %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseTimeframeMS(%q)=%d, want %d", input, got, want)
		}
	}
}

func TestRequireBaseTimeframe(t *testing.T) {
	if err := RequireBaseTimeframe("15m"); err != nil {
		t.Fatal(err)
	}
	if err := RequireBaseTimeframe("1h"); err == nil {
		t.Fatal("expected non-15m timeframe to fail")
	}
}
