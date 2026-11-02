package market

import "testing"

func TestNormalizeSymbol(t *testing.T) {
	tests := map[string]string{
		"BTCUSDT":  "BTC-USDT",
		"btc/usdt": "BTC-USDT",
		"eth_usdc": "ETH-USDC",
	}
	for input, want := range tests {
		got, err := NormalizeSymbol(input)
		if err != nil {
			t.Fatalf("NormalizeSymbol(%q) error: %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeSymbol(%q)=%q, want %q", input, got, want)
		}
	}
}
