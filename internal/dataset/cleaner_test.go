package dataset

import (
	"testing"

	"github.com/bytedance/crypto-backtest/internal/market"
)

func TestCleanBarsReportsGaps(t *testing.T) {
	const timeframeMS = int64(900_000)
	bars := []market.Bar{
		testBar(0),
		testBar(2 * timeframeMS),
	}
	cleaned, report, err := CleanBars(bars, "BTC-USDT", "15m", 0, 3*timeframeMS, DefaultCleanOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(cleaned) != 2 {
		t.Fatalf("len(cleaned)=%d, want 2", len(cleaned))
	}
	if report.MissingBars != 1 {
		t.Fatalf("MissingBars=%d, want 1", report.MissingBars)
	}
	if len(report.Gaps) != 1 || report.Gaps[0].StartMS != timeframeMS || report.Gaps[0].EndMS != 2*timeframeMS {
		t.Fatalf("unexpected gaps: %+v", report.Gaps)
	}
}

func TestCleanBarsRejectsInvalidOHLC(t *testing.T) {
	bar := testBar(0)
	bar.High = 9.5
	_, _, err := CleanBars([]market.Bar{bar}, "BTC-USDT", "15m", 0, 900_000, DefaultCleanOptions())
	if err == nil {
		t.Fatal("expected invalid OHLC to fail")
	}
}

func testBar(openTimeMS int64) market.Bar {
	return market.Bar{
		OpenTimeMS:  openTimeMS,
		Open:        10,
		High:        11,
		Low:         9,
		Close:       10.5,
		Volume:      1,
		QuoteVolume: 10,
		TradeCount:  1,
	}
}
