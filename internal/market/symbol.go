package market

import (
	"fmt"
	"strings"
)

var knownQuoteAssets = []string{"USDT", "USDC", "USD", "BTC", "ETH"}

func NormalizeSymbol(symbol string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(symbol))
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "_", "-")
	if value == "" {
		return "", fmt.Errorf("symbol is empty")
	}

	if strings.Contains(value, "-") {
		parts := strings.Split(value, "-")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", fmt.Errorf("invalid symbol %q", symbol)
		}
		return parts[0] + "-" + parts[1], nil
	}

	for _, quote := range knownQuoteAssets {
		if strings.HasSuffix(value, quote) && len(value) > len(quote) {
			return value[:len(value)-len(quote)] + "-" + quote, nil
		}
	}

	return "", fmt.Errorf("symbol must include a known quote asset: %q", symbol)
}

func ExchangeSymbol(symbol string) string {
	return strings.ReplaceAll(symbol, "-", "")
}
